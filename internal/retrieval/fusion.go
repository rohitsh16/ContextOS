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
