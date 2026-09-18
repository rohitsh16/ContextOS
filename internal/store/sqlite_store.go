package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"contextos/internal/db"
	"contextos/internal/gitidx"
	"contextos/internal/model"
	"contextos/internal/textutil"
)

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func hashID(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])[:20]
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// SQLiteStore implements the Store interface on top of SQLite with WAL mode and FTS5.
type SQLiteStore struct {
	DB *db.DB
}

// NewSQLiteStore opens or initializes an SQLiteStore.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	d, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := db.Init(d); err != nil {
		d.Close()
		return nil, err
	}
	return &SQLiteStore{DB: d}, nil
}

func (s *SQLiteStore) Close() error {
	if s != nil && s.DB != nil {
		s.DB.Close()
		s.DB = nil
	}
	return nil
}

func (s *SQLiteStore) GetOrCreateRepo(path, name, revision, branch, worktreeHash string) (string, error) {
	rows, err := s.DB.Query(`SELECT id FROM repositories WHERE path=?`, path)
	if err != nil {
		return "", err
	}
	if len(rows) > 0 {
		return rows[0][0], nil
	}
	if _, err := s.DB.Exec(`INSERT INTO repositories(path,name,revision,branch,worktree_hash,indexed_at) VALUES(?,?,?,?,?,?)`, path, name, revision, branch, worktreeHash, now()); err != nil {
		return "", err
	}
	rows, err = s.DB.Query(`SELECT id FROM repositories WHERE path=?`, path)
	if err != nil || len(rows) == 0 {
		return "", fmt.Errorf("repository insert failed: %v", err)
	}
	return rows[0][0], nil
}

func (s *SQLiteStore) UpdateRepo(repoID, revision, branch, worktreeHash string) error {
	_, err := s.DB.Exec(`UPDATE repositories SET revision=?,branch=?,worktree_hash=?,indexed_at=? WHERE id=?`, revision, branch, worktreeHash, now(), repoID)
	return err
}

func (s *SQLiteStore) AddRevision(repoID, revision, branch string) error {
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO revisions(repo_id,revision,branch,observed_at) VALUES(?,?,?,?)`, repoID, revision, branch, now())
	return err
}

func (s *SQLiteStore) SaveNodesAndEdges(repoID string, files []gitidx.SourceFile, syms []gitidx.Symbol, edges []EdgeRecord) error {
	_, _ = s.DB.Exec(`DELETE FROM edges WHERE src_id IN (SELECT id FROM nodes WHERE repo_id=?) OR dst_id IN (SELECT id FROM nodes WHERE repo_id=?)`, repoID, repoID)
	_, _ = s.DB.Exec(`DELETE FROM nodes WHERE repo_id=?`, repoID)

	for _, f := range files {
		if _, err := s.DB.Exec(`INSERT OR IGNORE INTO nodes(repo_id,kind,path,name,start_line,end_line,signature,content_hash) VALUES(?,?,?,?,?,?,?,?)`, repoID, "file", f.Path, filepath.Base(f.Path), 1, f.Lines, "", f.Hash); err != nil {
			return err
		}
	}
	for _, x := range syms {
		if _, err := s.DB.Exec(`INSERT OR IGNORE INTO nodes(repo_id,kind,path,name,start_line,end_line,signature,content_hash) VALUES(?,?,?,?,?,?,?,?)`, repoID, x.Kind, x.Path, x.Name, x.Start, x.End, x.Signature, x.Hash); err != nil {
			return err
		}
	}
	for _, e := range edges {
		_, _ = s.DB.Exec(`INSERT OR IGNORE INTO edges(src_id,dst_id,kind) VALUES(?,?,?)`, e.SrcID, e.DstID, e.Kind)
	}
	return nil
}

func (s *SQLiteStore) ListNodes(repoID string) ([]NodeRecord, error) {
	rows, err := s.DB.Query(`SELECT id,kind,path,name,COALESCE(start_line,1),COALESCE(end_line,1),COALESCE(signature,''),COALESCE(content_hash,'') FROM nodes WHERE repo_id=?`, repoID)
	if err != nil {
		return nil, err
	}
	out := make([]NodeRecord, 0, len(rows))
	for _, r := range rows {
		st, _ := strconv.Atoi(r[4])
		en, _ := strconv.Atoi(r[5])
		out = append(out, NodeRecord{
			ID:          r[0],
			RepoID:      repoID,
			Kind:        r[1],
			Path:        r[2],
			Name:        r[3],
			StartLine:   st,
			EndLine:     en,
			Signature:   r[6],
			ContentHash: r[7],
		})
	}
	return out, nil
}

func (s *SQLiteStore) Remember(repoID string, mem model.Memory, provenance []string) (model.Memory, error) {
	kind := mem.Kind
	if kind == "" {
		kind = "fact"
	}
	auth := mem.Authority
	if auth == "" {
		auth = "inference"
	}
	scope := mem.Scope
	if scope == "" {
		scope = "repo"
	}
	id := mem.ID
	if id == "" {
		id = hashID(strings.ToLower(kind) + "|" + strings.TrimSpace(mem.Content))
	}
	tok := mem.TokenCost
	if tok <= 0 {
		tok = textutil.EstimateTokens(mem.Content)
	}
	tm := now()

	_, err := s.DB.Exec(`INSERT OR REPLACE INTO memories(id,repo_id,work_item_id,kind,content,scope,valid_from_revision,invalidated_at_revision,authority,confidence,token_cost,created_at,updated_at,reuse_count,last_accessed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, repoID, nil, kind, mem.Content, scope, mem.ValidFromRevision, nullIfEmpty(mem.InvalidatedAtRevision), auth, mem.Confidence, tok, tm, tm, mem.ReuseCount, nullIfEmpty(mem.LastAccessedAt))
	if err != nil {
		return model.Memory{}, err
	}
	_, _ = s.DB.Exec(`DELETE FROM memory_fts WHERE memory_id=?`, id)
	_, _ = s.DB.Exec(`INSERT INTO memory_fts(memory_id,content,kind) VALUES(?,?,?)`, id, mem.Content, kind)
	_, _ = s.DB.Exec(`DELETE FROM evidence WHERE memory_id=?`, id)
	for _, p := range provenance {
		parts := strings.SplitN(p, "|", 2)
		st, src := parts[0], p
		if len(parts) == 2 {
			src = parts[1]
		}
		_, _ = s.DB.Exec(`INSERT INTO evidence(memory_id,source,source_type,revision) VALUES(?,?,?,?)`, id, src, st, mem.ValidFromRevision)
	}
	_, _ = s.DB.Exec(`INSERT OR REPLACE INTO claims(id,memory_id,claim,authority,confidence,created_at) VALUES(?,?,?,?,?,?)`, hashID("claim|"+id+"|"+mem.Content), id, mem.Content, auth, mem.Confidence, tm)

	mem.ID = id
	mem.Kind = kind
	mem.Authority = auth
	mem.Scope = scope
	mem.TokenCost = tok
	return mem, nil
}

func (s *SQLiteStore) ListMemories(repoID string, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := s.DB.Query(`SELECT id,kind,content,scope,COALESCE(valid_from_revision,''),COALESCE(invalidated_at_revision,''),authority,confidence,token_cost,reuse_count,COALESCE(last_accessed_at,''),COALESCE((SELECT source FROM evidence e WHERE e.memory_id=memories.id ORDER BY e.id DESC LIMIT 1),'') FROM memories WHERE repo_id=? ORDER BY updated_at DESC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	return scanMemories(rows), nil
}

func (s *SQLiteStore) SearchMemories(repoID string, task string, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 64
	}
	terms := textutil.Tokens(task)
	var rows []db.Row
	var err error
	if len(terms) > 0 {
		var b strings.Builder
		for i, t := range terms {
			if i > 0 {
				b.WriteString(" OR ")
			}
			b.WriteString(t)
			b.WriteString("*")
		}
		rows, err = s.DB.Query(`SELECT m.id,m.kind,m.content,m.scope,COALESCE(m.valid_from_revision,''),COALESCE(m.invalidated_at_revision,''),m.authority,m.confidence,m.token_cost,m.reuse_count,COALESCE(m.last_accessed_at,''),COALESCE((SELECT source FROM evidence e WHERE e.memory_id=m.id ORDER BY e.id DESC LIMIT 1),'') FROM memory_fts f JOIN memories m ON f.memory_id=m.id WHERE m.repo_id=? AND memory_fts MATCH ? LIMIT ?`, repoID, b.String(), limit)
	}
	if err != nil || len(rows) == 0 {
		return s.ListMemories(repoID, limit)
	}
	return scanMemories(rows), nil
}

func scanMemories(rows []db.Row) []model.Memory {
	out := make([]model.Memory, 0, len(rows))
	for _, r := range rows {
		conf, _ := strconv.ParseFloat(r[7], 64)
		tok, _ := strconv.Atoi(r[8])
		reuse, _ := strconv.Atoi(r[9])
		src := r[11]
		if src == "" {
			src = "memory:" + r[1]
		}
		out = append(out, model.Memory{
			ID:                    r[0],
			Kind:                  r[1],
			Content:               r[2],
			Scope:                 r[3],
			ValidFromRevision:     r[4],
			InvalidatedAtRevision: r[5],
			Authority:             r[6],
			Confidence:            conf,
			TokenCost:             tok,
			ReuseCount:            reuse,
			LastAccessedAt:        r[10],
			Source:                src,
		})
	}
	return out
}

func (s *SQLiteStore) Invalidate(repoID, id, currentRevision string) error {
	_, err := s.DB.Exec(`UPDATE memories SET invalidated_at_revision=?,updated_at=? WHERE id=? AND repo_id=?`, currentRevision, now(), id, repoID)
	return err
}

func (s *SQLiteStore) InvalidateByGitChange(repoID string) error {
	_, err := s.DB.Exec(`UPDATE memories SET updated_at=? WHERE repo_id=? AND invalidated_at_revision IS NULL AND authority IN ('source','commit')`, now(), repoID)
	return err
}

func (s *SQLiteStore) IncrementMemoryReuse(repoID, id string) error {
	_, err := s.DB.Exec(`UPDATE memories SET reuse_count=reuse_count+1,last_accessed_at=? WHERE id=? AND repo_id=?`, now(), id, repoID)
	return err
}

func (s *SQLiteStore) CurrentWorkItem(repoID string) (*model.WorkItem, error) {
	rows, err := s.DB.Query(`SELECT id,title,status,branch,created_at,updated_at FROM work_items WHERE repo_id=? AND status='active' ORDER BY updated_at DESC LIMIT 1`, repoID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	return &model.WorkItem{ID: r[0], Title: r[1], Status: r[2], Branch: r[3], CreatedAt: r[4], UpdatedAt: r[5]}, nil
}

func (s *SQLiteStore) SetWorkItem(repoID string, title string, branch string) (*model.WorkItem, error) {
	tm := now()
	_, _ = s.DB.Exec(`UPDATE work_items SET status='paused',updated_at=? WHERE repo_id=? AND status='active'`, tm, repoID)
	id := hashID(title + "|" + tm)
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO work_items(id,repo_id,title,status,branch,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, id, repoID, strings.TrimSpace(title), "active", branch, tm, tm)
	if err != nil {
		return nil, err
	}
	return &model.WorkItem{ID: id, Title: strings.TrimSpace(title), Status: "active", Branch: branch, CreatedAt: tm, UpdatedAt: tm}, nil
}

func (s *SQLiteStore) StartSession(repoID, sessionID, workID, agent string) error {
	_, err := s.DB.Exec(`INSERT INTO sessions(id,repo_id,work_item_id,agent,started_at) VALUES(?,?,?,?,?)`, sessionID, repoID, nullIfEmpty(workID), agent, now())
	return err
}

func (s *SQLiteStore) EndSession(repoID, sessionID string) error {
	_, err := s.DB.Exec(`UPDATE sessions SET ended_at=? WHERE id=? AND repo_id=?`, now(), sessionID, repoID)
	return err
}

func (s *SQLiteStore) LatestSession(repoID string) (*model.Session, error) {
	rows, err := s.DB.Query(`SELECT id,agent,COALESCE(work_item_id,''),started_at,COALESCE(ended_at,'') FROM sessions WHERE repo_id=? ORDER BY started_at DESC LIMIT 1`, repoID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	return &model.Session{ID: r[0], Agent: r[1], WorkItem: r[2], StartedAt: r[3], EndedAt: r[4]}, nil
}

func (s *SQLiteStore) AddEvent(repoID, sessionID, eventType string, payload string) error {
	_, err := s.DB.Exec(`INSERT INTO events(session_id,repo_id,event_type,payload,created_at) VALUES(?,?,?,?,?)`, nullIfEmpty(sessionID), repoID, eventType, payload, now())
	return err
}

func (s *SQLiteStore) ListEvents(sessionID string, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.DB.Query(`SELECT id,COALESCE(session_id,''),repo_id,event_type,payload,created_at FROM events WHERE session_id=? ORDER BY id DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]EventRecord, 0, len(rows))
	for _, r := range rows {
		id, _ := strconv.ParseInt(r[0], 10, 64)
		out = append(out, EventRecord{ID: id, SessionID: r[1], RepoID: r[2], EventType: r[3], Payload: r[4], CreatedAt: r[5]})
	}
	return out, nil
}

func (s *SQLiteStore) AddTrace(trace ContextTraceRecord) error {
	hitInt := 0
	if trace.CacheHit {
		hitInt = 1
	}
	_, err := s.DB.Exec(`INSERT INTO context_traces(repo_id,work_item_id,task,model,budget,selected_tokens,estimated_cost,cache_hit,decision_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		trace.RepoID, nullIfEmpty(trace.WorkItemID), trace.Task, trace.Model, trace.Budget, trace.SelectedTokens, trace.EstimatedCost, hitInt, trace.DecisionJSON, now())
	return err
}

func (s *SQLiteStore) LatestTrace(repoID string) (map[string]any, error) {
	rows, err := s.DB.Query(`SELECT task,COALESCE(model,''),budget,selected_tokens,estimated_cost,cache_hit,decision_json,created_at FROM context_traces WHERE repo_id=? ORDER BY id DESC LIMIT 1`, repoID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{}, nil
	}
	r := rows[0]
	return map[string]any{"task": r[0], "model": r[1], "budget": r[2], "selected_tokens": r[3], "estimated_cost": r[4], "cache_hit": r[5], "decision_json": r[6], "created_at": r[7]}, nil
}

func (s *SQLiteStore) ImportMemory(repoID string, mem model.Memory, provenance []string) error {
	tm := now()
	tok := mem.TokenCost
	if tok <= 0 {
		tok = textutil.EstimateTokens(mem.Content)
	}
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO memories(id,repo_id,work_item_id,kind,content,scope,valid_from_revision,invalidated_at_revision,authority,confidence,token_cost,created_at,updated_at,reuse_count,last_accessed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		mem.ID, repoID, nil, mem.Kind, mem.Content, mem.Scope, mem.ValidFromRevision, nullIfEmpty(mem.InvalidatedAtRevision), mem.Authority, mem.Confidence, tok, tm, tm, mem.ReuseCount, nullIfEmpty(mem.LastAccessedAt))
	if err != nil {
		return err
	}
	_, _ = s.DB.Exec(`DELETE FROM memory_fts WHERE memory_id=?`, mem.ID)
	_, _ = s.DB.Exec(`INSERT INTO memory_fts(memory_id,content,kind) VALUES(?,?,?)`, mem.ID, mem.Content, mem.Kind)
	return nil
}

func (s *SQLiteStore) ListWorkItems(repoID string) ([]model.WorkItem, error) {
	rows, err := s.DB.Query(`SELECT id,title,status,branch,created_at,updated_at FROM work_items WHERE repo_id=? ORDER BY created_at ASC`, repoID)
	if err != nil {
		return nil, err
	}
	out := make([]model.WorkItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.WorkItem{ID: r[0], Title: r[1], Status: r[2], Branch: r[3], CreatedAt: r[4], UpdatedAt: r[5]})
	}
	return out, nil
}

func (s *SQLiteStore) ImportWorkItem(repoID string, item model.WorkItem) error {
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO work_items(id,repo_id,title,status,branch,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		item.ID, repoID, item.Title, item.Status, item.Branch, item.CreatedAt, item.UpdatedAt)
	return err
}

func (s *SQLiteStore) ListSessions(repoID string) ([]model.Session, error) {
	rows, err := s.DB.Query(`SELECT id,agent,COALESCE(work_item_id,''),started_at,COALESCE(ended_at,'') FROM sessions WHERE repo_id=? ORDER BY started_at ASC`, repoID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Session{ID: r[0], Agent: r[1], WorkItem: r[2], StartedAt: r[3], EndedAt: r[4]})
	}
	return out, nil
}

func (s *SQLiteStore) ImportSession(repoID string, sess model.Session) error {
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO sessions(id,repo_id,work_item_id,agent,started_at,ended_at) VALUES(?,?,?,?,?,?)`,
		sess.ID, repoID, nullIfEmpty(sess.WorkItem), sess.Agent, sess.StartedAt, nullIfEmpty(sess.EndedAt))
	return err
}

func (s *SQLiteStore) ListTraces(repoID string, limit int) ([]ContextTraceRecord, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.DB.Query(`SELECT id,repo_id,COALESCE(work_item_id,''),task,COALESCE(model,''),budget,selected_tokens,estimated_cost,cache_hit,decision_json,created_at FROM context_traces WHERE repo_id=? ORDER BY id ASC LIMIT ?`, repoID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ContextTraceRecord, 0, len(rows))
	for _, r := range rows {
		id, _ := strconv.ParseInt(r[0], 10, 64)
		bud, _ := strconv.Atoi(r[5])
		tok, _ := strconv.Atoi(r[6])
		cost, _ := strconv.ParseFloat(r[7], 64)
		out = append(out, ContextTraceRecord{
			ID:             id,
			RepoID:         r[1],
			WorkItemID:     r[2],
			Task:           r[3],
			Model:          r[4],
			Budget:         bud,
			SelectedTokens: tok,
			EstimatedCost:  cost,
			CacheHit:       r[8] == "1",
			DecisionJSON:   r[9],
			CreatedAt:      r[10],
		})
	}
	return out, nil
}

func (s *SQLiteStore) TraceStats(repoID string) (int, int, int, error) {
	rows, err := s.DB.Query(`SELECT COALESCE(SUM(selected_tokens),0),COALESCE(SUM(CASE WHEN cache_hit=1 THEN 1 ELSE 0 END),0),COUNT(*) FROM context_traces WHERE repo_id=?`, repoID)
	if err != nil || len(rows) == 0 {
		return 0, 0, 0, err
	}
	tok, _ := strconv.Atoi(rows[0][0])
	hit, _ := strconv.Atoi(rows[0][1])
	cnt, _ := strconv.Atoi(rows[0][2])
	return tok, hit, cnt, nil
}

func (s *SQLiteStore) GetCache(key, repoID, revision, worktreeHash string) (string, error) {
	rows, err := s.DB.Query(`SELECT plan_json FROM context_cache WHERE cache_key=? AND repo_id=? AND revision=? AND COALESCE(worktree_hash,'')=?`, key, repoID, revision, worktreeHash)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[0][0], nil
}

func (s *SQLiteStore) PutCache(key, repoID, revision, worktreeHash, task, modelName string, budget int, planJSON string) error {
	_, err := s.DB.Exec(`INSERT OR REPLACE INTO context_cache(cache_key,repo_id,revision,worktree_hash,task_hash,model,budget,plan_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		key, repoID, revision, worktreeHash, hashID(task), modelName, budget, planJSON, now())
	return err
}

func (s *SQLiteStore) IncrementCacheHit(key string) error {
	_, err := s.DB.Exec(`UPDATE context_cache SET hit_count=hit_count+1,last_hit_at=? WHERE cache_key=?`, now(), key)
	return err
}

func (s *SQLiteStore) Prune(opts PruneOptions) (PruneReport, error) {
	var rep PruneReport
	rep.DryRun = opts.DryRun

	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 7 * 24 * time.Hour
	}
	cacheCutoff := time.Now().UTC().Add(-opts.CacheTTL).Format(time.RFC3339Nano)

	// 1. Cache pruning: entries for older revisions or past TTL
	cacheRows, err := s.DB.Query(`SELECT COUNT(*) FROM context_cache WHERE repo_id=? AND (revision != ? OR created_at < ?)`, opts.RepoID, opts.CurrentRevision, cacheCutoff)
	if err == nil && len(cacheRows) > 0 {
		rep.CacheEntriesPruned, _ = strconv.Atoi(cacheRows[0][0])
	}
	if !opts.DryRun && rep.CacheEntriesPruned > 0 {
		_, _ = s.DB.Exec(`DELETE FROM context_cache WHERE repo_id=? AND (revision != ? OR created_at < ?)`, opts.RepoID, opts.CurrentRevision, cacheCutoff)
	}

	// 2. Trace pruning: keep only MaxTraces (ring buffer)
	maxTraces := opts.MaxTraces
	if maxTraces <= 0 {
		maxTraces = 500
	}
	traceRows, err := s.DB.Query(`SELECT COUNT(*) FROM context_traces WHERE repo_id=? AND id NOT IN (SELECT id FROM context_traces WHERE repo_id=? ORDER BY id DESC LIMIT ?)`, opts.RepoID, opts.RepoID, maxTraces)
	if err == nil && len(traceRows) > 0 {
		rep.TracesPruned, _ = strconv.Atoi(traceRows[0][0])
	}
	if !opts.DryRun && rep.TracesPruned > 0 {
		_, _ = s.DB.Exec(`DELETE FROM context_traces WHERE repo_id=? AND id NOT IN (SELECT id FROM context_traces WHERE repo_id=? ORDER BY id DESC LIMIT ?)`, opts.RepoID, opts.RepoID, maxTraces)
	}

	// 3. Events pruning: events older than EventTTL
	if opts.EventTTL <= 0 {
		opts.EventTTL = 30 * 24 * time.Hour
	}
	eventCutoff := time.Now().UTC().Add(-opts.EventTTL).Format(time.RFC3339Nano)
	eventRows, err := s.DB.Query(`SELECT COUNT(*) FROM events WHERE repo_id=? AND created_at < ?`, opts.RepoID, eventCutoff)
	if err == nil && len(eventRows) > 0 {
		rep.EventsPruned, _ = strconv.Atoi(eventRows[0][0])
	}
	if !opts.DryRun && rep.EventsPruned > 0 {
		_, _ = s.DB.Exec(`DELETE FROM events WHERE repo_id=? AND created_at < ?`, opts.RepoID, eventCutoff)
	}

	// 4. Stale memories pruning (only if opted in)
	if opts.PruneStaleMemories {
		if opts.StaleTTL <= 0 {
			opts.StaleTTL = 60 * 24 * time.Hour
		}
		staleCutoff := time.Now().UTC().Add(-opts.StaleTTL).Format(time.RFC3339Nano)
		memRows, err := s.DB.Query(`SELECT COUNT(*) FROM memories WHERE repo_id=? AND invalidated_at_revision IS NOT NULL AND reuse_count=0 AND updated_at < ?`, opts.RepoID, staleCutoff)
		if err == nil && len(memRows) > 0 {
			rep.MemoriesPruned, _ = strconv.Atoi(memRows[0][0])
		}
		if !opts.DryRun && rep.MemoriesPruned > 0 {
			_, _ = s.DB.Exec(`DELETE FROM memory_fts WHERE memory_id IN (SELECT id FROM memories WHERE repo_id=? AND invalidated_at_revision IS NOT NULL AND reuse_count=0 AND updated_at < ?)`, opts.RepoID, staleCutoff)
			_, _ = s.DB.Exec(`DELETE FROM evidence WHERE memory_id IN (SELECT id FROM memories WHERE repo_id=? AND invalidated_at_revision IS NOT NULL AND reuse_count=0 AND updated_at < ?)`, opts.RepoID, staleCutoff)
			_, _ = s.DB.Exec(`DELETE FROM claims WHERE memory_id IN (SELECT id FROM memories WHERE repo_id=? AND invalidated_at_revision IS NOT NULL AND reuse_count=0 AND updated_at < ?)`, opts.RepoID, staleCutoff)
			_, _ = s.DB.Exec(`DELETE FROM memories WHERE repo_id=? AND invalidated_at_revision IS NOT NULL AND reuse_count=0 AND updated_at < ?`, opts.RepoID, staleCutoff)
		}
	}

	// Reclaim free pages if not dry run
	if !opts.DryRun && (rep.CacheEntriesPruned > 0 || rep.TracesPruned > 0 || rep.EventsPruned > 0 || rep.MemoriesPruned > 0) {
		_, _ = s.DB.Exec(`PRAGMA incremental_vacuum`)
	}

	return rep, nil
}
