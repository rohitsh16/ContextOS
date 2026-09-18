package allocator

import (
	"math"
	"sort"
	"strings"

	"contextos/internal/model"
	"contextos/internal/textutil"
)

type Request struct {
	Task         string
	Budget       int
	Model        string
	RepoRevision string
}

func authorityScore(v string) float64 {
	switch strings.ToLower(v) {
	case "user", "explicit":
		return 1.0
	case "test":
		return 0.99
	case "source":
		return 0.97
	case "commit":
		return 0.94
	case "doc":
		return 0.86
	case "inference":
		return 0.55
	default:
		return 0.45
	}
}

func stale(m model.Memory, currentRevision string) (float64, bool) {
	if m.InvalidatedAtRevision != "" {
		return 1, true
	}
	if m.ValidFromRevision != "" && currentRevision != "" && m.ValidFromRevision != currentRevision {
		// Validity intervals are only fully known when explicit invalidation exists.
		// Treat a different revision as a freshness penalty, not an automatic rejection.
		return 0.25, false
	}
	return 0, false
}

func kindBoost(kind string) float64 {
	switch strings.ToLower(kind) {
	case "decision":
		return 1.0
	case "failure":
		return 0.98
	case "constraint":
		return 0.96
	case "state":
		return 0.88
	case "observation":
		return 0.76
	case "code":
		return 0.74
	case "fact":
		return 0.65
	default:
		return 0.5
	}
}

func Score(req Request, m model.Memory) model.Candidate {
	tok := m.TokenCost
	if tok <= 0 {
		tok = textutil.EstimateTokens(m.Content)
	}
	lex := textutil.Overlap(req.Task, m.Content)
	sem := textutil.HashSemantic(req.Task, m.Content)
	freshRisk, hardStale := stale(m, req.RepoRevision)
	auth := authorityScore(m.Authority)
	reuse := math.Min(1, float64(m.ReuseCount)/10.0)
	affinity := 0.6*lex + 0.4*kindBoost(m.Kind)
	graph := 0.0
	if strings.Contains(strings.ToLower(m.Content), "file") || strings.Contains(strings.ToLower(m.Content), "/") || m.Kind == "code" {
		graph = 0.6
	}
	evidence := 0.0
	if m.Source != "" || m.Location != "" {
		evidence = 1.0
	} else if auth >= 0.94 {
		evidence = 0.75
	}
	cacheValue := reuse*0.8 + 0.2*math.Min(1, float64(tok)/8000.0)
	score := 1.15*sem + 1.0*lex + 0.9*affinity + 0.75*auth + 0.6*evidence + 0.45*reuse + 0.35*graph - 1.5*freshRisk - 0.15*math.Min(1, float64(tok)/4000.0)
	density := score / float64(max(1, tok))
	src := m.Source
	if src == "" {
		src = "memory"
	}
	if strings.HasPrefix(m.ID, "node:") {
		src = "repository"
	}
	reason := "candidate"
	if hardStale {
		reason = "rejected: explicitly invalidated"
	}
	if auth < 0.6 {
		reason = "rejected: low authority"
	}
	if hardStale || auth < 0.6 {
		density = math.Inf(-1)
	}
	return model.Candidate{Source: src, Location: m.Location, ID: m.ID, Kind: m.Kind, Content: m.Content, Tokens: tok, Semantic: sem, Lexical: lex, Graph: graph, Freshness: 1 - freshRisk, Authority: auth, Reuse: reuse, TaskAffinity: affinity, Evidence: evidence, StaleRisk: freshRisk, CacheValue: cacheValue, MarginalEstimate: score, Density: density, Score: score, Reason: reason}
}

func Plan(req Request, ms []model.Memory) model.ContextPlan {
	if req.Budget <= 0 {
		req.Budget = 4000
	}
	cands := make([]model.Candidate, 0, len(ms))
	for _, m := range ms {
		cands = append(cands, Score(req, m))
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Density > cands[j].Density })
	used := 0
	selected := make([]model.Candidate, 0)
	for i := range cands {
		if strings.HasPrefix(cands[i].Reason, "rejected:") {
			continue
		}
		if used+cands[i].Tokens <= req.Budget {
			cands[i].Selected = true
			cands[i].Reason = "selected: marginal-utility/token greedy"
			selected = append(selected, cands[i])
			used += cands[i].Tokens
		} else {
			cands[i].Reason = "rejected: token budget"
		}
	}
	// Stable prefix is intentionally dominated by repository facts and canonical decisions;
	// volatile state is appended after it so providers can reuse the prefix cache.
	sort.SliceStable(selected, func(i, j int) bool {
		stable := func(c model.Candidate) int {
			if c.Kind == "decision" || c.Kind == "constraint" || c.Kind == "code" || c.Kind == "fact" {
				return 0
			}
			return 1
		}
		return stable(selected[i]) < stable(selected[j])
	})
	var prefix, variable []model.Candidate
	for _, c := range selected {
		if c.Kind == "decision" || c.Kind == "constraint" || c.Kind == "code" || c.Kind == "fact" {
			prefix = append(prefix, c)
		} else {
			variable = append(variable, c)
		}
	}
	return model.ContextPlan{Task: req.Task, Budget: req.Budget, SelectedTokens: used, StablePrefix: prefix, VariableContext: variable, Candidates: cands, Selected: selected}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
