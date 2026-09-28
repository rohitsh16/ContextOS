package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"contextos/internal/mcp"
	"contextos/internal/saas/auth"
	"contextos/internal/saas/metering"
	"contextos/internal/saas/proxy"
	"contextos/internal/saas/ratelimit"
	"contextos/internal/saas/tenant"
	"contextos/internal/saas/webhooks"
)

// Config configures the SaaS HTTP Gateway.
type Config struct {
	Port         int
	AdminAPIKey  string
	AuthStore    auth.Store
	RateLimiter  *ratelimit.Limiter
	Meter        *metering.Meter
	TenantMgr    *tenant.Manager
	WebhookDisp  *webhooks.Dispatcher
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Gateway represents the HTTP API Gateway server.
type Gateway struct {
	cfg     Config
	handler http.Handler
}

// New creates and wires up a new SaaS HTTP API Gateway.
func New(cfg Config) (*Gateway, error) {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if cfg.AuthStore == nil {
		cfg.AuthStore = auth.NewMemoryStore()
	}
	if cfg.RateLimiter == nil {
		cfg.RateLimiter = ratelimit.NewLimiter()
	}
	if cfg.Meter == nil {
		cfg.Meter = metering.NewMeter()
	}
	if cfg.TenantMgr == nil {
		mgr, err := tenant.NewManager(tenant.Options{})
		if err != nil {
			return nil, err
		}
		cfg.TenantMgr = mgr
	}
	if cfg.WebhookDisp == nil {
		cfg.WebhookDisp = webhooks.NewDispatcher()
	}

	// Link quota alerts from Meter to Webhook Dispatcher
	cfg.Meter.SetQuotaAlertHandler(func(tenantID string, usagePercent float64, currentTokens, budgetTokens int64) {
		evtType := webhooks.EventQuotaWarning
		if usagePercent >= 100 {
			evtType = webhooks.EventQuotaExceeded
		}
		idBytes := make([]byte, 8)
		_, _ = rand.Read(idBytes)
		_ , _ = cfg.WebhookDisp.Dispatch(webhooks.Event{
			ID:        "evt_" + hex.EncodeToString(idBytes),
			Type:      evtType,
			TenantID:  tenantID,
			Timestamp: time.Now().UTC(),
			Data: map[string]any{
				"usage_percent":  usagePercent,
				"current_tokens": currentTokens,
				"budget_tokens":  budgetTokens,
			},
		})
	})

	gw := &Gateway{cfg: cfg}
	gw.handler = gw.buildRoutes()
	return gw, nil
}

// Handler returns the root HTTP handler for testing and mounting.
func (gw *Gateway) Handler() http.Handler {
	return gw.handler
}

func (gw *Gateway) buildRoutes() http.Handler {
	mux := http.NewServeMux()

	// 1. Ops & Health (public, unauthenticated)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "time": time.Now().UTC().Format(time.RFC3339)})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "tenants_active": strconv.Itoa(gw.cfg.TenantMgr.ActiveTenantCount())})
	})

	// 2. Admin APIs (protected by Admin Key or Admin Scope)
	adminAuth := gw.adminAuthMiddleware
	mux.Handle("POST /v1/admin/tenants", adminAuth(http.HandlerFunc(gw.handleAdminCreateTenant)))
	mux.Handle("GET /v1/admin/tenants", adminAuth(http.HandlerFunc(gw.handleAdminListTenants)))
	mux.Handle("POST /v1/admin/keys", adminAuth(http.HandlerFunc(gw.handleAdminCreateKey)))
	mux.Handle("GET /v1/admin/keys", adminAuth(http.HandlerFunc(gw.handleAdminListKeys)))
	mux.Handle("DELETE /v1/admin/keys/{id}", adminAuth(http.HandlerFunc(gw.handleAdminRevokeKey)))
	mux.Handle("POST /v1/admin/webhooks", adminAuth(http.HandlerFunc(gw.handleAdminRegisterWebhook)))

	// 3. Customer Tenant APIs (Protected by API Key Auth + Rate Limiting)
	customerStack := func(h http.Handler) http.Handler {
		return auth.Middleware(gw.cfg.AuthStore)(
			gw.cfg.RateLimiter.Middleware()(h),
		)
	}

	// Context Planning & Retrieval
	mux.Handle("POST /v1/context/plan", customerStack(http.HandlerFunc(gw.handlePlan)))
	mux.Handle("POST /v1/context/compute-plan", customerStack(http.HandlerFunc(gw.handleComputePlan)))
	mux.Handle("POST /v1/context/search", customerStack(http.HandlerFunc(gw.handleSearch)))
	mux.Handle("POST /v1/context/remember", customerStack(http.HandlerFunc(gw.handleRemember)))
	mux.Handle("GET /v1/context/stats", customerStack(http.HandlerFunc(gw.handleStats)))
	mux.Handle("POST /v1/context/route", customerStack(http.HandlerFunc(gw.handleRoute)))

	// Sessions & Audit Trail
	mux.Handle("POST /v1/work-items", customerStack(http.HandlerFunc(gw.handleWorkItem)))
	mux.Handle("POST /v1/sessions", customerStack(http.HandlerFunc(gw.handleStartSession)))
	mux.Handle("POST /v1/sessions/{id}/events", customerStack(http.HandlerFunc(gw.handleRecordEvent)))
	mux.Handle("GET /v1/sessions/latest", customerStack(http.HandlerFunc(gw.handleLatestSession)))
	mux.Handle("GET /v1/resume", customerStack(http.HandlerFunc(gw.handleResume)))

	// Billing & Usage
	mux.Handle("GET /v1/usage", customerStack(http.HandlerFunc(gw.handleUsage)))
	mux.Handle("GET /v1/invoice", customerStack(http.HandlerFunc(gw.handleInvoice)))

	// MCP-over-HTTP JSON-RPC 2.0
	mux.Handle("POST /v1/mcp", customerStack(http.HandlerFunc(gw.handleMCP)))

	// Hosted Model Proxy (/v1/chat/completions)
	proxyHandler := proxy.NewHandler(gw.cfg.TenantMgr, gw.cfg.Meter, "")
	mux.Handle("POST /v1/chat/completions", customerStack(proxyHandler))

	// Wrap root with recovery and request ID logging
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			reqID = "req_" + hex.EncodeToString(buf)
		}
		w.Header().Set("X-Request-ID", reqID)
		mux.ServeHTTP(w, r)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// CUSTOMER ENDPOINTS
// ─────────────────────────────────────────────────────────────────────────────

type PlanRequest struct {
	Task   string `json:"task"`
	Model  string `json:"model,omitempty"`
	Budget int    `json:"budget,omitempty"`
}

func (gw *Gateway) handlePlan(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	keyObj := auth.GetAPIKey(r.Context())

	var req PlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Task) == "" {
		writeError(w, http.StatusBadRequest, "task cannot be empty")
		return
	}

	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve tenant service: "+err.Error())
		return
	}

	start := time.Now()
	p, err := svc.Plan(req.Task, req.Model, req.Budget)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		writeError(w, http.StatusInternalServerError, "plan failed: "+err.Error())
		return
	}

	// Meter the usage
	var keyID string
	if keyObj != nil {
		keyID = keyObj.ID
	}
	var cachedTokens int64
	if p.CacheHit {
		cachedTokens = int64(p.SelectedTokens)
	}
	gw.cfg.Meter.Record(metering.UsageEvent{
		TenantID:      tenantObj.ID,
		KeyID:         keyID,
		Timestamp:     time.Now().UTC(),
		Endpoint:      "/v1/context/plan",
		Model:         p.Model,
		InputTokens:   int64(p.SelectedTokens),
		CachedTokens:  cachedTokens,
		EstimatedCost: p.EstimatedCost,
		CacheHit:      p.CacheHit,
		LatencyMS:     latency,
	})
	gw.cfg.Meter.CheckQuota(tenantObj)

	writeJSON(w, http.StatusOK, p)
}

type ComputePlanRequest struct {
	Task       string  `json:"task"`
	RiskTarget float64 `json:"risk_target,omitempty"`
	Model      string  `json:"model,omitempty"`
}

func (gw *Gateway) handleComputePlan(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	var req ComputePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cp := svc.ComputePlan(req.Task, req.RiskTarget, req.Model)

	// Meter compute plan query
	gw.cfg.Meter.Record(metering.UsageEvent{
		TenantID:      tenantObj.ID,
		Endpoint:      "/v1/context/compute-plan",
		Model:         req.Model,
		EstimatedCost: cp.EstimatedCostUSD,
	})

	writeJSON(w, http.StatusOK, cp)
}

type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

func (gw *Gateway) handleSearch(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}

	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := svc.SearchCandidates(req.Query, req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": res})
}

type RememberRequest struct {
	Kind       string   `json:"kind"`
	Content    string   `json:"content"`
	Author     string   `json:"author,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	WorkItemID string   `json:"work_item_id,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

func (gw *Gateway) handleRemember(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	var req RememberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content cannot be empty")
		return
	}
	if req.Confidence <= 0 {
		req.Confidence = 0.9
	}
	if req.Author == "" {
		req.Author = "user"
	}
	if req.Scope == "" {
		req.Scope = "repo"
	}

	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	memID, err := svc.Remember(req.Kind, req.Content, req.Author, req.Scope, req.WorkItemID, req.Confidence, req.Tags)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"memory_id": memID, "status": "stored"})
}

func (gw *Gateway) handleStats(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stats, err := svc.Stats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type RouteRequest struct {
	Task   string `json:"task"`
	Budget int    `json:"budget,omitempty"`
}

func (gw *Gateway) handleRoute(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res := svc.Route(req.Task, req.Budget)
	writeJSON(w, http.StatusOK, res)
}

type WorkItemRequest struct {
	Title string `json:"title"`
}

func (gw *Gateway) handleWorkItem(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	var req WorkItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	wi, err := svc.StartWorkItem(req.Title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wi)
}

type SessionRequest struct {
	Agent      string `json:"agent"`
	WorkItemID string `json:"work_item_id,omitempty"`
}

func (gw *Gateway) handleStartSession(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	var req SessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sess, err := svc.StartSession(req.Agent, req.WorkItemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

type EventRequest struct {
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
}

func (gw *Gateway) handleRecordEvent(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	var req EventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.RecordEvent(sessionID, req.EventType, req.Payload); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func (gw *Gateway) handleLatestSession(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sess, err := svc.LatestSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (gw *Gateway) handleResume(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	state, err := svc.Resume()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (gw *Gateway) handleUsage(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	agg, ok := gw.cfg.Meter.GetAggregate(tenantObj.ID)
	if !ok {
		agg = &metering.TenantUsageAggregate{
			TenantID: tenantObj.ID,
		}
	}
	writeJSON(w, http.StatusOK, agg)
}

func (gw *Gateway) handleInvoice(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())
	agg, ok := gw.cfg.Meter.GetAggregate(tenantObj.ID)
	if !ok {
		agg = &metering.TenantUsageAggregate{
			TenantID: tenantObj.ID,
		}
	}
	inv := metering.GenerateInvoice(tenantObj, agg)
	writeJSON(w, http.StatusOK, inv)
}

// ─────────────────────────────────────────────────────────────────────────────
// MCP OVER HTTP (JSON-RPC 2.0)
// ─────────────────────────────────────────────────────────────────────────────

func (gw *Gateway) handleMCP(w http.ResponseWriter, r *http.Request) {
	tenantObj := auth.GetTenant(r.Context())

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeRPCError(w, nil, -32700, "Parse error: "+err.Error())
		return
	}

	var rpcReq mcp.Request
	if err := json.Unmarshal(bodyBytes, &rpcReq); err != nil {
		writeRPCError(w, nil, -32700, "Parse error: "+err.Error())
		return
	}

	svc, err := gw.cfg.TenantMgr.GetService(tenantObj.ID)
	if err != nil {
		writeRPCError(w, rpcReq.ID, -32603, "Internal error: "+err.Error())
		return
	}

	mcpHandler := mcp.New(svc)
	rpcResp := mcpHandler.Handle(rpcReq)

	// Meter MCP context planning tool calls
	if rpcReq.Method == "tools/call" {
		var toolCall struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(rpcReq.Params, &toolCall); err == nil {
			gw.cfg.Meter.Record(metering.UsageEvent{
				TenantID: tenantObj.ID,
				Endpoint: "/v1/mcp/" + toolCall.Name,
			})
		}
	}

	writeJSON(w, http.StatusOK, rpcResp)
}

// ─────────────────────────────────────────────────────────────────────────────
// ADMIN ENDPOINTS
// ─────────────────────────────────────────────────────────────────────────────

func (gw *Gateway) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawKey := auth.ExtractRawKey(r)
		if rawKey == "" {
			writeError(w, http.StatusUnauthorized, "admin credentials required")
			return
		}

		// Master Admin Key bypass
		if gw.cfg.AdminAPIKey != "" && rawKey == gw.cfg.AdminAPIKey {
			next.ServeHTTP(w, r)
			return
		}

		// Or valid key with ScopeContextAdmin
		tenantObj, keyObj, err := gw.cfg.AuthStore.Authenticate(rawKey)
		if err != nil || keyObj == nil || !keyObj.HasScope(auth.ScopeContextAdmin) {
			writeError(w, http.StatusForbidden, "forbidden: admin scope required")
			return
		}

		ctx := auth.WithTenantContext(r.Context(), tenantObj, keyObj)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type AdminCreateTenantRequest struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Tier               string            `json:"tier"`
	MonthlyTokenBudget int64             `json:"monthly_token_budget,omitempty"`
	MonthlyQueryBudget int64             `json:"monthly_query_budget,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

func (gw *Gateway) handleAdminCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	if req.Tier == "" {
		req.Tier = auth.TierFree
	}

	tenantObj := &auth.Tenant{
		ID:                 req.ID,
		Name:               req.Name,
		Tier:               req.Tier,
		Active:             true,
		MonthlyTokenBudget: req.MonthlyTokenBudget,
		MonthlyQueryBudget: req.MonthlyQueryBudget,
		CreatedAt:          time.Now().UTC(),
		Metadata:           req.Metadata,
	}

	if err := gw.cfg.AuthStore.CreateTenant(tenantObj); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, tenantObj)
}

func (gw *Gateway) handleAdminListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := gw.cfg.AuthStore.ListTenants()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
}

type AdminCreateKeyRequest struct {
	TenantID     string   `json:"tenant_id"`
	Name         string   `json:"name"`
	Tier         string   `json:"tier,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	RateLimitQPS int      `json:"rate_limit_qps,omitempty"`
	ExpiresInDays int     `json:"expires_in_days,omitempty"`
}

func (gw *Gateway) handleAdminCreateKey(w http.ResponseWriter, r *http.Request) {
	var req AdminCreateKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id is required")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		exp := time.Now().UTC().AddDate(0, 0, req.ExpiresInDays)
		expiresAt = &exp
	}

	res, err := auth.GenerateAPIKey(req.TenantID, req.Name, req.Tier, req.Scopes, req.RateLimitQPS, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := gw.cfg.AuthStore.SaveKey(res.Key); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func (gw *Gateway) handleAdminListKeys(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "tenant_id query parameter required")
		return
	}
	keys, err := gw.cfg.AuthStore.ListKeysForTenant(tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (gw *Gateway) handleAdminRevokeKey(w http.ResponseWriter, r *http.Request) {
	keyID := r.PathValue("id")
	if keyID == "" {
		writeError(w, http.StatusBadRequest, "key id required")
		return
	}
	if err := gw.cfg.AuthStore.RevokeKey(keyID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "key_id": keyID})
}

type AdminWebhookRequest struct {
	TenantID string   `json:"tenant_id"`
	URL      string   `json:"url"`
	Secret   string   `json:"secret"`
	Events   []string `json:"events"`
}

func (gw *Gateway) handleAdminRegisterWebhook(w http.ResponseWriter, r *http.Request) {
	var req AdminWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TenantID == "" || req.URL == "" || req.Secret == "" {
		writeError(w, http.StatusBadRequest, "tenant_id, url, and secret are required")
		return
	}

	sub := webhooks.Subscription{
		TenantID: req.TenantID,
		URL:      req.URL,
		Secret:   req.Secret,
		Events:   req.Events,
		Active:   true,
	}
	gw.cfg.WebhookDisp.RegisterSubscription(sub)
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered", "tenant_id": req.TenantID})
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
		},
	})
}

func writeRPCError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // JSON-RPC errors typically return 200 HTTP with error object
	_ = json.NewEncoder(w).Encode(mcp.Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &mcp.RPCError{
			Code:    code,
			Message: msg,
		},
	})
}
