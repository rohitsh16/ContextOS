package retrieval

import (
	"os"
	"strings"
)

// SparseExpansion maps high-level natural language intent phrases to weighted sparse code keywords (PR.md Section 21).
type SparseExpansion struct {
	ConceptTerms map[string][]string
	TermWeights  map[string]float64
}

// DefaultSparseDictionary maps operational intents to domain code concepts.
var DefaultSparseDictionary = map[string][]string{
	"failover":       {"recovery", "fallback", "replica", "backup", "disaster", "standby"},
	"recovery":       {"restore", "snapshot", "replicate", "failover", "wal", "checkpoint"},
	"regional":       {"datacenter", "zone", "cluster", "distributed", "geo", "remote"},
	"persistence":    {"store", "sqlite", "wal", "disk", "write", "fsync", "db"},
	"reconciliation": {"sync", "diff", "delta", "merge", "resolve", "conflict"},
	"timeout":        {"deadline", "context", "cancel", "retry", "backoff", "duration"},
	"cache":          {"ttl", "evict", "lru", "invalidate", "entry", "stale"},
	"concurrency":    {"mutex", "lock", "atomic", "channel", "semaphore", "routine"},
}

// SparseExpander expands natural language queries into weighted sparse terms for inverted index retrieval.
type SparseExpander struct {
	enabled    bool
	dictionary map[string][]string
}

// NewSparseExpander initializes the sparse expander, checking CONTEXTOS_SPARSE_RETRIEVAL.
func NewSparseExpander() *SparseExpander {
	flag := os.Getenv("CONTEXTOS_SPARSE_RETRIEVAL")
	enabled := flag == "1" || strings.ToLower(flag) == "true"
	return &SparseExpander{
		enabled:    enabled,
		dictionary: DefaultSparseDictionary,
	}
}

// IsEnabled returns whether learned sparse expansion is active.
func (se *SparseExpander) IsEnabled() bool {
	return se.enabled
}

// SetEnabled dynamically enables or disables sparse expansion.
func (se *SparseExpander) SetEnabled(enabled bool) {
	se.enabled = enabled
}

// Expand extracts and expands concept terms from natural language queries.
func (se *SparseExpander) Expand(queryText string) map[string]float64 {
	expanded := make(map[string]float64)
	tokens := strings.Fields(strings.ToLower(queryText))

	for _, t := range tokens {
		clean := strings.Trim(t, `.,!?;:"'()[]{}<>-`)
		if clean == "" {
			continue
		}
		// Direct query term
		expanded[clean] = 1.0

		// Expand synonyms/concepts if dictionary matches
		if synonyms, found := se.dictionary[clean]; found {
			for _, syn := range synonyms {
				if _, exists := expanded[syn]; !exists {
					expanded[syn] = 0.65 // discounted expansion weight
				}
			}
		}
	}

	return expanded
}
