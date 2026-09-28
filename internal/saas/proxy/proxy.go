package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"contextos/internal/saas/auth"
	"contextos/internal/saas/metering"
	"contextos/internal/saas/tenant"
)

// ChatCompletionMessage represents an OpenAI-compatible message.
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents an OpenAI-compatible /v1/chat/completions payload.
type ChatCompletionRequest struct {
	Model       string                  `json:"model"`
	Messages    []ChatCompletionMessage `json:"messages"`
	Temperature float64                 `json:"temperature,omitempty"`
	MaxTokens   int                     `json:"max_tokens,omitempty"`
	Stream      bool                    `json:"stream,omitempty"`
}

// ChatCompletionChoice represents a completion choice in response.
type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

// UsageStatistics tracks token counts in the completion response.
type UsageStatistics struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionResponse represents an OpenAI-compatible response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   UsageStatistics        `json:"usage"`
}

// Handler intercepts LLM completion calls, injects ContextOS context, and meters consumption.
type Handler struct {
	tenantMgr   *tenant.Manager
	meter       *metering.Meter
	upstreamURL string // e.g. https://api.openai.com
	httpClient  *http.Client
}

// NewHandler creates a new hosted model proxy handler.
func NewHandler(tm *tenant.Manager, m *metering.Meter, upstreamURL string) *Handler {
	if upstreamURL == "" {
		upstreamURL = "https://api.openai.com"
	}
	return &Handler{
		tenantMgr:   tm,
		meter:       m,
		upstreamURL: strings.TrimRight(upstreamURL, "/"),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// SetTransport allows injecting custom transport for tests or enterprise firewalls.
func (h *Handler) SetTransport(rt http.RoundTripper) {
	h.httpClient.Transport = rt
}

// ServeHTTP handles /v1/chat/completions requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantObj := auth.GetTenant(r.Context())
	if tenantObj == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ChatCompletionRequest
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// 1. Extract prompt / task from the last user message
	task := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			task = req.Messages[i].Content
			break
		}
	}

	svc, err := h.tenantMgr.GetService(tenantObj.ID)
	if err != nil {
		http.Error(w, "Failed to resolve tenant service: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Execute ContextOS Plan to optimize prompt context
	start := time.Now()
	budget := 4000
	if req.MaxTokens > 0 {
		budget = req.MaxTokens
	}
	plan, err := svc.Plan(task, req.Model, budget)
	if err != nil {
		http.Error(w, "Context planning error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Inject optimized ContextOS context into system prompt
	renderedContext := svc.RenderPlan(plan)
	if renderedContext != "" {
		systemMsg := ChatCompletionMessage{
			Role:    "system",
			Content: "ContextOS Authoritative Context:\n" + renderedContext,
		}
		// Prepend system message
		req.Messages = append([]ChatCompletionMessage{systemMsg}, req.Messages...)
	}

	// 4. Forward or handle completion
	var resp ChatCompletionResponse
	upstreamKey := r.Header.Get("X-Upstream-API-Key")
	if upstreamKey == "" {
		upstreamKey = r.Header.Get("Authorization")
	}

	// If upstream key is not provided or in mock mode, provide synthetic response
	if upstreamKey == "" || strings.Contains(r.Header.Get("X-ContextOS-Mock"), "true") {
		idBuf := make([]byte, 12)
		_, _ = rand.Read(idBuf)
		resp = ChatCompletionResponse{
			ID:      "chatcmpl-" + hex.EncodeToString(idBuf),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []ChatCompletionChoice{
				{
					Index: 0,
					Message: ChatCompletionMessage{
						Role:    "assistant",
						Content: "Processed with ContextOS optimization: " + task,
					},
					FinishReason: "stop",
				},
			},
			Usage: UsageStatistics{
				PromptTokens:     plan.SelectedTokens,
				CompletionTokens: 50,
				TotalTokens:      plan.SelectedTokens + 50,
			},
		}
	} else {
		// Forward to upstream provider
		upReqBody, _ := json.Marshal(req)
		upReq, err := http.NewRequest(http.MethodPost, h.upstreamURL+"/v1/chat/completions", bytes.NewReader(upReqBody))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Authorization", upstreamKey)

		upResp, err := h.httpClient.Do(upReq)
		if err != nil {
			http.Error(w, "Upstream LLM error: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer upResp.Body.Close()

		if err := json.NewDecoder(upResp.Body).Decode(&resp); err != nil {
			http.Error(w, "Invalid upstream response", http.StatusBadGateway)
			return
		}
	}

	latencyMS := time.Since(start).Milliseconds()

	// 5. Meter usage for SaaS billing
	var cachedTokens int64
	if plan.CacheHit {
		cachedTokens = int64(plan.SelectedTokens)
	}
	h.meter.Record(metering.UsageEvent{
		TenantID:      tenantObj.ID,
		Endpoint:      "/v1/chat/completions",
		Model:         req.Model,
		InputTokens:   int64(resp.Usage.PromptTokens),
		CachedTokens:  cachedTokens,
		OutputTokens:  int64(resp.Usage.CompletionTokens),
		EstimatedCost: plan.EstimatedCost,
		CacheHit:      plan.CacheHit,
		LatencyMS:     latencyMS,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-ContextOS-Cache-Hit", fmt.Sprintf("%v", plan.CacheHit))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
