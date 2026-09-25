package retrieval

import (
	"context"
	"path/filepath"
	"strings"

	"contextos/internal/gitidx"
	"contextos/internal/store"
)

// GraphExpansionConfig configures bounded repository graph expansion (R18.1 §12).
type GraphExpansionConfig struct {
	MaxDepth         int     `json:"max_depth"`          // Maximum traversal hops (default: 2)
	MaxNodesPerHop   int     `json:"max_nodes_per_hop"`  // Maximum nodes expanded per step (default: 10)
	IncludeTests     bool    `json:"include_tests"`      // Expand test files
	IncludeSiblings  bool    `json:"include_siblings"`   // Expand same-package siblings
	IncludeCallers   bool    `json:"include_callers"`    // Expand callers & callees
	MinConfidence    float64 `json:"min_confidence"`     // Minimum edge confidence (default: 0.5)
}

// DefaultGraphExpansionConfig provides safe, bounded expansion defaults.
var DefaultGraphExpansionConfig = GraphExpansionConfig{
	MaxDepth:        2,
	MaxNodesPerHop:  10,
	IncludeTests:    true,
	IncludeSiblings: true,
	IncludeCallers:  true,
	MinConfidence:   0.5,
}

// ExpandCandidateGraph performs bounded expansion from seed candidates through repository structure.
func ExpandCandidateGraph(
	ctx context.Context,
	seeds []Candidate,
	st store.Store,
	repoID string,
	policy gitidx.AdmissionPolicy,
	cfg GraphExpansionConfig,
) ([]Candidate, []string) {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 2
	}
	if cfg.MaxNodesPerHop <= 0 {
		cfg.MaxNodesPerHop = 10
	}

	visitedNodes := make(map[string]bool)
	visitedPaths := make(map[string]bool)
	for _, s := range seeds {
		if s.NodeID != "" {
			visitedNodes[s.NodeID] = true
		}
		if s.Path != "" {
			visitedPaths[s.Path] = true
		}
	}

	currentHop := make([]Candidate, len(seeds))
	copy(currentHop, seeds)

	var expandedCands []Candidate
	var expandedPaths []string

	for depth := 1; depth <= cfg.MaxDepth; depth++ {
		select {
		case <-ctx.Done():
			return expandedCands, expandedPaths
		default:
		}

		nextHop := make([]Candidate, 0)
		for _, seed := range currentHop {
			// 1. Same-package siblings expansion
			if cfg.IncludeSiblings && seed.Path != "" {
				pkgDir := filepath.Dir(seed.Path)
				dirNodes, err := st.LookupPath(repoID, pkgDir)
				if err == nil {
					added := 0
					for _, dn := range dirNodes {
						if added >= cfg.MaxNodesPerHop {
							break
						}
						if visitedPaths[dn.Path] {
							continue
						}
						// Check admission
						elig := gitidx.EvaluateAdmission(dn.Path, []byte(dn.Signature), true, false, policy)
						if !elig.Eligible {
							continue
						}
						visitedPaths[dn.Path] = true
						c := Candidate{
							ID:         "cand:" + dn.ID,
							NodeID:     dn.ID,
							Kind:       dn.Kind,
							Name:       dn.Name,
							Path:       dn.Path,
							StartLine:  dn.StartLine,
							EndLine:    dn.EndLine,
							Signature:  dn.Signature,
							Content:    dn.Signature,
							GraphScore: 0.7 / float64(depth),
							Score:      0.7 / float64(depth),
							Stage:      string(StageExpansion),
							Provenance: []string{"same_package_sibling", seed.Path},
						}
						c.Trace = &CandidateTrace{
							ID:         c.ID,
							Path:       c.Path,
							Name:       c.Name,
							Stages:     []RetrievalStage{StageExpansion},
							GraphScore: c.GraphScore,
							FinalScore: c.Score,
							Admissible: true,
						}
						nextHop = append(nextHop, c)
						expandedCands = append(expandedCands, c)
						expandedPaths = append(expandedPaths, c.Path)
						added++
					}
				}
			}

			// 2. Call-graph expansion (Callers and Callees via LookupAdjacentEdges)
			if cfg.IncludeCallers && seed.NodeID != "" {
				edges, err := st.LookupAdjacentEdges(repoID, []string{seed.NodeID})
				if err == nil {
					added := 0
					var adjacentNodeIDs []string
					for _, edge := range edges {
						targetID := edge.DstID
						if targetID == seed.NodeID {
							targetID = edge.SrcID
						}
						if !visitedNodes[targetID] {
							visitedNodes[targetID] = true
							adjacentNodeIDs = append(adjacentNodeIDs, targetID)
							added++
							if added >= cfg.MaxNodesPerHop {
								break
							}
						}
					}

					for _, adjID := range adjacentNodeIDs {
						adjNodes, err := st.LookupPath(repoID, adjID)
						if err == nil && len(adjNodes) > 0 {
							dn := adjNodes[0]
							elig := gitidx.EvaluateAdmission(dn.Path, []byte(dn.Signature), true, false, policy)
							if !elig.Eligible {
								continue
							}
							c := Candidate{
								ID:         "cand:" + dn.ID,
								NodeID:     dn.ID,
								Kind:       dn.Kind,
								Name:       dn.Name,
								Path:       dn.Path,
								StartLine:  dn.StartLine,
								EndLine:    dn.EndLine,
								Signature:  dn.Signature,
								Content:    dn.Signature,
								GraphScore: 0.85 / float64(depth),
								Score:      0.85 / float64(depth),
								Stage:      string(StageExpansion),
								Provenance: []string{"call_graph_edge", seed.Path},
							}
							c.Trace = &CandidateTrace{
								ID:         c.ID,
								Path:       c.Path,
								Name:       c.Name,
								Stages:     []RetrievalStage{StageExpansion, StageGraph},
								GraphScore: c.GraphScore,
								FinalScore: c.Score,
								Admissible: true,
							}
							nextHop = append(nextHop, c)
							expandedCands = append(expandedCands, c)
							expandedPaths = append(expandedPaths, c.Path)
						}
					}
				}
			}

			// 3. Test file expansion: for foo.go look for foo_test.go
			if cfg.IncludeTests && seed.Path != "" && strings.HasSuffix(seed.Path, ".go") && !strings.HasSuffix(seed.Path, "_test.go") {
				testPath := strings.TrimSuffix(seed.Path, ".go") + "_test.go"
				if !visitedPaths[testPath] {
					visitedPaths[testPath] = true
					testNodes, err := st.LookupExactPath(repoID, testPath)
					if err == nil && len(testNodes) > 0 {
						for _, tn := range testNodes {
							elig := gitidx.EvaluateAdmission(tn.Path, []byte(tn.Signature), true, false, policy)
							if elig.Eligible {
								c := Candidate{
									ID:         "cand:" + tn.ID,
									NodeID:     tn.ID,
									Kind:       tn.Kind,
									Name:       tn.Name,
									Path:       tn.Path,
									StartLine:  tn.StartLine,
									EndLine:    tn.EndLine,
									Signature:  tn.Signature,
									Content:    tn.Signature,
									GraphScore: 0.6 / float64(depth),
									Score:      0.6 / float64(depth),
									Stage:      string(StageExpansion),
									Provenance: []string{"associated_test", seed.Path},
								}
								c.Trace = &CandidateTrace{
									ID:         c.ID,
									Path:       c.Path,
									Name:       c.Name,
									Stages:     []RetrievalStage{StageExpansion},
									GraphScore: c.GraphScore,
									FinalScore: c.Score,
									Admissible: true,
								}
								expandedCands = append(expandedCands, c)
								expandedPaths = append(expandedPaths, c.Path)
							}
						}
					}
				}
			}
		}

		if len(nextHop) == 0 {
			break
		}
		currentHop = nextHop
	}

	return expandedCands, expandedPaths
}
