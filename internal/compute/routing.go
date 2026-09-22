package compute

import (
	"contextos/internal/providers"
)

// ModelCandidate describes a candidate model for routing decisions.
type ModelCandidate struct {
	Provider string                      `json:"provider"`
	Model    string                      `json:"model"`
	Tier     string                      `json:"tier"` // "cheap", "strong", "strongest"

	Capabilities providers.ModelCapabilities `json:"capabilities"`

	ExpectedInputCost     float64 `json:"expected_input_cost"`
	ExpectedReasoningCost float64 `json:"expected_reasoning_cost"`
	ExpectedOutputCost    float64 `json:"expected_output_cost"`

	HistoricalSuccessRate float64 `json:"historical_success_rate"`
	HistoricalLatencyMS   float64 `json:"historical_latency_ms"`
}

// ExpectedCost returns the total expected cost for this model.
func (mc ModelCandidate) ExpectedCost() float64 {
	return mc.ExpectedInputCost + mc.ExpectedReasoningCost + mc.ExpectedOutputCost
}

// CostPerSuccess returns E[Cost(m)] / P(success | m, task).
func (mc ModelCandidate) CostPerSuccess() float64 {
	if mc.HistoricalSuccessRate <= 0 {
		return 1e9 // Extreme penalty for 0 success rate
	}
	return mc.ExpectedCost() / mc.HistoricalSuccessRate
}

// ModelRouter manages model selection and multi-tiered cascades.
type ModelRouter struct {
	candidates []ModelCandidate
}

// NewModelRouter initializes a router with standard multi-provider model candidates.
func NewModelRouter() *ModelRouter {
	reg := providers.NewRegistry()

	// 1. Cheap tier
	oMiniCaps, _ := reg.Lookup("openai", "o3-mini")
	gFlashCaps, _ := reg.Lookup("gemini", "gemini-2.5-flash")
	cHaikuCaps, _ := reg.Lookup("anthropic", "claude-3-5-haiku")

	// 2. Strong tier
	cSonnetCaps, _ := reg.Lookup("anthropic", "claude-3-7-sonnet")
	gProCaps, _ := reg.Lookup("gemini", "gemini-2.5-pro")

	// 3. Strongest tier
	o1Caps, _ := reg.Lookup("openai", "o1")

	candidates := []ModelCandidate{
		{
			Provider:              "openai",
			Model:                 "o3-mini",
			Tier:                  "cheap",
			Capabilities:          oMiniCaps,
			ExpectedInputCost:     0.002,
			ExpectedReasoningCost: 0.008,
			ExpectedOutputCost:    0.004,
			HistoricalSuccessRate: 0.82,
			HistoricalLatencyMS:   1200,
		},
		{
			Provider:              "gemini",
			Model:                 "gemini-2.5-flash",
			Tier:                  "cheap",
			Capabilities:          gFlashCaps,
			ExpectedInputCost:     0.0005,
			ExpectedReasoningCost: 0.001,
			ExpectedOutputCost:    0.0005,
			HistoricalSuccessRate: 0.78,
			HistoricalLatencyMS:   800,
		},
		{
			Provider:              "anthropic",
			Model:                 "claude-3-5-haiku",
			Tier:                  "cheap",
			Capabilities:          cHaikuCaps,
			ExpectedInputCost:     0.001,
			ExpectedReasoningCost: 0.002,
			ExpectedOutputCost:    0.002,
			HistoricalSuccessRate: 0.79,
			HistoricalLatencyMS:   750,
		},
		{
			Provider:              "anthropic",
			Model:                 "claude-3-7-sonnet",
			Tier:                  "strong",
			Capabilities:          cSonnetCaps,
			ExpectedInputCost:     0.006,
			ExpectedReasoningCost: 0.025,
			ExpectedOutputCost:    0.015,
			HistoricalSuccessRate: 0.94,
			HistoricalLatencyMS:   2500,
		},
		{
			Provider:              "gemini",
			Model:                 "gemini-2.5-pro",
			Tier:                  "strong",
			Capabilities:          gProCaps,
			ExpectedInputCost:     0.003,
			ExpectedReasoningCost: 0.012,
			ExpectedOutputCost:    0.008,
			HistoricalSuccessRate: 0.91,
			HistoricalLatencyMS:   2200,
		},
		{
			Provider:              "openai",
			Model:                 "o1",
			Tier:                  "strongest",
			Capabilities:          o1Caps,
			ExpectedInputCost:     0.030,
			ExpectedReasoningCost: 0.120,
			ExpectedOutputCost:    0.050,
			HistoricalSuccessRate: 0.97,
			HistoricalLatencyMS:   6000,
		},
	}

	return &ModelRouter{candidates: candidates}
}

// SelectOptimalModel picks m* = argmin_m (E[Cost(m)] / P(success | m, task)) subject to difficulty.
func (mr *ModelRouter) SelectOptimalModel(difficulty float64, preferredProvider string) ModelCandidate {
	var pool []ModelCandidate
	for _, c := range mr.candidates {
		// Filter by preferred provider if specified
		if preferredProvider != "" && c.Provider != preferredProvider {
			continue
		}
		// Critical tasks should not use cheap tier
		if difficulty > 0.75 && c.Tier == "cheap" {
			continue
		}
		pool = append(pool, c)
	}

	if len(pool) == 0 {
		pool = mr.candidates
	}

	best := pool[0]
	bestCPS := best.CostPerSuccess()

	for _, c := range pool[1:] {
		cps := c.CostPerSuccess()
		if cps < bestCPS {
			bestCPS = cps
			best = c
		}
	}

	return best
}

// CascadePlan specifies the multi-tier cascade order for a task.
type CascadePlan struct {
	InitialModel   ModelCandidate   `json:"initial_model"`
	EscalateModel  *ModelCandidate  `json:"escalate_model,omitempty"`
	FallbackModel  *ModelCandidate  `json:"fallback_model,omitempty"`
}

// BuildCascade constructs an adaptive cascade based on task difficulty.
func (mr *ModelRouter) BuildCascade(difficulty float64) CascadePlan {
	if difficulty < 0.30 {
		// Trivial tasks start with cheap tier, escalate to strong tier if needed
		cheap := mr.SelectOptimalModel(difficulty, "")
		strong := mr.candidates[3] // claude-3-7-sonnet
		return CascadePlan{
			InitialModel:  cheap,
			EscalateModel: &strong,
		}
	} else if difficulty < 0.75 {
		// Moderate to difficult tasks start directly with strong tier
		strong := mr.candidates[3] // claude-3-7-sonnet
		strongest := mr.candidates[5] // o1
		return CascadePlan{
			InitialModel:  strong,
			EscalateModel: &strongest,
		}
	} else {
		// Critical tasks start with strongest model directly
		strongest := mr.candidates[5] // o1
		return CascadePlan{
			InitialModel: strongest,
		}
	}
}

// Candidates returns the list of model candidates registered in the router.
func (mr *ModelRouter) Candidates() []ModelCandidate {
	return mr.candidates
}

// DefaultModels returns a default slice of candidate models across providers.
func DefaultModels() []ModelCandidate {
	return NewModelRouter().candidates
}
