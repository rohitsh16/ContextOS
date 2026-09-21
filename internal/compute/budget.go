package compute

import (
	"errors"
	"sync"

	"contextos/internal/providers"
)

// BudgetPolicy is re-exported from providers package.
type BudgetPolicy = providers.BudgetPolicy

// BudgetCategory represents a slice of the hierarchical budget.
type BudgetCategory string

const (
	BudgetCategoryContext      BudgetCategory = "context"
	BudgetCategoryRetrieval    BudgetCategory = "retrieval"
	BudgetCategoryReasoning    BudgetCategory = "reasoning"
	BudgetCategoryTools        BudgetCategory = "tools"
	BudgetCategoryVerification BudgetCategory = "verification"
	BudgetCategoryEscalation   BudgetCategory = "escalation"
)

// BudgetState represents the instantaneous available resources under a budget.
type BudgetState struct {
	MaxCostUSD          float64 `json:"max_cost_usd"`
	SpentCostUSD        float64 `json:"spent_cost_usd"`
	RemainingCostUSD    float64 `json:"remaining_cost_usd"`

	MaxReasoningTokens       int64 `json:"max_reasoning_tokens"`
	SpentReasoningTokens     int64 `json:"spent_reasoning_tokens"`
	RemainingReasoningTokens int64 `json:"remaining_reasoning_tokens"`

	MaxToolCalls       int `json:"max_tool_calls"`
	SpentToolCalls     int `json:"spent_tool_calls"`
	RemainingToolCalls int `json:"remaining_tool_calls"`

	VerificationReserveUSD float64 `json:"verification_reserve_usd"`
	EscalationReserveUSD   float64 `json:"escalation_reserve_usd"`
}

// HierarchicalBudgetManager ensures no action overdrafts reserved resources.
type HierarchicalBudgetManager struct {
	mu     sync.Mutex
	policy BudgetPolicy

	spentCostUSD          float64
	spentReasoningTokens  int64
	spentContextTokens    int64
	spentToolCalls        int
	spentTurns            int

	// Category breakdowns
	spentByCategory map[BudgetCategory]float64
}

// NewBudgetManager initializes a budget manager with the specified policy.
func NewBudgetManager(p BudgetPolicy) *HierarchicalBudgetManager {
	if p.MaxCostUSD <= 0 {
		p.MaxCostUSD = 2.00 // Default $2.00 max task budget
	}
	if p.MaxReasoningTokens <= 0 {
		p.MaxReasoningTokens = 32768
	}
	if p.MaxToolCalls <= 0 {
		p.MaxToolCalls = 25
	}
	if p.MaxTurns <= 0 {
		p.MaxTurns = 20
	}
	if p.VerificationReserve <= 0 {
		p.VerificationReserve = 0.20 // 20% reserved for verification
	}
	if p.EscalationReserve <= 0 {
		p.EscalationReserve = 0.20 // 20% reserved for escalation
	}

	return &HierarchicalBudgetManager{
		policy:          p,
		spentByCategory: make(map[BudgetCategory]float64),
	}
}

// State returns the current snapshot of budget utilization.
func (bm *HierarchicalBudgetManager) State() BudgetState {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	remCost := bm.policy.MaxCostUSD - bm.spentCostUSD
	if remCost < 0 {
		remCost = 0
	}
	remReasoning := bm.policy.MaxReasoningTokens - bm.spentReasoningTokens
	if remReasoning < 0 {
		remReasoning = 0
	}
	remTools := bm.policy.MaxToolCalls - bm.spentToolCalls
	if remTools < 0 {
		remTools = 0
	}

	return BudgetState{
		MaxCostUSD:               bm.policy.MaxCostUSD,
		SpentCostUSD:             bm.spentCostUSD,
		RemainingCostUSD:         remCost,
		MaxReasoningTokens:       bm.policy.MaxReasoningTokens,
		SpentReasoningTokens:     bm.spentReasoningTokens,
		RemainingReasoningTokens: remReasoning,
		MaxToolCalls:             bm.policy.MaxToolCalls,
		SpentToolCalls:           bm.spentToolCalls,
		RemainingToolCalls:       remTools,
		VerificationReserveUSD:   bm.policy.MaxCostUSD * bm.policy.VerificationReserve,
		EscalationReserveUSD:     bm.policy.MaxCostUSD * bm.policy.EscalationReserve,
	}
}

// CanSpend checks if a proposed spend can be accommodated without violating reserves or limits.
func (bm *HierarchicalBudgetManager) CanSpend(cat BudgetCategory, costUSD float64, reasoningTokens int64) bool {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Check reasoning token limit
	if reasoningTokens > 0 && bm.spentReasoningTokens+reasoningTokens > bm.policy.MaxReasoningTokens {
		return false
	}

	totalProjected := bm.spentCostUSD + costUSD
	if totalProjected > bm.policy.MaxCostUSD {
		return false
	}

	// Calculate protected reserves
	verificationReserve := bm.policy.MaxCostUSD * bm.policy.VerificationReserve
	escalationReserve := bm.policy.MaxCostUSD * bm.policy.EscalationReserve

	// If the category is NOT verification or escalation, it cannot consume reserved pool
	if cat != BudgetCategoryVerification && cat != BudgetCategoryEscalation {
		availableForNormalActions := bm.policy.MaxCostUSD - verificationReserve - escalationReserve
		if totalProjected > availableForNormalActions {
			return false
		}
	} else if cat == BudgetCategoryVerification {
		// Verification can use general pool + verification reserve, but cannot touch escalation reserve
		availableForVerification := bm.policy.MaxCostUSD - escalationReserve
		if totalProjected > availableForVerification {
			return false
		}
	}

	return true
}

// Spend commits the expenditure to the budget manager.
func (bm *HierarchicalBudgetManager) Spend(cat BudgetCategory, costUSD float64, reasoningTokens int64, toolCalls int) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	totalProjected := bm.spentCostUSD + costUSD
	if totalProjected > bm.policy.MaxCostUSD {
		return errors.New("budget exceeded: task max cost limit reached")
	}

	bm.spentCostUSD += costUSD
	bm.spentReasoningTokens += reasoningTokens
	bm.spentToolCalls += toolCalls
	bm.spentByCategory[cat] += costUSD

	return nil
}
