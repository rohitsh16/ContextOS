package graph

import (
	"sort"
	"sync"
)

// Edge represents a directed relationship between two code entities (PR.md Section 15).
type Edge struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Type   string  `json:"type"`   // e.g. "calls", "imports", "implements", "references"
	Weight float64 `json:"weight"` // default 1.0
}

// PersistentGraph maintains bounded persistent adjacency lists without full-repo rebuilds at query time.
type PersistentGraph struct {
	mu             sync.RWMutex
	outgoing       map[string][]Edge
	incoming       map[string][]Edge
	fileToSymbols  map[string][]string
	symbolToFile   map[string]string
	packageToFiles map[string][]string
	packageDeps    map[string][]string
}

// NewPersistentGraph initializes an empty persistent graph structure.
func NewPersistentGraph() *PersistentGraph {
	return &PersistentGraph{
		outgoing:       make(map[string][]Edge),
		incoming:       make(map[string][]Edge),
		fileToSymbols:  make(map[string][]string),
		symbolToFile:   make(map[string]string),
		packageToFiles: make(map[string][]string),
		packageDeps:    make(map[string][]string),
	}
}

// AddNode registers a node with its file and package mappings.
func (pg *PersistentGraph) AddNode(id, file, pkg string, isSymbol bool) {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if file != "" {
		if isSymbol {
			pg.fileToSymbols[file] = append(pg.fileToSymbols[file], id)
			pg.symbolToFile[id] = file
		}
		if pkg != "" {
			// Register file in package if not present
			files := pg.packageToFiles[pkg]
			found := false
			for _, f := range files {
				if f == file {
					found = true
					break
				}
			}
			if !found {
				pg.packageToFiles[pkg] = append(files, file)
			}
		}
	}
}

// AddEdge registers a directed edge in both outgoing and incoming adjacency lists.
func (pg *PersistentGraph) AddEdge(e Edge) {
	if e.Weight == 0 {
		e.Weight = 1.0
	}
	pg.mu.Lock()
	defer pg.mu.Unlock()

	pg.outgoing[e.From] = append(pg.outgoing[e.From], e)
	pg.incoming[e.To] = append(pg.incoming[e.To], e)

	if e.Type == "imports" {
		pg.packageDeps[e.From] = append(pg.packageDeps[e.From], e.To)
	}
}

// Neighbors performs a bounded BFS expansion up to specified depth and limit.
func (pg *PersistentGraph) Neighbors(nodeID string, depth, limit int) []string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	if depth <= 0 || limit <= 0 {
		return nil
	}

	visited := make(map[string]bool)
	visited[nodeID] = true
	queue := []string{nodeID}
	var results []string

	for d := 0; d < depth && len(queue) > 0; d++ {
		nextQueue := make([]string, 0, len(queue)*4)
		for _, curr := range queue {
			for _, edge := range pg.outgoing[curr] {
				if !visited[edge.To] {
					visited[edge.To] = true
					results = append(results, edge.To)
					if len(results) >= limit {
						return results
					}
					nextQueue = append(nextQueue, edge.To)
				}
			}
		}
		queue = nextQueue
	}

	return results
}

// ReverseNeighbors performs a bounded reverse BFS expansion up to specified depth and limit.
func (pg *PersistentGraph) ReverseNeighbors(nodeID string, depth, limit int) []string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	if depth <= 0 || limit <= 0 {
		return nil
	}

	visited := make(map[string]bool)
	visited[nodeID] = true
	queue := []string{nodeID}
	var results []string

	for d := 0; d < depth && len(queue) > 0; d++ {
		nextQueue := make([]string, 0, len(queue)*4)
		for _, curr := range queue {
			for _, edge := range pg.incoming[curr] {
				if !visited[edge.From] {
					visited[edge.From] = true
					results = append(results, edge.From)
					if len(results) >= limit {
						return results
					}
					nextQueue = append(nextQueue, edge.From)
				}
			}
		}
		queue = nextQueue
	}

	return results
}

// ExpandSeeds performs bounded multi-seed graph expansion from seed candidate IDs (PR.md Section 15).
func (pg *PersistentGraph) ExpandSeeds(seedIDs []string, depth, limitPerSeed int) []string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	if len(seedIDs) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	for _, id := range seedIDs {
		seen[id] = true
	}

	var expanded []string
	for _, seed := range seedIDs {
		neighbors := pg.Neighbors(seed, depth, limitPerSeed)
		for _, n := range neighbors {
			if !seen[n] {
				seen[n] = true
				expanded = append(expanded, n)
			}
		}
	}

	sort.Strings(expanded)
	return expanded
}

// FileSymbols returns all symbols belonging to a source file.
func (pg *PersistentGraph) FileSymbols(file string) []string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	syms := pg.fileToSymbols[file]
	res := make([]string, len(syms))
	copy(res, syms)
	return res
}

// SymbolFile returns the source file defining a symbol.
func (pg *PersistentGraph) SymbolFile(symbolID string) string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	return pg.symbolToFile[symbolID]
}

// PackageFiles returns all files associated with a package.
func (pg *PersistentGraph) PackageFiles(pkg string) []string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	files := pg.packageToFiles[pkg]
	res := make([]string, len(files))
	copy(res, files)
	return res
}

// PackageDependencies returns direct package dependencies.
func (pg *PersistentGraph) PackageDependencies(pkg string) []string {
	pg.mu.RLock()
	defer pg.mu.RUnlock()
	deps := pg.packageDeps[pkg]
	res := make([]string, len(deps))
	copy(res, deps)
	return res
}
