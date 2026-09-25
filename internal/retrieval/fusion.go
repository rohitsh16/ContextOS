package retrieval

import (
	"sort"
)

// FusionWeights specifies explicit weights for multi-signal candidate fusion (PR.md Section 17).
type FusionWeights struct {
	Exact   float64
	Lexical float64
	Trigram float64
	Graph   float64
	Recent  float64
	Sparse  float64
}

// DefaultFusionWeights provides balanced benchmarked starting weights.
var DefaultFusionWeights = FusionWeights{
	Exact:   0.30,
	Lexical: 0.20,
	Trigram: 0.20,
	Graph:   0.15,
	Recent:  0.10,
	Sparse:  0.05,
}

// CandidateSignals stores individual component scores before fusion.
type CandidateSignals struct {
	Exact   float64
	Lexical float64
	Trigram float64
	Graph   float64
	Recent  float64
	Sparse  float64
}

// FuseCandidates combines candidates from heterogeneous retrieval stages (PR.md Section 17).
// Formula: S(c) = w_e*E + w_l*L + w_t*T + w_g*G + w_r*R + w_s*S
func FuseCandidates(
	exactCandidates []Candidate,
	lexicalCandidates []Candidate,
	trigramCandidates []Candidate,
	graphCandidates []Candidate,
	recentCandidates []Candidate,
	sparseCandidates []Candidate,
	weights FusionWeights,
) []Candidate {
	type mergedEntry struct {
		cand    Candidate
		signals CandidateSignals
	}

	merged := make(map[string]*mergedEntry)

	getOrCreate := func(c Candidate) *mergedEntry {
		e, exists := merged[c.NodeID]
		if !exists {
			e = &mergedEntry{
				cand: Candidate{
					NodeID:     c.NodeID,
					Kind:       c.Kind,
					Name:       c.Name,
					Path:       c.Path,
					Package:    c.Package,
					Tokens:     c.Tokens,
					Stage:      "fused",
					Provenance: []string{},
				},
			}
			merged[c.NodeID] = e
		}
		if e.cand.Name == "" && c.Name != "" {
			e.cand.Name = c.Name
		}
		if e.cand.Path == "" && c.Path != "" {
			e.cand.Path = c.Path
		}
		if e.cand.Package == "" && c.Package != "" {
			e.cand.Package = c.Package
		}
		if e.cand.Tokens == 0 && c.Tokens > 0 {
			e.cand.Tokens = c.Tokens
		}
		return e
	}

	for _, c := range exactCandidates {
		e := getOrCreate(c)
		e.signals.Exact = c.Score
		e.cand.Provenance = append(e.cand.Provenance, "exact")
	}

	for _, c := range lexicalCandidates {
		e := getOrCreate(c)
		e.signals.Lexical = c.Score
		e.cand.Provenance = append(e.cand.Provenance, "lexical")
	}

	for _, c := range trigramCandidates {
		e := getOrCreate(c)
		e.signals.Trigram = c.Score
		e.cand.Provenance = append(e.cand.Provenance, "trigram")
	}

	for _, c := range graphCandidates {
		e := getOrCreate(c)
		e.signals.Graph = c.Score
		e.cand.Provenance = append(e.cand.Provenance, "graph")
	}

	for _, c := range recentCandidates {
		e := getOrCreate(c)
		e.signals.Recent = c.Score
		e.cand.Provenance = append(e.cand.Provenance, "recent")
	}

	for _, c := range sparseCandidates {
		e := getOrCreate(c)
		e.signals.Sparse = c.Score
		e.cand.Provenance = append(e.cand.Provenance, "sparse")
	}

	results := make([]Candidate, 0, len(merged))
	for _, entry := range merged {
		// Multi-signal weighted scoring
		finalScore := weights.Exact*entry.signals.Exact +
			weights.Lexical*entry.signals.Lexical +
			weights.Trigram*entry.signals.Trigram +
			weights.Graph*entry.signals.Graph +
			weights.Recent*entry.signals.Recent +
			weights.Sparse*entry.signals.Sparse

		entry.cand.Score = finalScore
		results = append(results, entry.cand)
	}

	// Sort descending by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// MultiChannelWeights specifies weights for multi-channel candidate fusion (R18.1 §11 & §14).
type MultiChannelWeights struct {
	Lexical  float64 `json:"lexical"`
	Semantic float64 `json:"semantic"`
	Symbol   float64 `json:"symbol"`
	Path     float64 `json:"path"`
	Entity   float64 `json:"entity"`
	Graph    float64 `json:"graph"`
}

// DefaultMultiChannelWeights provides balanced baseline channel weights.
var DefaultMultiChannelWeights = MultiChannelWeights{
	Lexical:  0.20,
	Semantic: 0.20,
	Symbol:   0.20,
	Path:     0.20,
	Entity:   0.15,
	Graph:    0.05,
}

// FuseMultiChannels fuses candidates across the 5 retrieval channels and updates candidate traces.
// S(c) = wL*lexical + wS*semantic + wY*symbol + wP*path + wE*entity + wG*graph
func FuseMultiChannels(candidates []Candidate, weights MultiChannelWeights) []Candidate {
	merged := make(map[string]*Candidate)

	for _, c := range candidates {
		key := c.Path
		if key == "" {
			key = c.NodeID
		}
		if key == "" {
			key = c.ID
		}

		existing, exists := merged[key]
		if !exists {
			clone := c
			if clone.Trace == nil {
				clone.Trace = &CandidateTrace{
					ID:         clone.ID,
					Path:       clone.Path,
					Name:       clone.Name,
					Stages:     []RetrievalStage{},
					Admissible: true,
				}
			}
			merged[key] = &clone
			existing = &clone
		}

		// Merge stage provenance
		if c.Stage != "" {
			existing.Trace.AddStage(RetrievalStage(c.Stage))
		}
		if c.Trace != nil {
			for _, st := range c.Trace.Stages {
				existing.Trace.AddStage(st)
			}
			if c.Trace.LexicalScore > existing.Trace.LexicalScore {
				existing.Trace.LexicalScore = c.Trace.LexicalScore
				existing.LexicalScore = c.Trace.LexicalScore
			}
			if c.Trace.SemanticScore > existing.Trace.SemanticScore {
				existing.Trace.SemanticScore = c.Trace.SemanticScore
				existing.SemanticScore = c.Trace.SemanticScore
			}
			if c.Trace.SymbolScore > existing.Trace.SymbolScore {
				existing.Trace.SymbolScore = c.Trace.SymbolScore
			}
			if c.Trace.PathScore > existing.Trace.PathScore {
				existing.Trace.PathScore = c.Trace.PathScore
				existing.PathScore = c.Trace.PathScore
			}
			if c.Trace.EntityScore > existing.Trace.EntityScore {
				existing.Trace.EntityScore = c.Trace.EntityScore
				existing.EntityScore = c.Trace.EntityScore
			}
			if c.Trace.GraphScore > existing.Trace.GraphScore {
				existing.Trace.GraphScore = c.Trace.GraphScore
				existing.GraphScore = c.Trace.GraphScore
			}
		}

		if c.LexicalScore > existing.LexicalScore {
			existing.LexicalScore = c.LexicalScore
			existing.Trace.LexicalScore = c.LexicalScore
		}
		if c.SemanticScore > existing.SemanticScore {
			existing.SemanticScore = c.SemanticScore
			existing.Trace.SemanticScore = c.SemanticScore
		}
		if c.EntityScore > existing.EntityScore {
			existing.EntityScore = c.EntityScore
			existing.Trace.EntityScore = c.EntityScore
		}
		if c.PathScore > existing.PathScore {
			existing.PathScore = c.PathScore
			existing.Trace.PathScore = c.PathScore
		}
		if c.GraphScore > existing.GraphScore {
			existing.GraphScore = c.GraphScore
			existing.Trace.GraphScore = c.GraphScore
		}
	}

	results := make([]Candidate, 0, len(merged))
	for _, c := range merged {
		tr := c.Trace
		// Compute fused score
		fused := weights.Lexical*tr.LexicalScore +
			weights.Semantic*tr.SemanticScore +
			weights.Symbol*tr.SymbolScore +
			weights.Path*tr.PathScore +
			weights.Entity*tr.EntityScore +
			weights.Graph*tr.GraphScore

		// Tier-0 bonus for exact path/symbol match
		if tr.HasStage(StageExactPath) || tr.HasStage(StageBasename) || tr.HasStage(StageSymbol) {
			fused += 0.5
		}

		c.Score = fused
		tr.FinalScore = fused
		results = append(results, *c)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	for i := range results {
		results[i].Trace.Rank = i + 1
	}

	return results
}

