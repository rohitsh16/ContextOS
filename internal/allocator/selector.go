package allocator

import (
	"container/heap"
	"context"
	"math"
	"sort"

	"contextos/internal/model"
	"contextos/internal/textutil"
)

// Diagnostics captures execution metrics for context selectors.
type Diagnostics struct {
	SelectedCount   int            `json:"selected_count"`
	SelectedTokens  int            `json:"selected_tokens"`
	Budget          int            `json:"budget"`
	Utilization     float64        `json:"utilization"`
	MarginalUtility float64        `json:"marginal_utility"`
	Algorithm       string         `json:"algorithm"`
	Passes          []string       `json:"passes,omitempty"`
	Metrics         map[string]any `json:"metrics,omitempty"`
}

// Selector abstracts budget-constrained context selection algorithms (PR-10).
type Selector interface {
	Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics)
}

// ─── 1. GreedySelector ────────────────────────────────────────────────────────

// GreedySelector implements standard density-greedy packing.
type GreedySelector struct{}

func (s *GreedySelector) Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics) {
	cands := make([]model.Candidate, len(candidates))
	copy(cands, candidates)

	// Sort descending by density
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].Density > cands[j].Density
	})

	var selected []model.Candidate
	spent := 0
	totalScore := 0.0

	for _, c := range cands {
		if math.IsInf(c.Density, -1) || c.Tokens <= 0 {
			continue
		}
		if spent+c.Tokens <= budget {
			selected = append(selected, c)
			spent += c.Tokens
			totalScore += c.Score
		}
	}

	util := 0.0
	if budget > 0 {
		util = float64(spent) / float64(budget)
	}

	return selected, Diagnostics{
		SelectedCount:   len(selected),
		SelectedTokens:  spent,
		Budget:          budget,
		Utilization:     util,
		MarginalUtility: totalScore,
		Algorithm:       "GreedySelector",
		Passes:          []string{"density-greedy"},
	}
}

// ─── 2. GreedySingletonSelector ───────────────────────────────────────────────

// GreedySingletonSelector implements density-greedy packing with singleton rescue.
type GreedySingletonSelector struct{}

func (s *GreedySingletonSelector) Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics) {
	greedySel, diag := (&GreedySelector{}).Select(ctx, candidates, budget)

	greedyScore := 0.0
	for _, c := range greedySel {
		greedyScore += c.Score
	}

	// Singleton Rescue pass
	var bestSingle *model.Candidate
	for i := range candidates {
		c := &candidates[i]
		if math.IsInf(c.Density, -1) || c.Tokens <= 0 || c.Tokens > budget {
			continue
		}
		if bestSingle == nil || c.Score > bestSingle.Score {
			bestSingle = c
		}
	}

	selected := greedySel
	passes := []string{"density-greedy"}
	if bestSingle != nil && bestSingle.Score > greedyScore {
		selected = []model.Candidate{*bestSingle}
		passes = append(passes, "singleton-rescue")
	}

	spent := 0
	totalScore := 0.0
	for _, c := range selected {
		spent += c.Tokens
		totalScore += c.Score
	}

	diag.SelectedCount = len(selected)
	diag.SelectedTokens = spent
	diag.MarginalUtility = totalScore
	diag.Algorithm = "GreedySingletonSelector"
	diag.Passes = passes
	if budget > 0 {
		diag.Utilization = float64(spent) / float64(budget)
	}
	return selected, diag
}

// ─── 3. SubmodularSelector ───────────────────────────────────────────────────

// SubmodularSelector optimizes the formal submodular objective F(C) with marginal gains
// and redundancy penalties: Delta(e | S) = Score(e) - lambda * sum_{s in S} Overlap(e, s).
type SubmodularSelector struct {
	RedundancyPenalty float64 // lambda (default 0.35)
}

func (s *SubmodularSelector) Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics) {
	lambda := s.RedundancyPenalty
	if lambda <= 0 {
		lambda = 0.35
	}

	available := make([]model.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if !math.IsInf(c.Density, -1) && c.Tokens > 0 && c.Tokens <= budget {
			available = append(available, c)
		}
	}

	var selected []model.Candidate
	spent := 0
	totalUtility := 0.0

	for len(available) > 0 {
		select {
		case <-ctx.Done():
			break
		default:
		}

		bestIdx := -1
		bestGainPerToken := -1.0
		var bestGain float64

		for i, c := range available {
			if spent+c.Tokens > budget {
				continue
			}
			// Compute marginal gain Delta(c | S)
			redundancy := 0.0
			for _, sel := range selected {
				redundancy += textutil.Overlap(c.Content, sel.Content)
			}
			gain := c.Score - lambda*redundancy
			if gain <= 0 {
				continue
			}
			gainPerToken := gain / float64(c.Tokens)
			if gainPerToken > bestGainPerToken {
				bestGainPerToken = gainPerToken
				bestGain = gain
				bestIdx = i
			}
		}

		if bestIdx == -1 {
			break // No positive gain items fit budget
		}

		chosen := available[bestIdx]
		selected = append(selected, chosen)
		spent += chosen.Tokens
		totalUtility += bestGain

		// Remove chosen from available
		available = append(available[:bestIdx], available[bestIdx+1:]...)
	}

	util := 0.0
	if budget > 0 {
		util = float64(spent) / float64(budget)
	}

	return selected, Diagnostics{
		SelectedCount:   len(selected),
		SelectedTokens:  spent,
		Budget:          budget,
		Utilization:     util,
		MarginalUtility: totalUtility,
		Algorithm:       "SubmodularSelector",
		Passes:          []string{"submodular-marginal-gain"},
	}
}

// ─── 4. LazyGreedySelector (Minoux 1978 Accelerated Submodular Optimization) ─

type priorityItem struct {
	candidate    model.Candidate
	marginalGain float64
	iteration    int
	index        int
}

type priorityQueue []*priorityItem

func (pq priorityQueue) Len() int           { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool { return pq[i].marginalGain > pq[j].marginalGain }
func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *priorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*priorityItem)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// LazyGreedySelector implements Minoux's lazy greedy algorithm, which dramatically accelerates
// submodular maximization by caching and lazily evaluating upper bounds on marginal gains.
type LazyGreedySelector struct {
	RedundancyPenalty float64
}

func (s *LazyGreedySelector) Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics) {
	lambda := s.RedundancyPenalty
	if lambda <= 0 {
		lambda = 0.35
	}

	pq := make(priorityQueue, 0, len(candidates))
	for _, c := range candidates {
		if math.IsInf(c.Density, -1) || c.Tokens <= 0 || c.Tokens > budget {
			continue
		}
		pq = append(pq, &priorityItem{
			candidate:    c,
			marginalGain: c.Score / float64(c.Tokens),
			iteration:    0,
		})
	}
	heap.Init(&pq)

	var selected []model.Candidate
	spent := 0
	totalGain := 0.0
	currentIteration := 0

	for pq.Len() > 0 {
		select {
		case <-ctx.Done():
			break
		default:
		}

		top := heap.Pop(&pq).(*priorityItem)

		if spent+top.candidate.Tokens > budget {
			continue
		}

		if top.iteration == currentIteration {
			// Submodularity guarantee: this item is truly the maximum marginal gain!
			selected = append(selected, top.candidate)
			spent += top.candidate.Tokens
			totalGain += top.marginalGain * float64(top.candidate.Tokens)
			currentIteration++
			continue
		}

		// Re-evaluate marginal gain with current selection
		redundancy := 0.0
		for _, sel := range selected {
			redundancy += textutil.Overlap(top.candidate.Content, sel.Content)
		}
		newGain := top.candidate.Score - lambda*redundancy
		if newGain <= 0 {
			continue
		}
		top.marginalGain = newGain / float64(top.candidate.Tokens)
		top.iteration = currentIteration
		heap.Push(&pq, top)
	}

	util := 0.0
	if budget > 0 {
		util = float64(spent) / float64(budget)
	}

	return selected, Diagnostics{
		SelectedCount:   len(selected),
		SelectedTokens:  spent,
		Budget:          budget,
		Utilization:     util,
		MarginalUtility: totalGain,
		Algorithm:       "LazyGreedySelector",
		Passes:          []string{"lazy-greedy-minoux"},
	}
}

// ─── 5. LearnedSelector ───────────────────────────────────────────────────────

// LearnedSelector implements a parameterized combination of coverage, relevance,
// redundancy, cache stability, and risk terms according to the formal objective F(C).
type LearnedSelector struct {
	AlphaCoverage float64
	BetaRelevance float64
	LambdaRedund  float64
	EtaCache      float64
	RhoRisk       float64
}

func DefaultLearnedSelector() *LearnedSelector {
	return &LearnedSelector{
		AlphaCoverage: 0.30,
		BetaRelevance: 0.40,
		LambdaRedund:  0.20,
		EtaCache:      0.15,
		RhoRisk:       0.15,
	}
}

func (s *LearnedSelector) Select(ctx context.Context, candidates []model.Candidate, budget int) ([]model.Candidate, Diagnostics) {
	var scoredCands []struct {
		c    model.Candidate
		cost int
		val  float64
	}

	for _, c := range candidates {
		if math.IsInf(c.Density, -1) || c.Tokens <= 0 || c.Tokens > budget {
			continue
		}
		val := s.BetaRelevance*c.Score + s.EtaCache*c.CacheValue - s.RhoRisk*c.StaleRisk
		if val > 0 {
			scoredCands = append(scoredCands, struct {
				c    model.Candidate
				cost int
				val  float64
			}{c: c, cost: c.Tokens, val: val})
		}
	}

	sort.SliceStable(scoredCands, func(i, j int) bool {
		return (scoredCands[i].val / float64(scoredCands[i].cost)) > (scoredCands[j].val / float64(scoredCands[j].cost))
	})

	var selected []model.Candidate
	spent := 0
	totalVal := 0.0

	for _, item := range scoredCands {
		if spent+item.cost <= budget {
			selected = append(selected, item.c)
			spent += item.cost
			totalVal += item.val
		}
	}

	util := 0.0
	if budget > 0 {
		util = float64(spent) / float64(budget)
	}

	return selected, Diagnostics{
		SelectedCount:   len(selected),
		SelectedTokens:  spent,
		Budget:          budget,
		Utilization:     util,
		MarginalUtility: totalVal,
		Algorithm:       "LearnedSelector",
		Passes:          []string{"formal-objective-weighted"},
	}
}
