package dreamrsi

import (
	"sort"
	"strings"
	"time"

	"contextos/internal/model"
)

// SimulationScenario represents a historical task with recorded ground truth
// evidence and baseline performance, ready for counterfactual replay.
type SimulationScenario struct {
	ID                 string            `json:"id"`
	Task               string            `json:"task"`
	Budget             int               `json:"budget"`
	Candidates         []model.Candidate `json:"candidates"`
	RequiredEvidence   []string          `json:"required_evidence"`
	BaselineTokens     int               `json:"baseline_tokens"`
	BaselineEfficiency float64           `json:"baseline_efficiency"`
}

// SimulationResult captures the counterfactual outcome of replaying a single task
// under a mutated exploration policy.
type SimulationResult struct {
	ScenarioID        string        `json:"scenario_id"`
	PolicyID          string        `json:"policy_id"`
	SimulatedTokens   int           `json:"simulated_tokens"`
	TokenReductionPct float64       `json:"token_reduction_pct"`
	EvidenceRecall    float64       `json:"evidence_recall"`
	PrunedCandidates  int           `json:"pruned_candidates"`
	PredictedSuccess  bool          `json:"predicted_success"`
	ExecutionDuration time.Duration `json:"execution_duration"`
}

// AggregateReplayReport summarizes the performance of an exploration policy
// across an entire corpus of replay scenarios.
type AggregateReplayReport struct {
	PolicyID            string             `json:"policy_id"`
	TotalScenarios      int                `json:"total_scenarios"`
	SuccessRate         float64            `json:"success_rate"`
	MeanTokenReduction  float64            `json:"mean_token_reduction_pct"`
	MeanEvidenceRecall  float64            `json:"mean_evidence_recall"`
	TotalPrunedCount    int                `json:"total_pruned_count"`
	TotalSimDuration    time.Duration      `json:"total_sim_duration"`
	ScenariosPerSecond  float64            `json:"scenarios_per_second"`
	IndividualResults   []SimulationResult `json:"individual_results,omitempty"`
}

// ReplaySimulator executes offline dreaming evaluations over historical trajectories.
type ReplaySimulator struct {
	Scenarios []SimulationScenario
}

// NewReplaySimulator constructs a replay simulator pre-loaded with historical scenarios.
func NewReplaySimulator(scenarios []SimulationScenario) *ReplaySimulator {
	return &ReplaySimulator{
		Scenarios: scenarios,
	}
}

// Replay executes a counterfactual simulation of a task scenario under policy.
// It recalculates candidate marginal utilities, applies dynamic pruning,
// knapsack budget packing, and verifies evidence retention.
func (s *ReplaySimulator) Replay(sc SimulationScenario, p ExplorationPolicy) SimulationResult {
	start := time.Now()

	budget := sc.Budget
	if budget <= 0 {
		budget = 4000
	}

	type scoredCandidate struct {
		cand    model.Candidate
		score   float64
		density float64
	}

	var scored []scoredCandidate
	prunedCount := 0

	for _, c := range sc.Candidates {
		scScore, density := p.ScoreCandidate(
			c.Semantic, c.Lexical, c.TaskAffinity, c.Authority,
			c.Evidence, c.Reuse, c.Graph, c.StaleRisk, c.Tokens,
		)

		// Dynamic candidate pruning bound
		if density < p.PruneThreshold {
			prunedCount++
			continue
		}

		scored = append(scored, scoredCandidate{
			cand:    c,
			score:   scScore,
			density: density,
		})
	}

	// Sort descending by policy-evaluated marginal density
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].density > scored[j].density
	})

	// Greedy knapsack packing under budget
	var selected []model.Candidate
	currentTokens := 0

	for _, scItem := range scored {
		tok := scItem.cand.Tokens
		if currentTokens+tok <= budget {
			cand := scItem.cand
			cand.Score = scItem.score
			cand.Density = scItem.density
			selected = append(selected, cand)
			currentTokens += tok
		}
	}

	// Singleton Rescue knapsack safeguard: if a single high-score item fits the budget
	// and exceeds aggregate selected score, rescue it
	var bestSingle scoredCandidate
	for _, scItem := range scored {
		if scItem.cand.Tokens <= budget && scItem.score > bestSingle.score {
			bestSingle = scItem
		}
	}
	var aggregateScore float64
	for _, sel := range selected {
		aggregateScore += sel.Score
	}
	if bestSingle.score > aggregateScore && bestSingle.cand.Tokens > 0 {
		selected = []model.Candidate{bestSingle.cand}
		currentTokens = bestSingle.cand.Tokens
	}

	// Calculate evidence recall
	evidenceHit := 0
	if len(sc.RequiredEvidence) > 0 {
		selectedSet := make(map[string]bool)
		for _, sel := range selected {
			selectedSet[sel.ID] = true
			if sel.Location != "" {
				selectedSet[sel.Location] = true
			}
			for _, req := range sc.RequiredEvidence {
				if strings.Contains(sel.Content, req) || strings.Contains(sel.Location, req) {
					selectedSet[req] = true
				}
			}
		}
		for _, req := range sc.RequiredEvidence {
			if selectedSet[req] {
				evidenceHit++
			}
		}
	}

	evidenceRecall := 1.0
	if len(sc.RequiredEvidence) > 0 {
		evidenceRecall = float64(evidenceHit) / float64(len(sc.RequiredEvidence))
	}

	tokenReduction := 0.0
	if sc.BaselineTokens > 0 {
		tokenReduction = (1.0 - float64(currentTokens)/float64(sc.BaselineTokens)) * 100.0
	}

	predictedSuccess := evidenceRecall >= 1.0 && currentTokens <= budget

	return SimulationResult{
		ScenarioID:        sc.ID,
		PolicyID:          p.ID,
		SimulatedTokens:   currentTokens,
		TokenReductionPct: tokenReduction,
		EvidenceRecall:    evidenceRecall,
		PrunedCandidates:  prunedCount,
		PredictedSuccess:  predictedSuccess,
		ExecutionDuration: time.Since(start),
	}
}

// ReplayAll runs the full scenario corpus through policy and aggregates statistical telemetry.
func (s *ReplaySimulator) ReplayAll(p ExplorationPolicy) AggregateReplayReport {
	start := time.Now()
	var results []SimulationResult

	successCount := 0
	totalReduction := 0.0
	totalRecall := 0.0
	totalPruned := 0

	for _, sc := range s.Scenarios {
		res := s.Replay(sc, p)
		results = append(results, res)

		if res.PredictedSuccess {
			successCount++
		}
		totalReduction += res.TokenReductionPct
		totalRecall += res.EvidenceRecall
		totalPruned += res.PrunedCandidates
	}

	totalDur := time.Since(start)
	n := len(s.Scenarios)
	if n == 0 {
		return AggregateReplayReport{
			PolicyID: p.ID,
		}
	}

	qps := float64(n) / totalDur.Seconds()
	if totalDur.Seconds() == 0 {
		qps = float64(n) * 1e6
	}

	return AggregateReplayReport{
		PolicyID:           p.ID,
		TotalScenarios:     n,
		SuccessRate:        float64(successCount) / float64(n),
		MeanTokenReduction: totalReduction / float64(n),
		MeanEvidenceRecall: totalRecall / float64(n),
		TotalPrunedCount:   totalPruned,
		TotalSimDuration:   totalDur,
		ScenariosPerSecond: qps,
		IndividualResults:  results,
	}
}
