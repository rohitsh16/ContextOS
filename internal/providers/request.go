package providers

import (
	"net/http"
	"time"

	"contextos/internal/model"
)

// MessageRole represents the role of a message sender in an LLM conversation.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message represents an input or output conversation turn.
type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	Name       string      `json:"name,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
}

// Tool describes a callable function for an LLM.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ContextBundle represents the optimized context selected by ContextOS.
type ContextBundle struct {
	Task            string            `json:"task"`
	Budget          int               `json:"budget"`
	SelectedTokens  int               `json:"selected_tokens"`
	EstimatedCost   float64           `json:"estimated_cost"`
	StablePrefix    []model.Candidate `json:"stable_prefix,omitempty"`
	VariableContext []model.Candidate `json:"variable_context,omitempty"`
	Candidates      []model.Candidate `json:"candidates,omitempty"`
}

// ComputePolicy defines provider-neutral compute controls.
type ComputePolicy struct {
	Effort             EffortLevel   `json:"effort"`
	Adaptive           bool          `json:"adaptive"`
	MaxReasoningTokens *int64        `json:"max_reasoning_tokens,omitempty"`
	MaxOutputTokens    *int64        `json:"max_output_tokens,omitempty"`
	RiskTarget         float64       `json:"risk_target"`
	Timeout            time.Duration `json:"timeout,omitempty"`
}

// BudgetPolicy defines hierarchical budget constraints for task execution.
type BudgetPolicy struct {
	MaxCostUSD          float64 `json:"max_cost_usd"`
	MaxContextTokens    int64   `json:"max_context_tokens"`
	MaxReasoningTokens  int64   `json:"max_reasoning_tokens"`
	MaxToolCalls        int     `json:"max_tool_calls"`
	MaxTurns            int     `json:"max_turns"`
	VerificationReserve float64 `json:"verification_reserve"` // fraction or USD reserved for verification
	EscalationReserve   float64 `json:"escalation_reserve"`   // fraction or USD reserved for escalation
}

// ContinuationState tracks ongoing multi-turn state across model calls.
type ContinuationState struct {
	TurnCount         int    `json:"turn_count"`
	TotalTokensUsed   int64  `json:"total_tokens_used"`
	PreviousAction    string `json:"previous_action,omitempty"`
	ContinuationToken string `json:"continuation_token,omitempty"`
}

// ProviderRequest is the normalized request object passed to any provider adapter.
type ProviderRequest struct {
	Model        string            `json:"model"`
	Messages     []Message         `json:"messages"`
	Tools        []Tool            `json:"tools,omitempty"`
	Context      ContextBundle     `json:"context,omitempty"`
	Compute      ComputePolicy     `json:"compute"`
	Budget       BudgetPolicy      `json:"budget,omitempty"`
	Continuation ContinuationState `json:"continuation,omitempty"`
	Execution    ExecutionOptions  `json:"execution,omitempty"`
}

// ExecutionOptions makes mock versus billable execution explicit. APIKey is
// intentionally request-scoped and omitted from JSON so telemetry cannot leak it.
type ExecutionOptions struct {
	Mode         ProviderMode `json:"mode,omitempty"`
	Endpoint     string       `json:"endpoint,omitempty"`
	ModelVersion string       `json:"model_version,omitempty"`
	APIKey       string       `json:"-"`
	HTTPClient   *http.Client `json:"-"`
}
