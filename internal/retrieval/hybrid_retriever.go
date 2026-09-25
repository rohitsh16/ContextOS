package retrieval

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/store"
	"contextos/internal/textutil"
)

// HybridRetriever orchestrates multi-channel candidate generation, admission filtering,
// graph expansion, and task-aware reranking (R18.1 §10, §11, §12, §14, §15).
type HybridRetriever struct {
	Store           store.Store
	RepoID          string
	AdmissionPolicy gitidx.AdmissionPolicy
}

// NewHybridRetriever instantiates a hybrid multi-channel retriever.
func NewHybridRetriever(st store.Store, repoID string, policy gitidx.AdmissionPolicy) *HybridRetriever {
	return &HybridRetriever{
		Store:           st,
		RepoID:          repoID,
		AdmissionPolicy: policy,
	}
}

// Retrieve implements the standard Retriever interface.
func (r *HybridRetriever) Retrieve(ctx context.Context, q Query) ([]Candidate, RetrievalTrace, error) {
	candidates, qTrace, err := r.RetrieveWithDetailedTrace(ctx, q, r.AdmissionPolicy)
	legacyTrace := RetrievalTrace{
		QueryID:         qTrace.QueryID,
		RepoID:          q.RepoID,
		Revision:        q.Revision,
		QueryClass:      string(qTrace.RetrievalStages[0]),
		FinalCandidates: len(candidates),
		ScopedNodes:     qTrace.CandidateCount,
	}
	return candidates, legacyTrace, err
}

// RetrieveWithDetailedTrace executes the full 5-channel pipeline and produces a detailed QueryRetrievalTrace.
func (r *HybridRetriever) RetrieveWithDetailedTrace(
	ctx context.Context,
	q Query,
	policy gitidx.AdmissionPolicy,
) ([]Candidate, *QueryRetrievalTrace, error) {
	start := time.Now()
	queryID := fmt.Sprintf("q_hyb_%d", time.Now().UnixNano())

	qTrace := &QueryRetrievalTrace{
		QueryID:             queryID,
		Query:               q.Task,
		TopCandidates:       make([]CandidateTrace, 0),
		RetrievalStages:     make([]RetrievalStage, 0),
		AdmissionRejections: make([]string, 0),
		ExpansionNodes:      make([]string, 0),
		FinalRankedResults:  make([]CandidateTrace, 0),
	}

	limit := q.MaxResults
	if limit <= 0 {
		limit = 50
	}

	// 1. Structured Query Decomposition (R18.1 §8)
	rep := DecomposeQueryRepresentation(q.Task)
	ext := ExpandQueryTerms(rep)
	profile := GetRoutingProfile(rep.Intent)

	var rawCandidates []Candidate
	addRaw := func(c Candidate, stage RetrievalStage, score float64) {
		// Initialize trace
		if c.Trace == nil {
			c.Trace = &CandidateTrace{
				ID:         c.ID,
				Path:       c.Path,
				Name:       c.Name,
				Stages:     []RetrievalStage{stage},
				Admissible: true,
			}
		} else {
			c.Trace.AddStage(stage)
		}
		switch stage {
		case StageExactPath, StageBasename:
			c.PathScore = score
			c.Trace.PathScore = score
		case StageSymbol:
			c.Trace.SymbolScore = score
		case StageEntity:
			c.EntityScore = score
			c.Trace.EntityScore = score
		case StageLexical:
			c.LexicalScore = score
			c.Trace.LexicalScore = score
		case StageSemantic:
			c.SemanticScore = score
			c.Trace.SemanticScore = score
		}
		c.Score = score
		rawCandidates = append(rawCandidates, c)
	}

	// 2. Channel 1: Exact Path and Basename (R18.1 §10)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageExactPath)
	for _, p := range rep.Paths {
		nodes, err := r.Store.LookupExactPath(q.RepoID, p)
		if err == nil {
			for _, n := range nodes {
				c := nodeToCandidate(n, string(StageExactPath))
				addRaw(c, StageExactPath, 1.0)
			}
		}
		baseNodes, err := r.Store.LookupBasename(q.RepoID, filepath.Base(p))
		if err == nil {
			for _, n := range baseNodes {
				c := nodeToCandidate(n, string(StageBasename))
				addRaw(c, StageBasename, 0.95)
			}
		}
	}

	// 3. Channel 2: Symbol Channel (R18.1 §10)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageSymbol)
	for _, sym := range rep.Symbols {
		nodes, err := r.Store.LookupQualifiedSymbol(q.RepoID, sym)
		if err == nil {
			for _, n := range nodes {
				c := nodeToCandidate(n, string(StageSymbol))
				addRaw(c, StageSymbol, 0.90)
			}
		}
		symNodes, err := r.Store.LookupSymbol(q.RepoID, sym)
		if err == nil {
			for _, n := range symNodes {
				c := nodeToCandidate(n, string(StageSymbol))
				addRaw(c, StageSymbol, 0.85)
			}
		}
	}

	// 4. Channel 3: Entity Channel (Domain acronyms, components e.g. DRMC, bunker, GCP, shared-vpc)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageEntity)
	for _, ent := range rep.Entities {
		// Try entity as basename or directory
		entNodes, err := r.Store.LookupPath(q.RepoID, ent.Name)
		if err == nil {
			for _, n := range entNodes {
				c := nodeToCandidate(n, string(StageEntity))
				addRaw(c, StageEntity, 0.80*ent.Salience)
			}
		}
		// Search code candidates specifically for the high-salience entity
		entSearch, err := r.Store.SearchCodeCandidates(q.RepoID, ent.Name, q.Scope, 20)
		if err == nil {
			for _, n := range entSearch {
				c := nodeToCandidate(n, string(StageEntity))
				addRaw(c, StageEntity, 0.75*ent.Salience)
			}
		}
	}

	// 5. Channel 4: Lexical Channel with Bounded Expansion (R18.1 §9 & §10)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageLexical)
	searchTokens := ext.AllSearchTokens
	if len(searchTokens) > 0 {
		joinedTokens := strings.Join(searchTokens, " ")
		lexNodes, err := r.Store.SearchCodeCandidates(q.RepoID, joinedTokens, q.Scope, limit*2)
		if err == nil {
			for _, n := range lexNodes {
				c := nodeToCandidate(n, string(StageLexical))
				content := fmt.Sprintf("%s %s %s %s", n.Kind, n.Name, n.Signature, n.Path)
				lexSc := 0.6*textutil.HashSemantic(q.Task, content) + 0.4*textutil.Overlap(q.Task, content)
				addRaw(c, StageLexical, lexSc)
			}
		}
	}

	// 6. Channel 5: Semantic Scoring for all generated candidates (R18.1 §10)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageSemantic)
	for i := range rawCandidates {
		c := &rawCandidates[i]
		content := fmt.Sprintf("%s %s %s %s", c.Kind, c.Name, c.Signature, c.Path)
		c.Content = content
		semSc := textutil.HashSemantic(q.Task, content)
		c.SemanticScore = semSc
		if c.Trace != nil {
			c.Trace.SemanticScore = semSc
		}
	}

	// 7. Enforce R17.5 Admission Gate (Zero-pollution invariant)
	var admitted []Candidate
	for _, c := range rawCandidates {
		elig := gitidx.EvaluateAdmission(c.Path, []byte(c.Content), true, false, policy)
		if elig.Eligible {
			if c.Trace != nil {
				c.Trace.Admissible = true
			}
			admitted = append(admitted, c)
		} else {
			if c.Trace != nil {
				c.Trace.Admissible = false
				c.Trace.RejectionReason = elig.Reason
			}
			qTrace.AdmissionRejections = append(qTrace.AdmissionRejections, fmt.Sprintf("%s: %s", c.Path, elig.Reason))
		}
	}

	// 8. Multi-Channel Candidate Fusion (R18.1 §11)
	fused := FuseMultiChannels(admitted, profile.FusionWeights)

	// 9. Repository-Aware Graph Expansion (R18.1 §12)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageExpansion)
	seedCount := 5
	if len(fused) < seedCount {
		seedCount = len(fused)
	}
	if seedCount > 0 {
		seeds := fused[:seedCount]
		expanded, expPaths := ExpandCandidateGraph(ctx, seeds, r.Store, q.RepoID, policy, GraphExpansionConfig{
			MaxDepth:        profile.GraphDepth,
			MaxNodesPerHop:  8,
			IncludeTests:    rep.Intent == QueryIntentTest,
			IncludeSiblings: true,
			IncludeCallers:  true,
			MinConfidence:   0.5,
		})
		qTrace.ExpansionNodes = expPaths
		if len(expanded) > 0 {
			fused = FuseMultiChannels(append(fused, expanded...), profile.FusionWeights)
		}
	}

	// 10. Task-Aware Reranking (R18.1 §14 & §15)
	qTrace.RetrievalStages = append(qTrace.RetrievalStages, StageRerank)
	finalRanked := TaskAwareRerank(fused, rep, profile, limit)

	// Populate final trace output
	qTrace.CandidateCount = len(admitted)
	for i, c := range finalRanked {
		if c.Trace != nil {
			c.Trace.Rank = i + 1
			qTrace.FinalRankedResults = append(qTrace.FinalRankedResults, *c.Trace)
			if i < 10 {
				qTrace.TopCandidates = append(qTrace.TopCandidates, *c.Trace)
			}
		}
	}

	_ = start
	return finalRanked, qTrace, nil
}

func nodeToCandidate(n store.NodeRecord, stage string) Candidate {
	return Candidate{
		ID:        "cand:" + n.ID,
		NodeID:    n.ID,
		Kind:      n.Kind,
		Name:      n.Name,
		Path:      n.Path,
		StartLine: n.StartLine,
		EndLine:   n.EndLine,
		Signature: n.Signature,
		Content:   fmt.Sprintf("%s %s %s %s:%d", n.Kind, n.Name, n.Signature, n.Path, n.StartLine),
		Stage:     stage,
		Provenance: []string{stage},
	}
}
