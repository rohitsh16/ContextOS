package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// CachedContext represents a compiled prompt prefix with tracked dependencies (PR.md Section 27).
type CachedContext struct {
	Key          string        `json:"key"`
	PrefixHash   string        `json:"prefix_hash"`
	Units        []ContextUnit `json:"units"`
	TotalTokens  int           `json:"total_tokens"`
	Dependencies map[string]bool `json:"dependencies"` // All file paths referenced by this context
	CreatedAt    time.Time     `json:"created_at"`
	HitCount     int           `json:"hit_count"`
}

// ContextCache provides dependency-aware context prefix caching (PR.md Section 28).
type ContextCache struct {
	mu      sync.RWMutex
	entries map[string]*CachedContext
	maxSize int
}

// NewContextCache initializes a ContextCache with maximum entry capacity.
func NewContextCache(maxSize int) *ContextCache {
	if maxSize <= 0 {
		maxSize = 256
	}
	return &ContextCache{
		entries: make(map[string]*CachedContext),
		maxSize: maxSize,
	}
}

// Put caches compiled ContextUnits, indexing their file dependencies.
func (cc *ContextCache) Put(key string, units []ContextUnit) *CachedContext {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Evict oldest if capacity exceeded
	if len(cc.entries) >= cc.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range cc.entries {
			if oldestTime.IsZero() || v.CreatedAt.Before(oldestTime) {
				oldestTime = v.CreatedAt
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(cc.entries, oldestKey)
		}
	}

	deps := make(map[string]bool)
	var totalTokens int
	h := sha256.New()

	for _, u := range units {
		totalTokens += u.Tokens
		h.Write([]byte(u.ID))
		if u.Path != "" {
			deps[u.Path] = true
		}
		for _, d := range u.Dependencies {
			deps[d] = true
		}
	}

	entry := &CachedContext{
		Key:          key,
		PrefixHash:   hex.EncodeToString(h.Sum(nil))[:16],
		Units:        units,
		TotalTokens:  totalTokens,
		Dependencies: deps,
		CreatedAt:    time.Now(),
		HitCount:     1,
	}
	cc.entries[key] = entry
	return entry
}

// Get retrieves a cached context by key if present.
func (cc *ContextCache) Get(key string) (*CachedContext, bool) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	entry, exists := cc.entries[key]
	if !exists {
		return nil, false
	}
	entry.HitCount++
	return entry, true
}

// InvalidateModifiedFiles removes cached contexts whose dependency closure intersects changedFiles (PR.md Section 28).
// Returns the number of invalidated cache entries.
func (cc *ContextCache) InvalidateModifiedFiles(changedFiles []string) int {
	if len(changedFiles) == 0 {
		return 0
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	changedSet := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	var toDelete []string
	for key, entry := range cc.entries {
		invalid := false
		for dep := range entry.Dependencies {
			if changedSet[dep] {
				invalid = true
				break
			}
			for cf := range changedSet {
				if strings.HasSuffix(dep, cf) || strings.HasSuffix(cf, dep) {
					invalid = true
					break
				}
			}
			if invalid {
				break
			}
		}
		if invalid {
			toDelete = append(toDelete, key)
		}
	}

	for _, k := range toDelete {
		delete(cc.entries, k)
	}

	return len(toDelete)
}

// SortUnitsForCache orders ContextUnits to maximize prefix stability (PR.md Section 27).
// Stable definitions and system rules come first (STABLE PREFIX); dynamic snippets come last.
func SortUnitsForCache(units []ContextUnit) []ContextUnit {
	ordered := make([]ContextUnit, len(units))
	copy(ordered, units)

	typeRank := func(t ContextUnitType) int {
		switch t {
		case UnitSignature:
			return 1
		case UnitDefinition:
			return 2
		case UnitDependency:
			return 3
		case UnitDecision:
			return 4
		case UnitEvidence:
			return 5
		case UnitSnippet:
			return 6
		default:
			return 7
		}
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		rI := typeRank(ordered[i].Type)
		rJ := typeRank(ordered[j].Type)
		if rI != rJ {
			return rI < rJ
		}
		// Alphabetical stability within the same rank
		return ordered[i].Path < ordered[j].Path
	})

	return ordered
}
