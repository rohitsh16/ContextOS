package router

import (
	"math"
	"strings"

	"contextos/internal/model"
)

// JointRouteRequest specifies task requirements and candidate context for joint optimization.
type JointRouteRequest struct {
	Task             string
	TaskValue        float64 // V (dollar value of success, defaults to 10.0)
	Candidates       []model.Candidate
	AvailableBudgets []int
	AvailableModels  []ModelProfile
	CacheHitRatio    float64
}

// JointRouteDecision encapsulates the selected (A*, C*) decision and economic metrics.
type JointRouteDecision struct {
	Strategy          string          `json:"strategy"`
	SelectedModel     ModelProfile    `json:"selected_model"`
	Budget            int             `json:"budget"`
	SelectedCount     int             `json:"selected_count"`
	SelectedTokens    int             `json:"selected_tokens"`
	SuccessProb       float64         `json:"success_prob"`
	EstimatedCost     float64         `json:"estimated_cost"`
	EstimatedLatency  float64         `json:"estimated_latency"`
	ExpectedValue     float64         `json:"expected_value"` // EV = P(success)*V - Cost - Latency
}

// DefaultBudgets returns standard budget search points.
func DefaultBudgets() []int {
	return []int{1000, 2500, 5000, 10000, 20000}
}

// EstimateSuccessProbability models:
// P(success | A, C, q) = min(0.99, Model.Quality * ContextCoverage * (1 - TaskComplexityDiscount))
func EstimateSuccessProbability(p ModelProfile, tokens int, maxUsefulTokens int, task string) float64 {
	if maxUsefulTokens <= 0 {
		maxUsefulTokens = 5000
	}

	// Context coverage follows diminishing returns
	coverage := 1.0 - math.Exp(-float64(tokens)/float64(maxUsefulTokens))
	if coverage < 0.1 {
		coverage = 0.1
	}

	// Task complexity analysis
	t := strings.ToLower(task)
	complexityDiscount := 0.0
	for _, kw := range []string{"distributed", "architecture", "security", "concurrency", "deadlock", "refactor"} {
		if strings.Contains(t, kw) {
			complexityDiscount += 0.05
		}
	}
	if complexityDiscount > 0.35 {
		complexityDiscount = 0.35
	}

	// Base model quality scaled by context and complexity
	prob := p.Quality * (0.4 + 0.6*coverage) * (1.0 - complexityDiscount)
	if prob > 0.99 {
		prob = 0.99
	}
	if prob < 0.05 {
		prob = 0.05
	}
	return prob
}

// EstimateCost calculates the dollar cost of prompt injection given model pricing and prompt-cache hit ratio.
func EstimateCost(p ModelProfile, tokens int, cacheHitRatio float64) float64 {
	cachedTokens := float64(tokens) * cacheHitRatio
	uncachedTokens := float64(tokens) * (1.0 - cacheHitRatio)

	cost := (uncachedTokens*p.InputPerM + cachedTokens*p.CachedInputPerM) / 1e6
	return cost
}

// EstimateLatency calculates expected latency in seconds.
func EstimateLatency(p ModelProfile, tokens int) float64 {
	baseLatency := 0.5
	if strings.EqualFold(p.Name, ModelLocal) {
		baseLatency = 0.1
	} else if strings.EqualFold(p.Name, ModelGPT53Codex) {
		baseLatency = 0.8
	}
	return baseLatency + float64(tokens)*0.00005
}

// OptimizeJointRoute implements PR-13:
// (A*, C*) = argmax_{A, C} P(success | A, C, q)*V - Cost(A, C) - Latency(A, C)
func OptimizeJointRoute(req JointRouteRequest) JointRouteDecision {
	if req.TaskValue <= 0 {
		req.TaskValue = 10.0
	}
	models := req.AvailableModels
	if len(models) == 0 {
		models = Profiles()
	}
	budgets := req.AvailableBudgets
	if len(budgets) == 0 {
		budgets = DefaultBudgets()
	}

	// Calculate total tokens available in candidate pool
	totalCandTokens := 0
	for _, c := range req.Candidates {
		totalCandTokens += c.Tokens
	}
	if totalCandTokens == 0 {
		totalCandTokens = 4000
	}

	bestEV := -1e9
	var bestDecision JointRouteDecision

	for _, m := range models {
		for _, b := range budgets {
			tokens := b
			if tokens > totalCandTokens {
				tokens = totalCandTokens
			}
			if tokens > m.ContextTokens {
				continue
			}

			prob := EstimateSuccessProbability(m, tokens, totalCandTokens, req.Task)
			cost := EstimateCost(m, tokens, req.CacheHitRatio)
			latency := EstimateLatency(m, tokens)

			// Latency penalty coefficient: $0.10 per second
			latencyCost := latency * 0.10

			ev := prob*req.TaskValue - cost - latencyCost

			if ev > bestEV {
				bestEV = ev
				bestDecision = JointRouteDecision{
					Strategy:         "joint",
					SelectedModel:    m,
					Budget:           b,
					SelectedTokens:   tokens,
					SuccessProb:      prob,
					EstimatedCost:    cost,
					EstimatedLatency: latency,
					ExpectedValue:    ev,
				}
			}
		}
	}

	return bestDecision
}

// CompareStrategies evaluates:
// 1. "fixed": default fixed model (Claude Sonnet) and fixed budget (5000)
// 2. "model-only": model recommendation based on task alone, with fixed budget (5000)
// 3. "context-only": fixed model (Claude Sonnet), optimizing budget only
// 4. "joint": full joint (A*, C*) optimization
func CompareStrategies(req JointRouteRequest) map[string]JointRouteDecision {
	if req.TaskValue <= 0 {
		req.TaskValue = 10.0
	}
	models := req.AvailableModels
	if len(models) == 0 {
		models = Profiles()
	}
	budgets := req.AvailableBudgets
	if len(budgets) == 0 {
		budgets = DefaultBudgets()
	}

	totalCandTokens := 0
	for _, c := range req.Candidates {
		totalCandTokens += c.Tokens
	}
	if totalCandTokens == 0 {
		totalCandTokens = 4000
	}

	results := make(map[string]JointRouteDecision)

	// 1. Fixed strategy: Claude Sonnet, 5000 budget
	fixedModel := GetProfile(ModelClaudeSonnet)
	fixedBudget := 5000
	fTokens := fixedBudget
	if fTokens > totalCandTokens {
		fTokens = totalCandTokens
	}
	fProb := EstimateSuccessProbability(fixedModel, fTokens, totalCandTokens, req.Task)
	fCost := EstimateCost(fixedModel, fTokens, req.CacheHitRatio)
	fLat := EstimateLatency(fixedModel, fTokens)
	results["fixed"] = JointRouteDecision{
		Strategy:         "fixed",
		SelectedModel:    fixedModel,
		Budget:           fixedBudget,
		SelectedTokens:   fTokens,
		SuccessProb:      fProb,
		EstimatedCost:    fCost,
		EstimatedLatency: fLat,
		ExpectedValue:    fProb*req.TaskValue - fCost - fLat*0.10,
	}

	// 2. Model-only routing: choose model via task complexity, fixed 5000 budget
	moModel := Recommend(req.Task, fixedBudget)
	moTokens := fixedBudget
	if moTokens > totalCandTokens {
		moTokens = totalCandTokens
	}
	moProb := EstimateSuccessProbability(moModel, moTokens, totalCandTokens, req.Task)
	moCost := EstimateCost(moModel, moTokens, req.CacheHitRatio)
	moLat := EstimateLatency(moModel, moTokens)
	results["model-only"] = JointRouteDecision{
		Strategy:         "model-only",
		SelectedModel:    moModel,
		Budget:           fixedBudget,
		SelectedTokens:   moTokens,
		SuccessProb:      moProb,
		EstimatedCost:    moCost,
		EstimatedLatency: moLat,
		ExpectedValue:    moProb*req.TaskValue - moCost - moLat*0.10,
	}

	// 3. Context-only routing: fixed model Claude Sonnet, optimize budget
	bestContextEV := -1e9
	var bestContextDecision JointRouteDecision
	for _, b := range budgets {
		tokens := b
		if tokens > totalCandTokens {
			tokens = totalCandTokens
		}
		prob := EstimateSuccessProbability(fixedModel, tokens, totalCandTokens, req.Task)
		cost := EstimateCost(fixedModel, tokens, req.CacheHitRatio)
		lat := EstimateLatency(fixedModel, tokens)
		ev := prob*req.TaskValue - cost - lat*0.10
		if ev > bestContextEV {
			bestContextEV = ev
			bestContextDecision = JointRouteDecision{
				Strategy:         "context-only",
				SelectedModel:    fixedModel,
				Budget:           b,
				SelectedTokens:   tokens,
				SuccessProb:      prob,
				EstimatedCost:    cost,
				EstimatedLatency: lat,
				ExpectedValue:    ev,
			}
		}
	}
	results["context-only"] = bestContextDecision

	// 4. Joint routing
	results["joint"] = OptimizeJointRoute(req)

	return results
}
