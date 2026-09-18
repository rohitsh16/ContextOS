package model

import "time"

type Memory struct {
	ID                    string  `json:"id"`
	Kind                  string  `json:"kind"`
	Content               string  `json:"content"`
	Scope                 string  `json:"scope"`
	ValidFromRevision     string  `json:"valid_from_revision,omitempty"`
	InvalidatedAtRevision string  `json:"invalidated_at_revision,omitempty"`
	Authority             string  `json:"authority"`
	Confidence            float64 `json:"confidence"`
	TokenCost             int     `json:"token_cost"`
	ReuseCount            int     `json:"reuse_count"`
	LastAccessedAt        string  `json:"last_accessed_at,omitempty"`
	Source                string  `json:"source,omitempty"`
	Location              string  `json:"location,omitempty"`
}

type Candidate struct {
	Source           string  `json:"source"`
	Location         string  `json:"location,omitempty"`
	ID               string  `json:"id"`
	Kind             string  `json:"kind"`
	Content          string  `json:"content"`
	Tokens           int     `json:"tokens"`
	Semantic         float64 `json:"semantic"`
	Lexical          float64 `json:"lexical"`
	Graph            float64 `json:"graph"`
	Freshness        float64 `json:"freshness"`
	Authority        float64 `json:"authority"`
	Reuse            float64 `json:"reuse"`
	TaskAffinity     float64 `json:"task_affinity"`
	Evidence         float64 `json:"evidence"`
	StaleRisk        float64 `json:"stale_risk"`
	CacheValue       float64 `json:"cache_value"`
	MarginalEstimate float64 `json:"marginal_estimate"`
	Density          float64 `json:"density"`
	Score            float64 `json:"score"`
	Selected         bool    `json:"selected"`
	Reason           string  `json:"reason,omitempty"`
}

type WorkItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Branch    string `json:"branch"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Session struct {
	ID        string `json:"id"`
	Agent     string `json:"agent"`
	WorkItem  string `json:"work_item,omitempty"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at,omitempty"`
}

type ContextPlan struct {
	Task            string      `json:"task"`
	Model           string      `json:"model"`
	Budget          int         `json:"budget"`
	SelectedTokens  int         `json:"selected_tokens"`
	EstimatedCost   float64     `json:"estimated_cost"`
	CacheHit        bool        `json:"cache_hit"`
	StablePrefix    []Candidate `json:"stable_prefix,omitempty"`
	VariableContext []Candidate `json:"variable_context,omitempty"`
	Candidates      []Candidate `json:"candidates"`
	Selected        []Candidate `json:"selected"`
	CreatedAt       time.Time   `json:"created_at"`
}

type HookEvent struct {
	Agent     string         `json:"agent"`
	EventType string         `json:"event_type"`
	CWD       string         `json:"cwd,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Prompt    string         `json:"prompt,omitempty"`
	Output    string         `json:"output,omitempty"`
	IsFailure bool           `json:"is_failure,omitempty"`
	Raw       map[string]any `json:"raw,omitempty"`
}
