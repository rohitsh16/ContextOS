package retrieval

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"contextos/internal/gitidx"
)

// EvidenceRelation represents typed semantic, call-graph, and dependency links between evidence nodes (R18 §25).
type EvidenceRelation string

const (
	Supports    EvidenceRelation = "supports"
	DependsOn   EvidenceRelation = "depends_on"
	Calls       EvidenceRelation = "calls"
	Implements  EvidenceRelation = "implements"
	Tests       EvidenceRelation = "tests"
	Documents   EvidenceRelation = "documents"
	Contradicts EvidenceRelation = "contradicts"
	Supersedes  EvidenceRelation = "supersedes"
	Invalidates EvidenceRelation = "invalidates"
)

// EvidenceNode is an atomic evidence unit carrying provenance, authority, and token metadata (R18 §25).
type EvidenceNode struct {
	ID         string                    `json:"id"`
	Path       string                    `json:"path"`
	Symbol     string                    `json:"symbol"`
	Kind       string                    `json:"kind"`
	Provenance gitidx.EvidenceProvenance `json:"provenance"`
	Authority  float64                   `json:"authority"`
	Freshness  float64                   `json:"freshness"`
	Tokens     int                       `json:"tokens"`
	Content    string                    `json:"content"`
}

// EvidenceEdge defines a directed relationship in the evidence graph.
type EvidenceEdge struct {
	From        string           `json:"from"`
	To          string           `json:"to"`
	Relation    EvidenceRelation `json:"relation"`
	Weight      float64          `json:"weight"`
	Confidence  float64          `json:"confidence,omitempty"`
	Source      string           `json:"source,omitempty"` // "ast-derived", "index-derived", "text-derived", "heuristic"
	Description string           `json:"description,omitempty"`
}

// EvidenceGraph maintains the dependency, call-chain, and conflict relationships between evidence nodes.
// Unlike a structural code graph, the EvidenceGraph describes what establishes what (R18 §25).
type EvidenceGraph struct {
	mu      sync.RWMutex
	nodes   map[string]*EvidenceNode
	edges   map[string][]EvidenceEdge
	inEdges map[string][]EvidenceEdge
}

// NewEvidenceGraph initializes a new thread-safe EvidenceGraph.
func NewEvidenceGraph() *EvidenceGraph {
	return &EvidenceGraph{
		nodes:   make(map[string]*EvidenceNode),
		edges:   make(map[string][]EvidenceEdge),
		inEdges: make(map[string][]EvidenceEdge),
	}
}

// AddNode registers an evidence node.
func (eg *EvidenceGraph) AddNode(node *EvidenceNode) {
	eg.mu.Lock()
	defer eg.mu.Unlock()
	if node == nil || node.ID == "" {
		return
	}
	eg.nodes[node.ID] = node
}

// AddEdge registers a directed relation between evidence nodes.
func (eg *EvidenceGraph) AddEdge(edge EvidenceEdge) {
	eg.mu.Lock()
	defer eg.mu.Unlock()
	eg.edges[edge.From] = append(eg.edges[edge.From], edge)
	eg.inEdges[edge.To] = append(eg.inEdges[edge.To], edge)
}

// GetNode looks up an evidence node by ID.
func (eg *EvidenceGraph) GetNode(id string) (*EvidenceNode, bool) {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	n, ok := eg.nodes[id]
	return n, ok
}

// OutNeighbors returns outgoing edges from nodeID.
func (eg *EvidenceGraph) OutNeighbors(id string) []EvidenceEdge {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	edges := eg.edges[id]
	out := make([]EvidenceEdge, len(edges))
	copy(out, edges)
	return out
}

// InNeighbors returns incoming edges to nodeID.
func (eg *EvidenceGraph) InNeighbors(id string) []EvidenceEdge {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	edges := eg.inEdges[id]
	out := make([]EvidenceEdge, len(edges))
	copy(out, edges)
	return out
}

// NodeCount returns total registered nodes.
func (eg *EvidenceGraph) NodeCount() int {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	return len(eg.nodes)
}

// EdgeCount returns total registered edges.
func (eg *EvidenceGraph) EdgeCount() int {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	count := 0
	for _, l := range eg.edges {
		count += len(l)
	}
	return count
}

// FindChains returns all directed paths from startNode to endNode up to maxDepth (R18 §51).
func (eg *EvidenceGraph) FindChains(startID, endID string, maxDepth int) [][]string {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	var results [][]string
	var dfs func(curr string, path []string, visited map[string]bool)

	dfs = func(curr string, path []string, visited map[string]bool) {
		if len(path) > maxDepth {
			return
		}
		if curr == endID && len(path) > 1 {
			chain := make([]string, len(path))
			copy(chain, path)
			results = append(results, chain)
			return
		}
		for _, e := range eg.edges[curr] {
			if !visited[e.To] {
				visited[e.To] = true
				dfs(e.To, append(path, e.To), visited)
				delete(visited, e.To)
			}
		}
	}

	vis := map[string]bool{startID: true}
	dfs(startID, []string{startID}, vis)
	return results
}

// DependencyClosure finds all transitive dependencies of seedIDs up to maxDepth.
func (eg *EvidenceGraph) DependencyClosure(seedIDs []string, maxDepth int) []*EvidenceNode {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	visited := make(map[string]bool)
	var queue []string

	for _, s := range seedIDs {
		if !visited[s] {
			visited[s] = true
			queue = append(queue, s)
		}
	}

	depth := 0
	for len(queue) > 0 && depth < maxDepth {
		size := len(queue)
		for i := 0; i < size; i++ {
			curr := queue[i]
			for _, edge := range eg.edges[curr] {
				if (edge.Relation == DependsOn || edge.Relation == Calls || edge.Relation == Implements) && !visited[edge.To] {
					visited[edge.To] = true
					queue = append(queue, edge.To)
				}
			}
		}
		queue = queue[size:]
		depth++
	}

	var nodes []*EvidenceNode
	for id := range visited {
		if n, ok := eg.nodes[id]; ok {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// HydrateCandidate produces a canonical, hydrated EvidenceNode using DefaultAdmissionPolicy.
func HydrateCandidate(c Candidate, repoID, revision string) *EvidenceNode {
	return HydrateCandidateWithPolicy(c, repoID, revision, gitidx.DefaultAdmissionPolicy())
}

// HydrateCandidateWithPolicy produces a canonical, hydrated EvidenceNode under the given policy (R18 §26).
func HydrateCandidateWithPolicy(c Candidate, repoID, revision string, policy gitidx.AdmissionPolicy) *EvidenceNode {
	relPath := c.Path
	if relPath == "" {
		relPath = c.ID
	}
	relPath = filepath.ToSlash(relPath)

	tokens := c.Tokens
	if tokens <= 0 {
		tokens = len(strings.Fields(c.Content))
		if tokens <= 0 {
			tokens = 50
		}
	}

	class, _ := gitidx.Classify(relPath, []byte(c.Content), true, false)
	eligibility := gitidx.EvaluateAdmission(relPath, []byte(c.Content), true, false, policy)

	startLine := c.StartLine
	if startLine <= 0 {
		startLine = 1
	}
	endLine := c.EndLine
	if endLine < startLine {
		endLine = startLine + strings.Count(c.Content, "\n")
	}

	prov := gitidx.EvidenceProvenance{
		RepoID:        repoID,
		Revision:      revision,
		Path:          relPath,
		StartLine:     startLine,
		EndLine:       endLine,
		ContentHash:   gitidx.ComputeContentHash([]byte(c.Content)),
		SourceClass:   class,
		Eligible:      eligibility.Eligible,
		Authority:     eligibility.Authority,
		GitTracked:    true,
		GitIgnored:    false,
		IndexedAt:     time.Now().UTC(),
		PolicyVersion: "R18-v1",
	}

	return &EvidenceNode{
		ID:         c.ID,
		Path:       relPath,
		Symbol:     c.Name,
		Kind:       c.Kind,
		Provenance: prov,
		Authority:  prov.Authority,
		Freshness:  1.0,
		Tokens:     tokens,
		Content:    c.Content,
	}
}
