package agent

import (
	"sort"
	"sync"
)

var (
	registryMu sync.RWMutex
	adapters   = make(map[string]AgentAdapter)
)

// Register adds an AgentAdapter to the global registry.
func Register(a AgentAdapter) {
	registryMu.Lock()
	defer registryMu.Unlock()
	adapters[a.ID()] = a
}

// Get looks up an adapter by its ID (e.g. "claude", "cursor").
func Get(id string) (AgentAdapter, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := adapters[id]
	return a, ok
}

// List returns all registered adapters sorted by ID.
func List() []AgentAdapter {
	registryMu.RLock()
	defer registryMu.RUnlock()
	res := make([]AgentAdapter, 0, len(adapters))
	for _, a := range adapters {
		res = append(res, a)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID() < res[j].ID()
	})
	return res
}

// AllIDs returns the IDs of all registered adapters sorted alphabetically.
func AllIDs() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	ids := make([]string, 0, len(adapters))
	for id := range adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// DetectAll runs detection across all registered adapters.
func DetectAll(ctx DetectContext) map[string]DetectionResult {
	all := List()
	results := make(map[string]DetectionResult, len(all))
	for _, a := range all {
		results[a.ID()] = a.Detect(ctx)
	}
	return results
}
