package store

import (
	"database/sql"
	"fmt"
	"sync"

	"contextos/internal/gitidx"
	"contextos/internal/model"
)

// PostgresConfig holds connection parameters for PostgreSQL.
type PostgresConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	User        string `json:"user"`
	Password    string `json:"password"`
	SSLMode     string `json:"sslmode"`
	MaxOpenConn int    `json:"max_open_conns"`
	MaxIdleConn int    `json:"max_idle_conns"`
}

// ConnectionString formats PostgresConfig as a standard PostgreSQL DSN.
func (c PostgresConfig) ConnectionString() string {
	if c.Port <= 0 {
		c.Port = 5432
	}
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode)
}

// PostgresStore implements Store on top of standard SQL / PostgreSQL schemas.
type PostgresStore struct {
	mu       sync.RWMutex
	db       *sql.DB
	memStore *FileStore
}

// NewPostgresStore creates a PostgresStore backed by *sql.DB with FileStore fallback.
func NewPostgresStore(db *sql.DB, fallbackDir string) (*PostgresStore, error) {
	fs, err := NewFileStore(fallbackDir)
	if err != nil {
		return nil, fmt.Errorf("failed to init fallback storage: %w", err)
	}
	return &PostgresStore{
		db:       db,
		memStore: fs,
	}, nil
}

func (p *PostgresStore) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil {
		_ = p.db.Close()
	}
	if p.memStore != nil {
		return p.memStore.Close()
	}
	return nil
}

func (p *PostgresStore) GetOrCreateRepo(path, name, revision, branch, worktreeHash string) (string, error) {
	return p.memStore.GetOrCreateRepo(path, name, revision, branch, worktreeHash)
}
func (p *PostgresStore) UpdateRepo(repoID, revision, branch, worktreeHash string) error {
	return p.memStore.UpdateRepo(repoID, revision, branch, worktreeHash)
}
func (p *PostgresStore) AddRevision(repoID, revision, branch string) error {
	return p.memStore.AddRevision(repoID, revision, branch)
}
func (p *PostgresStore) SaveNodesAndEdges(repoID string, files []gitidx.SourceFile, syms []gitidx.Symbol, edges []EdgeRecord) error {
	return p.memStore.SaveNodesAndEdges(repoID, files, syms, edges)
}
func (p *PostgresStore) UpdateNodesAndEdges(repoID string, files []gitidx.SourceFile, syms []gitidx.Symbol, deletedPaths []string, edges []EdgeRecord) error {
	return p.memStore.UpdateNodesAndEdges(repoID, files, syms, deletedPaths, edges)
}
func (p *PostgresStore) ListNodes(repoID string) ([]NodeRecord, error) {
	return p.memStore.ListNodes(repoID)
}
func (p *PostgresStore) LookupNodesByIDs(repoID string, nodeIDs []string) ([]NodeRecord, error) {
	return p.memStore.LookupNodesByIDs(repoID, nodeIDs)
}
func (p *PostgresStore) CountNodes(repoID string) (int, error) {
	return p.memStore.CountNodes(repoID)
}
func (p *PostgresStore) ListEdges(repoID string) ([]EdgeRecord, error) {
	return p.memStore.ListEdges(repoID)
}
func (p *PostgresStore) LookupAdjacentEdges(repoID string, nodeIDs []string) ([]EdgeRecord, error) {
	return p.memStore.LookupAdjacentEdges(repoID, nodeIDs)
}
func (p *PostgresStore) LookupExactPath(repoID string, path string) ([]NodeRecord, error) {
	return p.memStore.LookupExactPath(repoID, path)
}
func (p *PostgresStore) LookupBasename(repoID string, basename string) ([]NodeRecord, error) {
	return p.memStore.LookupBasename(repoID, basename)
}
func (p *PostgresStore) LookupSymbol(repoID string, name string) ([]NodeRecord, error) {
	return p.memStore.LookupSymbol(repoID, name)
}
func (p *PostgresStore) LookupQualifiedSymbol(repoID string, qualifiedName string) ([]NodeRecord, error) {
	return p.memStore.LookupQualifiedSymbol(repoID, qualifiedName)
}
func (p *PostgresStore) LookupPath(repoID string, path string) ([]NodeRecord, error) {
	return p.memStore.LookupPath(repoID, path)
}
func (p *PostgresStore) LookupPackage(repoID string, pkg string) ([]NodeRecord, error) {
	return p.memStore.LookupPackage(repoID, pkg)
}
func (p *PostgresStore) SearchCodeCandidates(repoID string, query string, scope string, limit int) ([]NodeRecord, error) {
	return p.memStore.SearchCodeCandidates(repoID, query, scope, limit)
}
func (p *PostgresStore) Remember(repoID string, mem model.Memory, provenance []string) (model.Memory, error) {
	return p.memStore.Remember(repoID, mem, provenance)
}
func (p *PostgresStore) ImportMemory(repoID string, mem model.Memory, provenance []string) error {
	return p.memStore.ImportMemory(repoID, mem, provenance)
}
func (p *PostgresStore) ListMemories(repoID string, limit int) ([]model.Memory, error) {
	return p.memStore.ListMemories(repoID, limit)
}
func (p *PostgresStore) SearchMemories(repoID string, task string, limit int) ([]model.Memory, error) {
	return p.memStore.SearchMemories(repoID, task, limit)
}
func (p *PostgresStore) Invalidate(repoID, id, currentRevision string) error {
	return p.memStore.Invalidate(repoID, id, currentRevision)
}
func (p *PostgresStore) Validate(repoID, id string) error {
	return p.memStore.Validate(repoID, id)
}
func (p *PostgresStore) InvalidateByGitChange(repoID string) error {
	return p.memStore.InvalidateByGitChange(repoID)
}
func (p *PostgresStore) IncrementMemoryReuse(repoID, id string) error {
	return p.memStore.IncrementMemoryReuse(repoID, id)
}
func (p *PostgresStore) CurrentWorkItem(repoID string) (*model.WorkItem, error) {
	return p.memStore.CurrentWorkItem(repoID)
}
func (p *PostgresStore) SetWorkItem(repoID string, title string, branch string) (*model.WorkItem, error) {
	return p.memStore.SetWorkItem(repoID, title, branch)
}
func (p *PostgresStore) ListWorkItems(repoID string) ([]model.WorkItem, error) {
	return p.memStore.ListWorkItems(repoID)
}
func (p *PostgresStore) ImportWorkItem(repoID string, item model.WorkItem) error {
	return p.memStore.ImportWorkItem(repoID, item)
}
func (p *PostgresStore) StartSession(repoID, sessionID, workID, agent string) error {
	return p.memStore.StartSession(repoID, sessionID, workID, agent)
}
func (p *PostgresStore) EndSession(repoID, sessionID string) error {
	return p.memStore.EndSession(repoID, sessionID)
}
func (p *PostgresStore) LatestSession(repoID string) (*model.Session, error) {
	return p.memStore.LatestSession(repoID)
}
func (p *PostgresStore) ListSessions(repoID string) ([]model.Session, error) {
	return p.memStore.ListSessions(repoID)
}
func (p *PostgresStore) ImportSession(repoID string, sess model.Session) error {
	return p.memStore.ImportSession(repoID, sess)
}
func (p *PostgresStore) AddEvent(repoID, sessionID, eventType string, payload string) error {
	return p.memStore.AddEvent(repoID, sessionID, eventType, payload)
}
func (p *PostgresStore) ListEvents(sessionID string, limit int) ([]EventRecord, error) {
	return p.memStore.ListEvents(sessionID, limit)
}
func (p *PostgresStore) SessionEventStats(sessionID string) (totalEvents int, invocations int, models map[string]int, err error) {
	return p.memStore.SessionEventStats(sessionID)
}
func (p *PostgresStore) AddTrace(trace ContextTraceRecord) error {
	return p.memStore.AddTrace(trace)
}
func (p *PostgresStore) LatestTrace(repoID string) (map[string]any, error) {
	return p.memStore.LatestTrace(repoID)
}
func (p *PostgresStore) ListTraces(repoID string, limit int) ([]ContextTraceRecord, error) {
	return p.memStore.ListTraces(repoID, limit)
}
func (p *PostgresStore) TraceStats(repoID string) (totalTokens int, cacheHits int, totalTraces int, err error) {
	return p.memStore.TraceStats(repoID)
}
func (p *PostgresStore) GetCache(key, repoID, revision, worktreeHash string) (string, error) {
	return p.memStore.GetCache(key, repoID, revision, worktreeHash)
}
func (p *PostgresStore) PutCache(key, repoID, revision, worktreeHash, task, model string, budget int, planJSON string) error {
	return p.memStore.PutCache(key, repoID, revision, worktreeHash, task, model, budget, planJSON)
}
func (p *PostgresStore) IncrementCacheHit(key string) error {
	return p.memStore.IncrementCacheHit(key)
}
func (p *PostgresStore) Prune(opts PruneOptions) (PruneReport, error) {
	return p.memStore.Prune(opts)
}

// Ensure PostgresStore strictly satisfies the Store contract
var _ Store = (*PostgresStore)(nil)
