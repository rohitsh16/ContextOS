package report

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"contextos/internal/model"
	"contextos/internal/server"
)

// Summary holds high-level aggregate metrics for the benchmark report.
type Summary struct {
	TotalTraces          int     `json:"total_traces"`
	TotalBudgetTokens    int     `json:"total_budget_tokens"`
	TotalSelectedTokens  int     `json:"total_selected_tokens"`
	TokensSaved          int     `json:"tokens_saved"`
	SavingsPercentage    float64 `json:"savings_percentage"`
	CacheHitTraces       int     `json:"cache_hit_traces"`
	CacheHitRate         float64 `json:"cache_hit_rate"`
	TotalMemories        int     `json:"total_memories"`
	DecisionsCount       int     `json:"decisions_count"`
	FailuresCount        int     `json:"failures_count"`
	ConstraintsCount     int     `json:"constraints_count"`
	FactsCount           int     `json:"facts_count"`
	EstimatedCostSaved   float64 `json:"estimated_cost_saved"`
	RawDumpTokensEst     int     `json:"raw_dump_tokens_est"`
	RawDumpReductionRate float64 `json:"raw_dump_reduction_rate"`
}

// TraceSummary captures key fields for an individual allocation trace.
type TraceSummary struct {
	ID             int64   `json:"id"`
	Task           string  `json:"task"`
	Model          string  `json:"model"`
	Budget         int     `json:"budget"`
	SelectedTokens int     `json:"selected_tokens"`
	TokensSaved    int     `json:"tokens_saved"`
	SavingsPct     float64 `json:"savings_pct"`
	CacheHit       bool    `json:"cache_hit"`
	CreatedAt      string  `json:"created_at"`
}

// Report encapsulates a complete publication-ready benchmark analysis.
type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	RepoName    string         `json:"repo_name"`
	RepoPath    string         `json:"repo_path"`
	Revision    string         `json:"revision"`
	Branch      string         `json:"branch"`
	Summary     Summary        `json:"summary"`
	Traces      []TraceSummary `json:"traces"`
	Memories    []model.Memory `json:"memories"`
}

// Generate creates a comprehensive Report by querying the active service store.
func Generate(s *server.Service) (*Report, error) {
	traces, err := s.Store.ListTraces(s.RepoID, 1000)
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}

	mems, err := s.Store.ListMemories(s.RepoID, 1000)
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}

	var totalBudget, totalSelected, cacheHits int
	var traceSummaries []TraceSummary

	for _, t := range traces {
		saved := t.Budget - t.SelectedTokens
		if saved < 0 {
			saved = 0
		}
		var pct float64
		if t.Budget > 0 {
			pct = (float64(saved) / float64(t.Budget)) * 100.0
		}
		if t.CacheHit {
			cacheHits++
		}
		totalBudget += t.Budget
		totalSelected += t.SelectedTokens

		traceSummaries = append(traceSummaries, TraceSummary{
			ID:             t.ID,
			Task:           t.Task,
			Model:          t.Model,
			Budget:         t.Budget,
			SelectedTokens: t.SelectedTokens,
			TokensSaved:    saved,
			SavingsPct:     math.Round(pct*10) / 10,
			CacheHit:       t.CacheHit,
			CreatedAt:      t.CreatedAt,
		})
	}

	var decisions, failures, constraints, facts int
	for _, m := range mems {
		switch strings.ToLower(m.Kind) {
		case "decision":
			decisions++
		case "failure":
			failures++
		case "constraint":
			constraints++
		default:
			facts++
		}
	}

	netSaved := totalBudget - totalSelected
	if netSaved < 0 {
		netSaved = 0
	}
	var savingsPct, cacheHitRate float64
	if totalBudget > 0 {
		savingsPct = (float64(netSaved) / float64(totalBudget)) * 100.0
	}
	if len(traces) > 0 {
		cacheHitRate = (float64(cacheHits) / float64(len(traces))) * 100.0
	}

	rawDumpEst := len(traces) * 32000
	var rawReduction float64
	if rawDumpEst > 0 {
		rawReduction = (1.0 - (float64(totalSelected) / float64(rawDumpEst))) * 100.0
	}

	costSaved := (float64(netSaved) / 1000000.0) * 3.00

	rep := &Report{
		GeneratedAt: time.Now().UTC(),
		RepoName:    s.Repo.Name,
		RepoPath:    s.Repo.Path,
		Revision:    s.Repo.Revision,
		Branch:      s.Repo.Branch,
		Summary: Summary{
			TotalTraces:          len(traces),
			TotalBudgetTokens:    totalBudget,
			TotalSelectedTokens:  totalSelected,
			TokensSaved:          netSaved,
			SavingsPercentage:    math.Round(savingsPct*10) / 10,
			CacheHitTraces:       cacheHits,
			CacheHitRate:         math.Round(cacheHitRate*10) / 10,
			TotalMemories:        len(mems),
			DecisionsCount:       decisions,
			FailuresCount:        failures,
			ConstraintsCount:     constraints,
			FactsCount:           facts,
			EstimatedCostSaved:   math.Round(costSaved*1000) / 1000,
			RawDumpTokensEst:     rawDumpEst,
			RawDumpReductionRate: math.Round(rawReduction*10) / 10,
		},
		Traces:   traceSummaries,
		Memories: mems,
	}

	return rep, nil
}

// ToJSON formats the report as indented JSON.
func (r *Report) ToJSON() (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ToMarkdown generates an academic / publication-ready Markdown report.
func (r *Report) ToMarkdown() string {
	var sb strings.Builder

	shortRev := r.Revision
	if len(shortRev) > 10 {
		shortRev = shortRev[:10]
	}

	sb.WriteString("# ContextOS Empirical Evaluation & Benchmark Report\n\n")
	sb.WriteString(fmt.Sprintf("**Repository:** `%s` (`%s` @ `%s`)  \n", r.RepoName, r.Branch, shortRev))
	sb.WriteString(fmt.Sprintf("**Generated At:** `%s UTC`  \n", r.GeneratedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString("**Evaluation Framework:** ContextOS ASC-1 6-Pass Context Runtime  \n\n")

	sb.WriteString("### Badges\n")
	sb.WriteString(fmt.Sprintf("[![ContextOS Token Savings](https://img.shields.io/badge/Token_Pruned-%.1f%%25-brightgreen.svg)](#)\n", r.Summary.SavingsPercentage))
	sb.WriteString(fmt.Sprintf("[![KV Cache Hit Rate](https://img.shields.io/badge/KV--Cache_Hits-%.1f%%25-blue.svg)](#)\n", r.Summary.CacheHitRate))
	sb.WriteString(fmt.Sprintf("[![Raw Context Reduction](https://img.shields.io/badge/Raw_Reduction-%.1f%%25-orange.svg)](#)\n\n", r.Summary.RawDumpReductionRate))

	sb.WriteString("---\n\n")
	sb.WriteString("## 1. Executive Summary\n\n")
	sb.WriteString("| Metric | Measured Result | Significance / Baseline |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Total Real-World Allocations** | **%d traces** | Live agent turns and IDE invocations |\n", r.Summary.TotalTraces))
	sb.WriteString(fmt.Sprintf("| **Total Requested Budget Ceiling** | **%s tokens** | Upper token constraints set by developers/tools |\n", formatNumber(r.Summary.TotalBudgetTokens)))
	sb.WriteString(fmt.Sprintf("| **Actual Context Selected & Injected** | **%s tokens** | Minimum-sufficient packed context |\n", formatNumber(r.Summary.TotalSelectedTokens)))
	sb.WriteString(fmt.Sprintf("| **Net Tokens Pruned / Saved** | **%s tokens (%.1f%%)** | **Immediate token reduction vs. budget ceiling** |\n", formatNumber(r.Summary.TokensSaved), r.Summary.SavingsPercentage))
	sb.WriteString(fmt.Sprintf("| **KV-Cache Hit Rate** | **%d / %d (%.1f%%)** | **Byte-identical Stable Prefix reuse** (75–90%% provider discount) |\n", r.Summary.CacheHitTraces, r.Summary.TotalTraces, r.Summary.CacheHitRate))
	sb.WriteString(fmt.Sprintf("| **Reduction vs. Raw Repository Dumps** | **%.1f%% reduction** | vs. ~32k raw uncompressed dumps per turn |\n", r.Summary.RawDumpReductionRate))
	sb.WriteString(fmt.Sprintf("| **Estimated Cost Reduction** | **$%.3f** | Based on standard $3.00 / 1M token input pricing |\n\n", r.Summary.EstimatedCostSaved))

	sb.WriteString("---\n\n")
	sb.WriteString("## 2. Engineering Knowledge & Decision Inventory\n\n")
	sb.WriteString(fmt.Sprintf("ContextOS manages **%d total durable memories** across the codebase:\n\n", r.Summary.TotalMemories))
	sb.WriteString(fmt.Sprintf("- **Architectural Decisions (`decision`):** %d (durable architectural choices tagged to git revisions)\n", r.Summary.DecisionsCount))
	sb.WriteString(fmt.Sprintf("- **Negative Knowledge (`failure`):** %d (documented dead ends preventing repeating errors)\n", r.Summary.FailuresCount))
	sb.WriteString(fmt.Sprintf("- **Engineering Constraints (`constraint`):** %d (KV cache ordering and budget limits)\n", r.Summary.ConstraintsCount))
	sb.WriteString(fmt.Sprintf("- **General Facts (`fact`):** %d\n\n", r.Summary.FactsCount))

	sb.WriteString("---\n\n")
	sb.WriteString("## 3. Allocation Trace History\n\n")
	sb.WriteString("| # | Task / Objective | Model | Budget | Injected | Saved | KV-Cache | Timestamp |\n")
	sb.WriteString("| :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: |\n")

	for _, t := range r.Traces {
		cacheTag := "MISS"
		if t.CacheHit {
			cacheTag = "**HIT**"
		}
		modelName := t.Model
		if modelName == "" {
			modelName = "auto"
		}
		taskClean := strings.ReplaceAll(t.Task, "|", "/")
		if len(taskClean) > 40 {
			taskClean = taskClean[:37] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | `%s` | %d | %d | **+%d (%.1f%%)** | %s | %s |\n",
			t.ID, taskClean, modelName, t.Budget, t.SelectedTokens, t.TokensSaved, t.SavingsPct, cacheTag, t.CreatedAt))
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("## 4. Methodology & Optimization Architecture\n\n")
	sb.WriteString("ContextOS uses the **ASC-1 6-Pass Algorithmic Pipeline**:\n\n")
	sb.WriteString("1. **BM25 Lexical Retrieval**: Identifies relevant engineering symbols and memories matching lexical tokens.\n")
	sb.WriteString("2. **HashSemantic Cosine Similarity**: Matches concepts across varied wording without external model latency.\n")
	sb.WriteString("3. **Reciprocal Rank Fusion & Authority Scoring**: Combines lexical and semantic ranks with user authority multipliers.\n")
	sb.WriteString("4. **Knapsack Density Packing**: Selects highest utility per token until budget boundary.\n")
	sb.WriteString("5. **(1 - 1/e) Sviridenko Singleton Rescue**: Rescues high-value monolithic decisions from being starved by small items.\n")
	sb.WriteString("6. **KV-Cache Alignment Partition**: Enforces byte-identical ordering on the Stable Prefix for Claude/Gemini cache hits.\n\n")
	sb.WriteString("*Report generated autonomously by ContextOS.*  \n")

	return sb.String()
}

func formatNumber(n int) string {
	in := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(in)+len(in)/3)
	for i, c := range in {
		if i > 0 && (len(in)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}
