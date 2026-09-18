package store

import (
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/model"
)

// NodeRecord represents an AST symbol or source file entry.
type NodeRecord struct {
	ID          string `json:"id"`
	RepoID      string `json:"repo_id"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Signature   string `json:"signature"`
	ContentHash string `json:"content_hash"`
}

// EdgeRecord represents a dependency, import, or call edge between nodes.
type EdgeRecord struct {
	SrcID string `json:"src_id"`
	DstID string `json:"dst_id"`
	Kind  string `json:"kind"`
}

// EventRecord represents an agent lifecycle or observation event.
type EventRecord struct {
	ID        int64  `json:"id"`
	SessionID string `json:"session_id,omitempty"`
	RepoID    string `json:"repo_id"`
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

// ContextTraceRecord captures diagnostic telemetry for a context planning operation.
type ContextTraceRecord struct {
	ID             int64   `json:"id"`
	RepoID         string  `json:"repo_id"`
	WorkItemID     string  `json:"work_item_id,omitempty"`
	Task           string  `json:"task"`
	Model          string  `json:"model,omitempty"`
	Budget         int     `json:"budget"`
	SelectedTokens int     `json:"selected_tokens"`
	EstimatedCost  float64 `json:"estimated_cost"`
	CacheHit       bool    `json:"cache_hit"`
	DecisionJSON   string  `json:"decision_json"`
	CreatedAt      string  `json:"created_at"`
}

// PruneOptions configures garbage collection and storage retention.
type PruneOptions struct {
	RepoID             string        // Target repository ID
	CurrentRevision    string        // Active HEAD commit hash
	CacheTTL           time.Duration // Retain unhit cache entries for old revisions (default: 7 days)
	MaxTraces          int           // Maximum traces to keep per repository (default: 500 ring buffer)
	EventTTL           time.Duration // Retain ephemeral events (default: 30 days)
	PruneStaleMemories bool          // Whether to purge hard-stale memories with zero reuse older than StaleTTL
	StaleTTL           time.Duration // Retention window for unreferenced hard-stale memories (default: 60 days)
	DryRun             bool          // When true, calculate reclaimable items without deleting
}

// DefaultPruneOptions returns sensible production defaults for storage management.
func DefaultPruneOptions(repoID, currentRevision string) PruneOptions {
	return PruneOptions{
		RepoID:             repoID,
		CurrentRevision:    currentRevision,
		CacheTTL:           7 * 24 * time.Hour,
		MaxTraces:          500,
		EventTTL:           30 * 24 * time.Hour,
		PruneStaleMemories: false,
		StaleTTL:           60 * 24 * time.Hour,
		DryRun:             false,
	}
}

// PruneReport summarizes the outcome of a garbage collection pass.
type PruneReport struct {
	CacheEntriesPruned int  `json:"cache_entries_pruned"`
	TracesPruned       int  `json:"traces_pruned"`
	EventsPruned       int  `json:"events_pruned"`
	MemoriesPruned     int  `json:"memories_pruned"`
	DryRun             bool `json:"dry_run"`
}

// Store defines the storage engine contract for ContextOS.
// It is implemented by SQLiteStore (full SQLite WAL + FTS5) and FileStore (pure Go JSON/JSONL).
type Store interface {
	Close() error

	// Repository & Revisions
	GetOrCreateRepo(path, name, revision, branch, worktreeHash string) (string, error)
	UpdateRepo(repoID, revision, branch, worktreeHash string) error
	AddRevision(repoID, revision, branch string) error

	// Nodes & Edges (AST symbol indexing)
	SaveNodesAndEdges(repoID string, files []gitidx.SourceFile, syms []gitidx.Symbol, edges []EdgeRecord) error
	ListNodes(repoID string) ([]NodeRecord, error)

	// Typed Memories
	Remember(repoID string, mem model.Memory, provenance []string) (model.Memory, error)
	ImportMemory(repoID string, mem model.Memory, provenance []string) error
	ListMemories(repoID string, limit int) ([]model.Memory, error)
	SearchMemories(repoID string, task string, limit int) ([]model.Memory, error)
	Invalidate(repoID, id, currentRevision string) error
	InvalidateByGitChange(repoID string) error
	IncrementMemoryReuse(repoID, id string) error

	// WorkItems & Sessions
	CurrentWorkItem(repoID string) (*model.WorkItem, error)
	SetWorkItem(repoID string, title string, branch string) (*model.WorkItem, error)
	ListWorkItems(repoID string) ([]model.WorkItem, error)
	ImportWorkItem(repoID string, item model.WorkItem) error
	StartSession(repoID, sessionID, workID, agent string) error
	EndSession(repoID, sessionID string) error
	LatestSession(repoID string) (*model.Session, error)
	ListSessions(repoID string) ([]model.Session, error)
	ImportSession(repoID string, sess model.Session) error

	// Events & Traces
	AddEvent(repoID, sessionID, eventType string, payload string) error
	ListEvents(sessionID string, limit int) ([]EventRecord, error)
	AddTrace(trace ContextTraceRecord) error
	LatestTrace(repoID string) (map[string]any, error)
	ListTraces(repoID string, limit int) ([]ContextTraceRecord, error)
	TraceStats(repoID string) (totalTokens int, cacheHits int, totalTraces int, err error)

	// Context Plan Cache
	GetCache(key, repoID, revision, worktreeHash string) (string, error)
	PutCache(key, repoID, revision, worktreeHash, task, model string, budget int, planJSON string) error
	IncrementCacheHit(key string) error

	// Storage Management / Garbage Collection
	Prune(opts PruneOptions) (PruneReport, error)
}
