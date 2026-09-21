package providers

import "contextos/internal/telemetry"

// FinishReason describes why the provider finished generating.
type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonLength    FinishReason = "length"
	FinishReasonToolCalls FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonError     FinishReason = "error"
)

// ToolCall represents an invocation of a tool by the model.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ProviderState holds opaque or continuation state returned by the provider.
type ProviderState struct {
	RawResponseID string         `json:"raw_response_id,omitempty"`
	SystemFingerprint string     `json:"system_fingerprint,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ProviderResponse is the standardized response returned by any provider adapter.
type ProviderResponse struct {
	Text          string                 `json:"text"`
	ToolCalls     []ToolCall             `json:"tool_calls,omitempty"`
	Usage         telemetry.UsageMetrics `json:"usage"`
	FinishReason  FinishReason           `json:"finish_reason"`
	ProviderState ProviderState          `json:"provider_state,omitempty"`
}
