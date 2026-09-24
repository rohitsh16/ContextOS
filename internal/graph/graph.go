package graph

import (
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

// Default edge weights per PR-03 specification.
var DefaultEdgeWeights = map[string]float64{
	"call":           1.0,
	"test-reference": 0.9,
	"test":           0.9,
	"import":         0.8,
	"type-reference": 0.6,
	"type":           0.6,
	"references":     0.7,
	"documentation":  0.3,
	"doc":            0.3,
}

// Config controls PPR convergence and edge weighting.
type Config struct {
	Alpha          float64            // Restart probability (default 0.15)
	MaxIterations  int                // Maximum power-iteration steps (default 30)
	Epsilon        float64            // Convergence threshold (default 1e-5)
	EdgeWeights    map[string]float64 // Kind -> weight mapping
	BetaPPR        float64            // Weight for PPR score (default 0.50)
	BetaDegree     float64            // Weight for normalized degree (default 0.20)
	BetaTest       float64            // Weight for test centrality (default 0.15)
	BetaProximity  float64            // Weight for change proximity (default 0.15)
}

func DefaultConfig() Config {
	return Config{
		Alpha:         0.15,
		MaxIterations: 30,
		Epsilon:       1e-5,
		EdgeWeights:   DefaultEdgeWeights,
		BetaPPR:       0.50,
		BetaDegree:    0.20,
		BetaTest:      0.15,
		BetaProximity: 0.15,
	}
}

type Node struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"` // "file", "symbol", "test", etc.
	Path  string `json:"path"`
	Lines int    `json:"lines,omitempty"`
}

type Edge struct {
	SrcID  string  `json:"src_id"`
	DstID  string  `json:"dst_id"`
	Kind   string  `json:"kind"`
	Weight float64 `json:"weight"`
}

// Graph represents the code dependency graph G = (V, E, W).
type Graph struct {
	nodes             map[string]*Node
	outEdges          map[string][]Edge // src -> out edges
	inEdges           map[string][]Edge // dst -> in edges
	cfg               Config
	cachedMaxDeg      int
	cachedMaxDegValid bool
}

func New(cfg Config) *Graph {
	if cfg.Alpha <= 0 || cfg.Alpha >= 1 {
		cfg.Alpha = 0.15
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 30
	}
	if cfg.Epsilon <= 0 {
		cfg.Epsilon = 1e-5
	}
	if cfg.EdgeWeights == nil {
		cfg.EdgeWeights = DefaultEdgeWeights
	}
	if cfg.BetaPPR == 0 && cfg.BetaDegree == 0 {
		cfg.BetaPPR = 0.50
		cfg.BetaDegree = 0.20
		cfg.BetaTest = 0.15
		cfg.BetaProximity = 0.15
	}
	return &Graph{
		nodes:    make(map[string]*Node),
		outEdges: make(map[string][]Edge),
		inEdges:  make(map[string][]Edge),
		cfg:      cfg,
	}
}

func (g *Graph) AddNode(n *Node) {
	if n != nil && n.ID != "" {
		g.nodes[n.ID] = n
	}
}

func (g *Graph) AddEdge(srcID, dstID, kind string) {
	w, ok := g.cfg.EdgeWeights[kind]
	if !ok {
		w = 0.5
	}
	e := Edge{SrcID: srcID, DstID: dstID, Kind: kind, Weight: w}
	g.outEdges[srcID] = append(g.outEdges[srcID], e)
	g.inEdges[dstID] = append(g.inEdges[dstID], e)
	g.cachedMaxDegValid = false
}

// HasEdge reports whether a directed edge exists from srcID to dstID.
func (g *Graph) HasEdge(srcID, dstID string) bool {
	for _, e := range g.outEdges[srcID] {
		if e.DstID == dstID {
			return true
		}
	}
	return false
}

func (g *Graph) Node(id string) *Node {
	return g.nodes[id]
}

func (g *Graph) NodeCount() int {
	return len(g.nodes)
}

// MaxDegree computes the maximum total degree (in + out) across all nodes.
func (g *Graph) MaxDegree() int {
	if g.cachedMaxDegValid {
		return g.cachedMaxDeg
	}
	maxDeg := 0
	for id := range g.nodes {
		deg := len(g.outEdges[id]) + len(g.inEdges[id])
		if deg > maxDeg {
			maxDeg = deg
		}
	}
	g.cachedMaxDeg = maxDeg
	g.cachedMaxDegValid = true
	return maxDeg
}

// DegreeNorm computes the logarithmic normalized degree:
// DegreeNorm(v) = log(1 + deg(v)) / log(1 + deg_max)
func (g *Graph) DegreeNorm(id string) float64 {
	maxDeg := g.MaxDegree()
	if maxDeg == 0 {
		return 0
	}
	deg := len(g.outEdges[id]) + len(g.inEdges[id])
	return math.Log1p(float64(deg)) / math.Log1p(float64(maxDeg))
}

// ComputePPR calculates Task-conditioned Personalized PageRank:
// r = \alpha s + (1 - \alpha) P^T r
// where s is the normalized seed vector, P is the row-normalized transition matrix.
func (g *Graph) ComputePPR(seeds map[string]float64) map[string]float64 {
	n := len(g.nodes)
	scores := make(map[string]float64, n)
	if n == 0 {
		return scores
	}

	// Normalize seed vector: \sum_v s(v) = 1.0
	var seedSum float64
	for id, w := range seeds {
		if _, exists := g.nodes[id]; exists && w > 0 {
			seedSum += w
		}
	}

	s := make(map[string]float64, n)
	if seedSum > 0 {
		for id, w := range seeds {
			if _, exists := g.nodes[id]; exists && w > 0 {
				s[id] = w / seedSum
			}
		}
	} else {
		// Uniform fallback if no valid seeds
		uniform := 1.0 / float64(n)
		for id := range g.nodes {
			s[id] = uniform
		}
	}

	// Initialize rank r_0 = s
	r := make(map[string]float64, n)
	for id := range g.nodes {
		r[id] = s[id]
	}

	// Precompute out-weight sums for transition probability
	outWeights := make(map[string]float64, n)
	for id, edges := range g.outEdges {
		var sum float64
		for _, e := range edges {
			sum += e.Weight
		}
		outWeights[id] = sum
	}

	alpha := g.cfg.Alpha
	eps := g.cfg.Epsilon

	// Power iteration with convergence check
	for iter := 0; iter < g.cfg.MaxIterations; iter++ {
		nextR := make(map[string]float64, n)

		// Base restart distribution: \alpha * s(v)
		for id := range g.nodes {
			nextR[id] = alpha * s[id]
		}

		// Handle dangling nodes (nodes with 0 out-degree distribute rank uniformly via s)
		var danglingSum float64
		for id := range g.nodes {
			if outWeights[id] == 0 {
				danglingSum += r[id]
			}
		}
		if danglingSum > 0 {
			for id := range g.nodes {
				nextR[id] += (1 - alpha) * danglingSum * s[id]
			}
		}

		// Transition step: (1 - \alpha) P^T r
		for id, edges := range g.outEdges {
			wTotal := outWeights[id]
			if wTotal == 0 {
				continue
			}
			pContrib := (1 - alpha) * r[id]
			for _, e := range edges {
				prob := e.Weight / wTotal
				nextR[e.DstID] += pContrib * prob
			}
		}

		// Check L1 convergence: \sum |nextR[v] - r[v]| < epsilon
		var diff float64
		for id := range g.nodes {
			diff += math.Abs(nextR[id] - r[id])
		}

		r = nextR
		if diff < eps {
			break
		}
	}

	// Min-max scale PPR scores into [0.0, 1.0] for downstream fusion
	var maxScore float64
	for _, v := range r {
		if v > maxScore {
			maxScore = v
		}
	}
	if maxScore > 0 {
		for id, v := range r {
			scores[id] = v / maxScore
		}
	} else {
		for id := range g.nodes {
			scores[id] = 0
		}
	}

	return scores
}

// TestCentrality measures incoming test references to a node.
func (g *Graph) TestCentrality(id string) float64 {
	var testCount int
	for _, e := range g.inEdges[id] {
		if e.Kind == "test-reference" || e.Kind == "test" || strings.Contains(strings.ToLower(e.SrcID), "test") {
			testCount++
		}
	}
	return math.Min(1.0, float64(testCount)*0.33)
}

// CompositeScore computes the advanced graph score per PR-03:
// GScore(v) = \beta_1 PPR(v) + \beta_2 DegreeNorm(v) + \beta_3 TestCentrality(v) + \beta_4 ChangeProximity(v)
func (g *Graph) CompositeScore(id string, pprScores map[string]float64, changeProximity map[string]float64) float64 {
	ppr := pprScores[id]
	deg := g.DegreeNorm(id)
	test := g.TestCentrality(id)
	prox := changeProximity[id]

	score := g.cfg.BetaPPR*ppr + g.cfg.BetaDegree*deg + g.cfg.BetaTest*test + g.cfg.BetaProximity*prox
	return math.Min(1.0, math.Max(0.0, score))
}

var identRegex = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// ExtractSeeds extracts task-conditioned seed weights for PPR from task query text,
// changed git files, and test failures.
func (g *Graph) ExtractSeeds(task string, changedFiles []string, testFailures []string) map[string]float64 {
	seeds := make(map[string]float64)

	// 1. Task query tokens matched against node names and paths
	queryTokens := identRegex.FindAllString(strings.ToLower(task), -1)
	tokenSet := make(map[string]bool)
	for _, t := range queryTokens {
		if len(t) > 2 {
			tokenSet[t] = true
		}
	}

	for id, n := range g.nodes {
		nameLower := strings.ToLower(n.Name)
		pathLower := strings.ToLower(n.Path)

		// Exact match
		if tokenSet[nameLower] {
			seeds[id] += 1.0
		} else {
			// Substring token match
			for tok := range tokenSet {
				if strings.Contains(nameLower, tok) || strings.Contains(pathLower, tok) {
					seeds[id] += 0.5
					break
				}
			}
		}
	}

	// 2. Changed files from git get high seed weight
	for _, cf := range changedFiles {
		cfSlash := filepath.ToSlash(cf)
		for id, n := range g.nodes {
			if n.Path == cfSlash || strings.HasSuffix(n.Path, cfSlash) || strings.HasSuffix(cfSlash, n.Path) {
				seeds[id] += 1.2
			}
		}
	}

	// 3. Test failures get highest seed weight
	for _, tf := range testFailures {
		tfLower := strings.ToLower(tf)
		for id, n := range g.nodes {
			if strings.Contains(strings.ToLower(n.Name), tfLower) || strings.Contains(strings.ToLower(n.Path), tfLower) {
				seeds[id] += 1.5
			}
		}
	}

	return seeds
}

// ComputeChangeProximity computes 1-hop and 2-hop proximity to changed files.
func (g *Graph) ComputeChangeProximity(changedFiles []string) map[string]float64 {
	prox := make(map[string]float64)
	changedNodeIDs := make(map[string]bool)

	for _, cf := range changedFiles {
		cfSlash := filepath.ToSlash(cf)
		for id, n := range g.nodes {
			if n.Path == cfSlash || strings.HasSuffix(n.Path, cfSlash) {
				changedNodeIDs[id] = true
				prox[id] = 1.0
			}
		}
	}

	// 1-hop neighbors get 0.7
	for cID := range changedNodeIDs {
		for _, e := range g.outEdges[cID] {
			if prox[e.DstID] < 0.7 {
				prox[e.DstID] = 0.7
			}
		}
		for _, e := range g.inEdges[cID] {
			if prox[e.SrcID] < 0.7 {
				prox[e.SrcID] = 0.7
			}
		}
	}

	// 2-hop neighbors get 0.4
	for id, p := range prox {
		if p == 0.7 {
			for _, e := range g.outEdges[id] {
				if prox[e.DstID] < 0.4 {
					prox[e.DstID] = 0.4
				}
			}
			for _, e := range g.inEdges[id] {
				if prox[e.SrcID] < 0.4 {
					prox[e.SrcID] = 0.4
				}
			}
		}
	}

	return prox
}
