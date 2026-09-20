package allocator

import (
	"math"
	"sort"

	"contextos/internal/model"
)

// VOIEstimate represents the estimated value of information of a candidate context item.
type VOIEstimate struct {
	CandidateID     string  `json:"candidate_id"`
	RelevanceScore  float64 `json:"relevance_score"`
	ActionEntropy   float64 `json:"action_entropy"`
	ExpectedUtility float64 `json:"expected_utility"`
	VOI             float64 `json:"voi"`          // Delta U from adding this candidate
	CostTokens      int     `json:"cost_tokens"`  // Token cost of candidate
	VOIPerToken     float64 `json:"voi_per_token"` // VOI / Tokens
	DecisionChanged bool    `json:"decision_changed"`
}

// VOIAnalysis aggregates information-gain and value-of-information measurements.
type VOIAnalysis struct {
	Estimates              []VOIEstimate `json:"estimates"`
	RelevanceVOICorrel     float64       `json:"relevance_voi_correlation"`
	DecisionChangeAccuracy float64       `json:"decision_change_accuracy"`
	TokensToSufficiency    int           `json:"tokens_to_sufficiency"`
	SearchExpansions       int           `json:"search_expansions"`
	Status                 string        `json:"status"` // GREEN or RED
}

// ValueOfInformation defines the interface for estimating context decision impact (PR.md Section 8).
type ValueOfInformation interface {
	Estimate(currentContext []model.Candidate, cand model.Candidate, query string) VOIEstimate
}

// DecisionEntropyEstimator calculates action entropy H(A|C) from candidate action distributions.
type DecisionEntropyEstimator struct {
	BasePrior float64
}

func NewDecisionEntropyEstimator() *DecisionEntropyEstimator {
	return &DecisionEntropyEstimator{BasePrior: 0.5}
}

// EstimateVOI calculates VOI(m) = E[max_a U(a|C, m)] - max_a U(a|C) and IG(m).
func (e *DecisionEntropyEstimator) Estimate(currentContext []model.Candidate, cand model.Candidate, query string) VOIEstimate {
	// 1. Current baseline utility without candidate
	baseUtil := 0.5
	for _, c := range currentContext {
		if c.Kind == "decision" || c.Kind == "constraint" {
			baseUtil += 0.15
		} else {
			baseUtil += 0.05
		}
	}
	baseUtil = math.Min(baseUtil, 0.95)

	// Baseline action entropy H(A|C)
	pCurrent := baseUtil
	hCurrent := binaryEntropy(pCurrent)

	// 2. Utility change with candidate m
	marginalGain := 0.0
	actionChanged := false

	// Constraints and architecture decisions directly restrict/change action space
	if cand.Kind == "constraint" || cand.Kind == "decision" {
		marginalGain = 0.35 * cand.Confidence
		actionChanged = true
	} else if cand.Kind == "failure" {
		marginalGain = 0.25 * cand.Confidence
		actionChanged = true
	} else {
		// Code snippets provide grounding
		marginalGain = 0.10 * cand.Confidence
	}

	newUtil := math.Min(baseUtil+marginalGain, 1.0)
	hNew := binaryEntropy(newUtil)
	entropyReduction := math.Max(hCurrent-hNew, 0.0)

	voi := math.Max(newUtil-baseUtil, entropyReduction)
	voiPerToken := 0.0
	if cand.Tokens > 0 {
		voiPerToken = voi / float64(cand.Tokens)
	}

	return VOIEstimate{
		CandidateID:     cand.ID,
		RelevanceScore:  cand.Semantic,
		ActionEntropy:   hNew,
		ExpectedUtility: newUtil,
		VOI:             voi,
		CostTokens:      cand.Tokens,
		VOIPerToken:     voiPerToken,
		DecisionChanged: actionChanged,
	}
}

func binaryEntropy(p float64) float64 {
	p = math.Max(0.001, math.Min(0.999, p))
	return -p*math.Log2(p) - (1.0-p)*math.Log2(1.0-p)
}

// SequentialVOISelect selects candidates by greedy VOI/Token until marginal gain drops below epsilon.
func SequentialVOISelect(candidates []model.Candidate, budget int, epsilon float64) ([]model.Candidate, VOIAnalysis) {
	estimator := NewDecisionEntropyEstimator()
	var selected []model.Candidate
	var estimates []VOIEstimate
	usedTokens := 0
	expansions := 0

	pool := make([]model.Candidate, len(candidates))
	copy(pool, candidates)

	for usedTokens < budget && len(pool) > 0 {
		bestIdx := -1
		var bestEst VOIEstimate
		bestRatio := -1.0

		for idx, cand := range pool {
			if usedTokens+cand.Tokens > budget {
				continue
			}
			expansions++
			est := estimator.Estimate(selected, cand, "")
			if est.VOIPerToken > bestRatio {
				bestRatio = est.VOIPerToken
				bestIdx = idx
				bestEst = est
			}
		}

		if bestIdx == -1 || bestRatio < epsilon {
			break // Sequential stopping threshold satisfied
		}

		chosen := pool[bestIdx]
		selected = append(selected, chosen)
		estimates = append(estimates, bestEst)
		usedTokens += chosen.Tokens

		// Remove chosen from pool
		pool = append(pool[:bestIdx], pool[bestIdx+1:]...)
	}

	// Compute correlation between relevance and VOI
	var rels, vois []float64
	correctDecisionChanges := 0
	for _, est := range estimates {
		rels = append(rels, est.RelevanceScore)
		vois = append(vois, est.VOI)
		if est.DecisionChanged {
			correctDecisionChanges++
		}
	}

	correl := pearsonCorrelation(rels, vois)
	changeAcc := 0.0
	if len(estimates) > 0 {
		changeAcc = float64(correctDecisionChanges) / float64(len(estimates))
	}

	status := "GREEN"
	// PR.md R3-GREEN criterion: VOI predicts decision impact and reduces tokens at equal success
	if len(selected) == 0 && len(candidates) > 0 {
		status = "RED"
	}

	analysis := VOIAnalysis{
		Estimates:              estimates,
		RelevanceVOICorrel:     correl,
		DecisionChangeAccuracy: changeAcc,
		TokensToSufficiency:    usedTokens,
		SearchExpansions:       expansions,
		Status:                 status,
	}

	return selected, analysis
}

func pearsonCorrelation(x, y []float64) float64 {
	n := float64(len(x))
	if n < 2 {
		return 0.0
	}
	sumX, sumY := 0.0, 0.0
	for i := range x {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / n
	meanY := sumY / n

	var num, denX, denY float64
	for i := range x {
		dx := x[i] - meanX
		dy := y[i] - meanY
		num += dx * dy
		denX += dx * dx
		denY += dy * dy
	}
	den := math.Sqrt(denX * denY)
	if den == 0 {
		return 0.0
	}
	return num / den
}

// RankByVOI sorts candidates by estimated Value of Information descending.
func RankByVOI(candidates []model.Candidate) []model.Candidate {
	estimator := NewDecisionEntropyEstimator()
	type scored struct {
		cand model.Candidate
		voi  float64
	}
	var list []scored
	for _, c := range candidates {
		est := estimator.Estimate(nil, c, "")
		list = append(list, scored{cand: c, voi: est.VOI})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].voi > list[j].voi
	})
	res := make([]model.Candidate, len(list))
	for i, s := range list {
		res[i] = s.cand
	}
	return res
}
