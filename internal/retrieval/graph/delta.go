package graph

// GraphDelta represents incremental modifications applied to a PersistentGraph (PR.md Section 16).
type GraphDelta struct {
	AddedNodes   []string `json:"added_nodes"`
	RemovedNodes []string `json:"removed_nodes"`
	AddedEdges   []Edge   `json:"added_edges"`
	RemovedEdges []Edge   `json:"removed_edges"`
}

// ComputeFileDelta calculates the incremental graph delta when a file's symbols or dependencies change.
func ComputeFileDelta(
	filePath string,
	oldSymbols []string,
	newSymbols []string,
	oldEdges []Edge,
	newEdges []Edge,
) GraphDelta {
	oldSymMap := make(map[string]bool, len(oldSymbols))
	for _, s := range oldSymbols {
		oldSymMap[s] = true
	}

	newSymMap := make(map[string]bool, len(newSymbols))
	for _, s := range newSymbols {
		newSymMap[s] = true
	}

	var addedNodes []string
	for _, s := range newSymbols {
		if !oldSymMap[s] {
			addedNodes = append(addedNodes, s)
		}
	}

	var removedNodes []string
	for _, s := range oldSymbols {
		if !newSymMap[s] {
			removedNodes = append(removedNodes, s)
		}
	}

	edgeKey := func(e Edge) string {
		return e.From + "->" + e.To + ":" + e.Type
	}

	oldEdgeMap := make(map[string]Edge, len(oldEdges))
	for _, e := range oldEdges {
		oldEdgeMap[edgeKey(e)] = e
	}

	newEdgeMap := make(map[string]Edge, len(newEdges))
	for _, e := range newEdges {
		newEdgeMap[edgeKey(e)] = e
	}

	var addedEdges []Edge
	for k, e := range newEdgeMap {
		if _, exists := oldEdgeMap[k]; !exists {
			addedEdges = append(addedEdges, e)
		}
	}

	var removedEdges []Edge
	for k, e := range oldEdgeMap {
		if _, exists := newEdgeMap[k]; !exists {
			removedEdges = append(removedEdges, e)
		}
	}

	return GraphDelta{
		AddedNodes:   addedNodes,
		RemovedNodes: removedNodes,
		AddedEdges:   addedEdges,
		RemovedEdges: removedEdges,
	}
}

// ApplyDelta applies a GraphDelta to the PersistentGraph incrementally.
func (pg *PersistentGraph) ApplyDelta(delta GraphDelta) {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	// 1. Remove nodes
	for _, nodeID := range delta.RemovedNodes {
		delete(pg.outgoing, nodeID)
		delete(pg.incoming, nodeID)
		file := pg.symbolToFile[nodeID]
		delete(pg.symbolToFile, nodeID)

		if file != "" {
			syms := pg.fileToSymbols[file]
			var updated []string
			for _, s := range syms {
				if s != nodeID {
					updated = append(updated, s)
				}
			}
			pg.fileToSymbols[file] = updated
		}
	}

	// 2. Remove edges
	for _, rem := range delta.RemovedEdges {
		// Remove from outgoing
		if outEdges, ok := pg.outgoing[rem.From]; ok {
			var updated []Edge
			for _, e := range outEdges {
				if !(e.To == rem.To && e.Type == rem.Type) {
					updated = append(updated, e)
				}
			}
			pg.outgoing[rem.From] = updated
		}
		// Remove from incoming
		if inEdges, ok := pg.incoming[rem.To]; ok {
			var updated []Edge
			for _, e := range inEdges {
				if !(e.From == rem.From && e.Type == rem.Type) {
					updated = append(updated, e)
				}
			}
			pg.incoming[rem.To] = updated
		}
	}

	// 3. Add edges
	for _, add := range delta.AddedEdges {
		if add.Weight == 0 {
			add.Weight = 1.0
		}
		pg.outgoing[add.From] = append(pg.outgoing[add.From], add)
		pg.incoming[add.To] = append(pg.incoming[add.To], add)

		if add.Type == "imports" {
			pg.packageDeps[add.From] = append(pg.packageDeps[add.From], add.To)
		}
	}
}
