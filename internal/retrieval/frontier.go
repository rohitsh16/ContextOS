package retrieval

import (
	"context"
	"strings"

	"contextos/internal/gitidx"
)

// EvidenceFrontier coordinates iterative candidate expansion and sufficiency gating (R18 §27).
type EvidenceFrontier struct {
	Graph       *EvidenceGraph
	Contract    QueryContract
	Policy      gitidx.AdmissionPolicy
	RepoID      string
	Revision    string
	MaxRounds   int
	MinCoverage float64
}

// NewEvidenceFrontier initializes an evidence frontier with DefaultAdmissionPolicy.
func NewEvidenceFrontier(contract QueryContract, repoID, revision string) *EvidenceFrontier {
	return NewEvidenceFrontierWithPolicy(contract, repoID, revision, gitidx.DefaultAdmissionPolicy())
}

// NewEvidenceFrontierWithPolicy initializes an evidence frontier with explicit admission policy.
func NewEvidenceFrontierWithPolicy(contract QueryContract, repoID, revision string, policy gitidx.AdmissionPolicy) *EvidenceFrontier {
	return &EvidenceFrontier{
		Graph:       NewEvidenceGraph(),
		Contract:    contract,
		Policy:      policy,
		RepoID:      repoID,
		Revision:    revision,
		MaxRounds:   3,
		MinCoverage: 0.8,
	}
}

// FrontierResult holds the expanded evidence pool and its sufficiency status.
type FrontierResult struct {
	Evidence    []*EvidenceNode
	Sufficiency SufficiencyResult
	RoundsRun   int
	GraphNodes  int
	GraphEdges  int
}

// ExpandFrontier iteratively explores dependencies from seed candidates until sufficiency is satisfied (R18 §27).
func (ef *EvidenceFrontier) ExpandFrontier(ctx context.Context, seeds []Candidate) FrontierResult {
	seedNodes := make([]*EvidenceNode, 0, len(seeds))
	for _, c := range seeds {
		node := HydrateCandidateWithPolicy(c, ef.RepoID, ef.Revision, ef.Policy)
		if node.Provenance.Eligible {
			ef.Graph.AddNode(node)
			seedNodes = append(seedNodes, node)
		}
	}

	currentPool := seedNodes
	var suff SufficiencyResult
	rounds := 0

	for rounds < ef.MaxRounds {
		select {
		case <-ctx.Done():
			break
		default:
		}

		rounds++
		suff = EvaluateSufficiency(currentPool, ef.Contract, ef.MinCoverage)
		if suff.Sufficient {
			break
		}

		// Expand dependencies from current pool
		seedIDs := make([]string, 0, len(currentPool))
		for _, n := range currentPool {
			seedIDs = append(seedIDs, n.ID)
		}

		deps := ef.Graph.DependencyClosure(seedIDs, 2)
		poolMap := make(map[string]*EvidenceNode)
		for _, n := range currentPool {
			poolMap[n.ID] = n
		}
		for _, d := range deps {
			if d.Provenance.Eligible {
				poolMap[d.ID] = d
			}
		}

		// Connect call-chain edges between entities if contract is a trace
		if ef.Contract.NeedsCallGraph {
			for i := 0; i < len(currentPool); i++ {
				for j := 0; j < len(currentPool); j++ {
					if i == j {
						continue
					}
					src := currentPool[i]
					dst := currentPool[j]
					if strings.Contains(src.Content, dst.Symbol) && dst.Symbol != "" {
						ef.Graph.AddEdge(EvidenceEdge{
							From:     src.ID,
							To:       dst.ID,
							Relation: Calls,
							Weight:   1.0,
						})
					}
				}
			}
		}

		currentPool = make([]*EvidenceNode, 0, len(poolMap))
		for _, n := range poolMap {
			currentPool = append(currentPool, n)
		}
	}

	return FrontierResult{
		Evidence:    currentPool,
		Sufficiency: suff,
		RoundsRun:   rounds,
		GraphNodes:  ef.Graph.NodeCount(),
		GraphEdges:  ef.Graph.EdgeCount(),
	}
}
