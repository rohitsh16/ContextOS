package graph

import (
	"container/heap"
	"math"
	"time"

	"contextos/internal/store"
)

// BoundedExpansionConfig controls hard limits and cost-benefit trade-offs.
type BoundedExpansionConfig struct {
	MaxNodes        int           // O4: Maximum nodes to expand (default: 30)
	MaxEdges        int           // O4: Maximum edges to traverse (default: 50)
	MaxDepth        int           // O4: Maximum hop distance from seed nodes (default: 2)
	MaxDuration     time.Duration // O4: Hard execution time budget (default: 15ms)
	DecayFactor     float64       // Distance attenuation gamma (default: 0.75)
	CostLatencyRate float64       // Cost per millisecond of traversal (default: 0.01)
	CostTokenRate   float64       // Cost per estimated token (default: 0.002)
}

// DefaultExpansionConfig returns production bounds.
func DefaultExpansionConfig() BoundedExpansionConfig {
	return BoundedExpansionConfig{
		MaxNodes:        30,
		MaxEdges:        50,
		MaxDepth:        2,
		MaxDuration:     15 * time.Millisecond,
		DecayFactor:     0.75,
		CostLatencyRate: 0.01,
		CostTokenRate:   0.002,
	}
}

// QueueItem represents a frontier candidate in the best-first expansion priority queue.
type QueueItem struct {
	NodeID     string
	SeedID     string
	Depth      int
	Score      float64
	Centrality float64
	InDegree   int
	OutDegree  int
	Priority   float64
	index      int
}

// PriorityQueue implements heap.Interface for best-first expansion (max-heap).
type PriorityQueue []*QueueItem

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Priority > pq[j].Priority } // Max-heap
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *PriorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*QueueItem)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// ExpansionResult returns expanded subgraph nodes, edge counts, and termination reason.
type ExpansionResult struct {
	NodeScores      map[string]float64 `json:"node_scores"`
	NodesExpanded   int                `json:"nodes_expanded"`
	EdgesTraversed  int                `json:"edges_traversed"`
	MaxDepthReached int                `json:"max_depth_reached"`
	Duration        time.Duration      `json:"duration"`
	Termination     string             `json:"termination"` // "voi_below_cost", "max_nodes", "max_edges", "max_depth", "timeout", "queue_empty"
}

// EdgeProvider abstracts fetching adjacent edges for given nodes.
type EdgeProvider interface {
	LookupAdjacentEdges(repoID string, nodeIDs []string) ([]store.EdgeRecord, error)
}

// BoundedBestFirstExpansion executes O4, O5, and O6.
func BoundedBestFirstExpansion(
	repoID string,
	seeds []store.NodeRecord,
	seedScores map[string]float64,
	edges EdgeProvider,
	cfg BoundedExpansionConfig,
) ExpansionResult {
	start := time.Now()
	if cfg.MaxNodes <= 0 {
		cfg.MaxNodes = 30
	}
	if cfg.MaxEdges <= 0 {
		cfg.MaxEdges = 50
	}
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 2
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = 15 * time.Millisecond
	}
	if cfg.DecayFactor <= 0 || cfg.DecayFactor > 1 {
		cfg.DecayFactor = 0.75
	}
	if cfg.CostLatencyRate <= 0 {
		cfg.CostLatencyRate = 0.01
	}

	result := ExpansionResult{
		NodeScores:  make(map[string]float64),
		Termination: "queue_empty",
	}

	if len(seeds) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	pq := make(PriorityQueue, 0)
	heap.Init(&pq)

	visited := make(map[string]bool)

	// Initialize frontier with seeds
	for _, s := range seeds {
		sc := seedScores[s.ID]
		if sc <= 0 {
			sc = 0.5
		}
		prio := sc * (1.0 + s.Centrality)
		item := &QueueItem{
			NodeID:     s.ID,
			SeedID:     s.ID,
			Depth:      0,
			Score:      sc,
			Centrality: s.Centrality,
			InDegree:   s.InDegree,
			OutDegree:  s.OutDegree,
			Priority:   prio,
		}
		heap.Push(&pq, item)
		result.NodeScores[s.ID] = prio
	}

	for pq.Len() > 0 {
		// O4 Timeout check
		elapsed := time.Since(start)
		if elapsed >= cfg.MaxDuration {
			result.Termination = "timeout"
			break
		}

		// O4 Node bound check
		if result.NodesExpanded >= cfg.MaxNodes {
			result.Termination = "max_nodes"
			break
		}

		// Pop best candidate (O5)
		curr := heap.Pop(&pq).(*QueueItem)
		if visited[curr.NodeID] {
			continue
		}
		visited[curr.NodeID] = true
		result.NodesExpanded++
		if curr.Depth > result.MaxDepthReached {
			result.MaxDepthReached = curr.Depth
		}

		// O6: Adaptive Stopping (VOI_next < Cost_next)
		// Value of information is proportional to novelty score and centrality
		voiNext := curr.Priority * math.Pow(cfg.DecayFactor, float64(curr.Depth))
		costNext := cfg.CostLatencyRate*float64(elapsed.Milliseconds()) + cfg.CostTokenRate*10.0 // estimated token overhead
		if curr.Depth > 0 && voiNext < costNext {
			result.Termination = "voi_below_cost"
			break
		}

		// O4 Depth bound
		if curr.Depth >= cfg.MaxDepth {
			continue
		}

		// Lookup adjacent edges for current node
		if edges != nil {
			adjEdges, err := edges.LookupAdjacentEdges(repoID, []string{curr.NodeID})
			if err != nil || len(adjEdges) == 0 {
				continue
			}

			for _, e := range adjEdges {
				if result.EdgesTraversed >= cfg.MaxEdges {
					result.Termination = "max_edges"
					break
				}
				result.EdgesTraversed++

				neighborID := e.DstID
				if neighborID == curr.NodeID {
					neighborID = e.SrcID
				}
				if visited[neighborID] {
					continue
				}

				edgeWeight := DefaultEdgeWeights[e.Kind]
				if edgeWeight == 0 {
					edgeWeight = 0.5
				}

				nextScore := curr.Score * edgeWeight * cfg.DecayFactor
				nextPriority := nextScore * (1.0 + curr.Centrality)

				if existing, exists := result.NodeScores[neighborID]; !exists || nextPriority > existing {
					result.NodeScores[neighborID] = nextPriority
					heap.Push(&pq, &QueueItem{
						NodeID:   neighborID,
						SeedID:   curr.SeedID,
						Depth:    curr.Depth + 1,
						Score:    nextScore,
						Priority: nextPriority,
					})
				}
			}
			if result.Termination == "max_edges" {
				break
			}
		}
	}

	result.Duration = time.Since(start)
	return result
}
