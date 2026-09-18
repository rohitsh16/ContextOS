package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"contextos/internal/allocator"
	"contextos/internal/db"
	"contextos/internal/gitidx"
	"contextos/internal/model"
	"contextos/internal/router"
	"contextos/internal/textutil"
)

type Service struct {
	DB     *db.DB
	Repo   gitidx.Repo
	RepoID string
}

func New(dbPath, repoPath string) (*Service, error) {
	d, e := db.Open(dbPath)
	if e != nil {
		return nil, e
	}
	if e = db.Init(d); e != nil {
		d.Close()
		return nil, e
	}
	r, e := gitidx.Detect(repoPath)
	if e != nil {
		d.Close()
		return nil, e
	}
	s := &Service{DB: d, Repo: r}
	rows, e := d.Query(`SELECT id FROM repositories WHERE path=?`, r.Path)
	if e != nil {
		s.Close()
		return nil, e
	}
	if len(rows) > 0 {
		s.RepoID = rows[0][0]
	} else {
		if _, e = d.Exec(`INSERT INTO repositories(path,name,revision,branch,worktree_hash,indexed_at) VALUES(?,?,?,?,?,?)`, r.Path, r.Name, r.Revision, r.Branch, r.WorktreeHash, now()); e != nil {
			s.Close()
			return nil, e
		}
		rows, e = d.Query(`SELECT id FROM repositories WHERE path=?`, r.Path)
		if e != nil || len(rows) == 0 {
			s.Close()
			return nil, fmt.Errorf("repository insert failed: %v", e)
		}
		s.RepoID = rows[0][0]
	}
	_, _ = d.Exec(`INSERT OR IGNORE INTO revisions(repo_id,revision,branch,observed_at) VALUES(?,?,?,?)`, s.RepoID, r.Revision, r.Branch, now())
	return s, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (s *Service) Close() {
	if s != nil && s.DB != nil {
		s.DB.Close()
	}
}

func (s *Service) RefreshRepo() error {
	old := s.Repo
	r, e := gitidx.Detect(s.Repo.Path)
	if e != nil {
		return e
	}
	s.Repo = r
	_, e = s.DB.Exec(`UPDATE repositories SET revision=?,branch=?,worktree_hash=?,indexed_at=? WHERE id=?`, r.Revision, r.Branch, r.WorktreeHash, now(), s.RepoID)
	if e == nil {
		_, _ = s.DB.Exec(`INSERT OR IGNORE INTO revisions(repo_id,revision,branch,observed_at) VALUES(?,?,?,?)`, s.RepoID, r.Revision, r.Branch, now())
	}
	if e == nil && (old.Revision != r.Revision || old.Branch != r.Branch) {
		_ = s.InvalidateByGitChange()
	}
	return e
}

func (s *Service) Index() error {
	if e := s.RefreshRepo(); e != nil {
		return e
	}
	syms, e := gitidx.WalkSymbols(s.Repo.Path)
	if e != nil {
		return e
	}
	_, _ = s.DB.Exec(`DELETE FROM edges WHERE src_id IN (SELECT id FROM nodes WHERE repo_id=?) OR dst_id IN (SELECT id FROM nodes WHERE repo_id=?)`, s.RepoID, s.RepoID)
	_, _ = s.DB.Exec(`DELETE FROM nodes WHERE repo_id=?`, s.RepoID)
	// Add file nodes first.
	files, _ := gitidx.ListSourceFiles(s.Repo.Path)
	for _, f := range files {
		_, e = s.DB.Exec(`INSERT OR IGNORE INTO nodes(repo_id,kind,path,name,start_line,end_line,signature,content_hash) VALUES(?,?,?,?,?,?,?,?)`, s.RepoID, "file", f.Path, filepath.Base(f.Path), 1, f.Lines, "", f.Hash)
		if e != nil {
			return e
		}
	}
	for _, x := range syms {
		_, e = s.DB.Exec(`INSERT OR IGNORE INTO nodes(repo_id,kind,path,name,start_line,end_line,signature,content_hash) VALUES(?,?,?,?,?,?,?,?)`, s.RepoID, x.Kind, x.Path, x.Name, x.Start, x.End, x.Signature, x.Hash)
		if e != nil {
			return e
		}
	}
	// Lightweight deterministic import/dependency edges.
	_ = s.buildEdges()
	return nil
}

func (s *Service) buildEdges() error {
	rows, e := s.DB.Query(`SELECT id,kind,path,name FROM nodes WHERE repo_id=?`, s.RepoID)
	if e != nil {
		return e
	}
	fileByBase := map[string]string{}
	for _, r := range rows {
		if len(r) >= 4 && r[1] == "file" {
			fileByBase[r[3]] = r[0]
			fileByBase[r[2]] = r[0]
		}
	}
	for _, r := range rows {
		if len(r) < 4 || r[1] != "file" {
			continue
		}
		p := filepath.Join(s.Repo.Path, filepath.FromSlash(r[2]))
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		txt := string(b)
		for base, dst := range fileByBase {
			if base == r[3] || base == r[2] {
				continue
			}
			if strings.Contains(txt, base) {
				_, _ = s.DB.Exec(`INSERT OR IGNORE INTO edges(src_id,dst_id,kind) VALUES(?,?,?)`, r[0], dst, "references")
			}
		}
	}
	return nil
}

func (s *Service) Remember(kind, content, authority, scope, workID string, confidence float64, provenance []string) (model.Memory, error) {
	if kind == "" {
		kind = "fact"
	}
	if authority == "" {
		authority = "inference"
	}
	if scope == "" {
		scope = "repo"
	}
	id := hashID(strings.ToLower(kind) + "|" + strings.TrimSpace(content))
	t := textutil.EstimateTokens(content)
	tm := now()
	_, e := s.DB.Exec(`INSERT OR REPLACE INTO memories(id,repo_id,work_item_id,kind,content,scope,valid_from_revision,invalidated_at_revision,authority,confidence,token_cost,created_at,updated_at,reuse_count,last_accessed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, s.RepoID, nullIfEmpty(workID), kind, content, scope, s.Repo.Revision, nil, authority, confidence, t, tm, tm, 0, nil)
	if e != nil {
		return model.Memory{}, e
	}
	_, _ = s.DB.Exec(`DELETE FROM memory_fts WHERE memory_id=?`, id)
	_, _ = s.DB.Exec(`INSERT INTO memory_fts(memory_id,content,kind) VALUES(?,?,?)`, id, content, kind)
	_, _ = s.DB.Exec(`DELETE FROM evidence WHERE memory_id=?`, id)
	for _, p := range provenance {
		parts := strings.SplitN(p, "|", 2)
		st, src := parts[0], p
		if len(parts) == 2 {
			src = parts[1]
		}
		_, _ = s.DB.Exec(`INSERT INTO evidence(memory_id,source,source_type,revision) VALUES(?,?,?,?)`, id, src, st, s.Repo.Revision)
	}
	_, _ = s.DB.Exec(`INSERT OR REPLACE INTO claims(id,memory_id,claim,authority,confidence,created_at) VALUES(?,?,?,?,?,?)`, hashID("claim|"+id+"|"+content), id, content, authority, confidence, tm)
	return model.Memory{ID: id, Kind: kind, Content: content, Scope: scope, ValidFromRevision: s.Repo.Revision, Authority: authority, Confidence: confidence, TokenCost: t}, nil
}

func hashID(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:])[:20] }
func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func (s *Service) CurrentWorkItem() (*model.WorkItem, error) {
	rows, e := s.DB.Query(`SELECT id,title,status,branch,created_at,updated_at FROM work_items WHERE repo_id=? AND status='active' ORDER BY updated_at DESC LIMIT 1`, s.RepoID)
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	return &model.WorkItem{ID: r[0], Title: r[1], Status: r[2], Branch: r[3], CreatedAt: r[4], UpdatedAt: r[5]}, nil
}
func (s *Service) StartWorkItem(title string) (model.WorkItem, error) {
	if strings.TrimSpace(title) == "" {
		return model.WorkItem{}, fmt.Errorf("work item title is required")
	}
	tm := now()
	id := hashID(s.RepoID + "|work|" + strings.TrimSpace(title))
	_, e := s.DB.Exec(`UPDATE work_items SET status='paused',updated_at=? WHERE repo_id=? AND status='active'`, tm, s.RepoID)
	if e != nil {
		return model.WorkItem{}, e
	}
	_, e = s.DB.Exec(`INSERT OR REPLACE INTO work_items(id,repo_id,title,status,branch,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, id, s.RepoID, strings.TrimSpace(title), "active", s.Repo.Branch, tm, tm)
	if e != nil {
		return model.WorkItem{}, e
	}
	return model.WorkItem{ID: id, Title: title, Status: "active", Branch: s.Repo.Branch, CreatedAt: tm, UpdatedAt: tm}, nil
}
func (s *Service) StartSession(agent, workID string) (model.Session, error) {
	if agent == "" {
		agent = "unknown"
	}
	tm := now()
	id := hashID(s.RepoID + "|session|" + agent + "|" + tm)
	_, e := s.DB.Exec(`INSERT INTO sessions(id,repo_id,work_item_id,agent,started_at) VALUES(?,?,?,?,?)`, id, s.RepoID, nullIfEmpty(workID), agent, tm)
	if e != nil {
		return model.Session{}, e
	}
	return model.Session{ID: id, Agent: agent, WorkItem: workID, StartedAt: tm}, nil
}
func (s *Service) EndSession(id string) error {
	_, e := s.DB.Exec(`UPDATE sessions SET ended_at=? WHERE id=? AND repo_id=?`, now(), id, s.RepoID)
	return e
}
func (s *Service) LatestSession() (*model.Session, error) {
	rows, e := s.DB.Query(`SELECT id,agent,COALESCE(work_item_id,''),started_at,COALESCE(ended_at,'') FROM sessions WHERE repo_id=? ORDER BY started_at DESC LIMIT 1`, s.RepoID)
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, nil
	}
	r := rows[0]
	return &model.Session{ID: r[0], Agent: r[1], WorkItem: r[2], StartedAt: r[3], EndedAt: r[4]}, nil
}
func (s *Service) RecordEvent(sessionID, eventType string, payload any) error {
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT INTO events(session_id,repo_id,event_type,payload,created_at) VALUES(?,?,?,?,?)`, nullIfEmpty(sessionID), s.RepoID, eventType, string(b), now())
	return e
}

func (s *Service) IngestHook(ev model.HookEvent) error {
	wi, _ := s.CurrentWorkItem()
	if wi == nil && strings.TrimSpace(ev.Prompt) != "" && strings.Contains(strings.ToLower(ev.EventType), "prompt") {
		title := strings.TrimSpace(ev.Prompt)
		if len(title) > 120 {
			title = title[:120] + "…"
		}
		w, e := s.StartWorkItem(title)
		if e == nil {
			wi = &w
		}
	}
	var ss *model.Session
	if ev.SessionID != "" {
		if rows, err := s.DB.Query(`SELECT id,agent,COALESCE(work_item_id,''),started_at,COALESCE(ended_at,'') FROM sessions WHERE id=? AND repo_id=? LIMIT 1`, ev.SessionID, s.RepoID); err == nil && len(rows) > 0 {
			r := rows[0]
			ss = &model.Session{ID: r[0], Agent: r[1], WorkItem: r[2], StartedAt: r[3], EndedAt: r[4]}
		}
	}
	if ss == nil {
		ss, _ = s.LatestSession()
	}
	if ss == nil || (ss.EndedAt != "") || (ss.Agent != ev.Agent) {
		var wid string
		if wi != nil {
			wid = wi.ID
		}
		x, e := s.StartSession(ev.Agent, wid)
		if e != nil {
			return e
		}
		ss = &x
	}
	if e := s.RecordEvent(ss.ID, ev.EventType, ev.Raw); e != nil {
		return e
	}
	if strings.Contains(strings.ToLower(ev.EventType), "failure") || ev.IsFailure {
		text := ev.Output
		if text == "" {
			text = flattenPayload(ev.Raw)
		}
		if len(text) > 800 {
			text = text[:800] + "…"
		}
		if strings.TrimSpace(text) != "" {
			_, _ = s.Remember("failure", fmt.Sprintf("%s failed: %s", ev.ToolName, text), "source", "repo", ss.WorkItem, 0.92, []string{"hook|" + ev.EventType})
		}
	}
	if strings.Contains(strings.ToLower(ev.EventType), "prompt") && strings.TrimSpace(ev.Prompt) != "" {
		// Preserve the user intent as an observation, but don't flood memory with duplicate prompts.
		p := strings.TrimSpace(ev.Prompt)
		if len(p) > 500 {
			p = p[:500] + "…"
		}
		_, _ = s.Remember("observation", "User task: "+p, "user", "repo", ss.WorkItem, 1.0, nil)
	}
	if strings.EqualFold(ev.EventType, "SessionEnd") {
		_ = s.ConsolidateHeuristics(ss.ID)
		_ = s.EndSession(ss.ID)
	} else if strings.EqualFold(ev.EventType, "Stop") || strings.EqualFold(ev.EventType, "AfterAgent") {
		_ = s.ConsolidateHeuristics(ss.ID)
	}
	return nil
}

func flattenPayload(m map[string]any) string { b, _ := json.Marshal(m); return string(b) }

func (s *Service) ConsolidateHeuristics(sessionID string) error {
	rows, e := s.DB.Query(`SELECT event_type,payload FROM events WHERE session_id=? ORDER BY id DESC LIMIT 30`, sessionID)
	if e != nil {
		return e
	}
	workID := ""
	if wr, err := s.DB.Query(`SELECT COALESCE(work_item_id,'') FROM sessions WHERE id=? AND repo_id=? LIMIT 1`, sessionID, s.RepoID); err == nil && len(wr) > 0 {
		workID = wr[0][0]
	}
	for _, r := range rows {
		if len(r) < 2 {
			continue
		}
		p := r[1]
		lp := strings.ToLower(p)
		if strings.Contains(lp, "we decided") || strings.Contains(lp, "decision:") || strings.Contains(lp, "use outbox") || strings.Contains(lp, "use kafka") {
			txt := p
			if len(txt) > 700 {
				txt = txt[:700] + "…"
			}
			_, _ = s.Remember("decision", txt, "inference", "repo", workID, 0.72, []string{"session_event|" + r[0]})
		}
	}
	return nil
}

func (s *Service) SearchCandidates(task string, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 100
	}
	base := `SELECT id,kind,content,scope,COALESCE(valid_from_revision,''),COALESCE(invalidated_at_revision,''),authority,confidence,token_cost,reuse_count,COALESCE(last_accessed_at,''),COALESCE((SELECT source FROM evidence e WHERE e.memory_id=memories.id ORDER BY e.id DESC LIMIT 1),'') FROM memories`
	args := []any{}
	if strings.TrimSpace(task) != "" {
		fts := ftsQuery(task)
		if fts != "" {
			base += ` WHERE repo_id=? AND id IN (SELECT memory_id FROM memory_fts WHERE memory_fts MATCH ?) ORDER BY updated_at DESC LIMIT ?`
			args = []any{s.RepoID, fts, limit}
		} else {
			base += ` WHERE repo_id=? ORDER BY updated_at DESC LIMIT ?`
			args = []any{s.RepoID, limit}
		}
	} else {
		base += ` WHERE repo_id=? ORDER BY updated_at DESC LIMIT ?`
		args = []any{s.RepoID, limit}
	}
	rows, e := s.DB.Query(base, args...)
	if e != nil {
		// FTS is an accelerator, not a correctness dependency: fall back to a bounded scan.
		rows, e = s.DB.Query(`SELECT id,kind,content,scope,COALESCE(valid_from_revision,''),COALESCE(invalidated_at_revision,''),authority,confidence,token_cost,reuse_count,COALESCE(last_accessed_at,''),COALESCE((SELECT source FROM evidence e WHERE e.memory_id=memories.id ORDER BY e.id DESC LIMIT 1),'') FROM memories WHERE repo_id=? ORDER BY updated_at DESC LIMIT ?`, s.RepoID, limit)
	}
	if e != nil {
		return nil, e
	}
	out := make([]model.Memory, 0, len(rows))
	for _, r := range rows {
		if len(r) < 12 {
			continue
		}
		// The current user prompt is captured for auditability, but should not
		// crowd its own context plan with an echo of the request.
		if strings.HasPrefix(r[2], "User task: ") {
			continue
		}
		conf, _ := strconv.ParseFloat(r[7], 64)
		tok, _ := strconv.Atoi(r[8])
		reuse, _ := strconv.Atoi(r[9])
		src := r[11]
		if src == "" {
			src = "memory"
		}
		out = append(out, model.Memory{ID: r[0], Kind: r[1], Content: r[2], Scope: r[3], ValidFromRevision: r[4], InvalidatedAtRevision: r[5], Authority: r[6], Confidence: conf, TokenCost: tok, ReuseCount: reuse, LastAccessedAt: r[10], Source: src})
	}
	return out, nil
}

func ftsQuery(task string) string {
	fields := strings.Fields(strings.ToLower(task))
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "\"'`.,:;!?()[]{}")
		if f == "" || len(f) < 2 {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, "")+`"`)
	}
	return strings.Join(parts, " OR ")
}

func (s *Service) CodeMemories(task string, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 150
	}
	rows, e := s.DB.Query(`SELECT id,kind,path,name,COALESCE(start_line,1),COALESCE(end_line,1),COALESCE(signature,''),COALESCE(content_hash,'') FROM nodes WHERE repo_id=?`, s.RepoID)
	if e != nil {
		return nil, e
	}
	type scored struct {
		m     model.Memory
		score float64
	}
	tmp := []scored{}
	for _, r := range rows {
		if len(r) < 8 {
			continue
		}
		content := fmt.Sprintf("%s %s %s %s:%s", r[1], r[3], r[6], r[2], r[4])
		sc := 0.6*textutil.HashSemantic(task, content) + 0.4*textutil.Overlap(task, content)
		if sc < 0.05 && task != "" {
			continue
		}
		tmp = append(tmp, scored{model.Memory{ID: "node:" + r[0], Kind: "code", Content: content, Scope: "repo", ValidFromRevision: s.Repo.Revision, Authority: "source", Confidence: 1, TokenCost: textutil.EstimateTokens(content), Source: "repository", Location: r[2]}, sc})
	}
	sort.Slice(tmp, func(i, j int) bool { return tmp[i].score > tmp[j].score })
	if len(tmp) > limit {
		tmp = tmp[:limit]
	}
	out := make([]model.Memory, 0, len(tmp))
	for _, x := range tmp {
		out = append(out, x.m)
	}
	return out, nil
}

func (s *Service) Plan(task, modelName string, budget int) (model.ContextPlan, error) {
	_ = s.RefreshRepo()
	if budget <= 0 {
		budget = 4000
	}
	if modelName == "" {
		modelName = router.Recommend(task, budget).Name
	}
	key := hashID(fmt.Sprintf("%s|%s|%s|%s|%d", s.Repo.Revision, s.Repo.WorktreeHash, task, modelName, budget))
	rows, e := s.DB.Query(`SELECT plan_json FROM context_cache WHERE cache_key=? AND repo_id=? AND revision=? AND COALESCE(worktree_hash,'')=?`, key, s.RepoID, s.Repo.Revision, s.Repo.WorktreeHash)
	if e != nil {
		return model.ContextPlan{}, e
	}
	if len(rows) > 0 {
		var p model.ContextPlan
		if json.Unmarshal([]byte(rows[0][0]), &p) == nil {
			_, _ = s.DB.Exec(`UPDATE context_cache SET hit_count=hit_count+1,last_hit_at=? WHERE cache_key=?`, now(), key)
			p.CacheHit = true
			p.CreatedAt = time.Now().UTC()
			_ = s.trace(task, modelName, budget, p, true)
			return p, nil
		}
	}
	ms, e := s.SearchCandidates(task, 200)
	if e != nil {
		return model.ContextPlan{}, e
	}
	codes, e := s.CodeMemories(task, 100)
	if e != nil {
		return model.ContextPlan{}, e
	}
	ms = append(ms, codes...)
	p := allocator.Plan(allocator.Request{Task: task, Budget: budget, Model: modelName, RepoRevision: s.Repo.Revision}, ms)
	p.Model = modelName
	p.CreatedAt = time.Now().UTC()
	prof := profile(modelName)
	p.EstimatedCost = float64(p.SelectedTokens) * prof.InputPerM / 1e6
	b, _ := json.Marshal(p)
	_, _ = s.DB.Exec(`INSERT OR REPLACE INTO context_cache(cache_key,repo_id,revision,worktree_hash,task_hash,model,budget,plan_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, key, s.RepoID, s.Repo.Revision, s.Repo.WorktreeHash, hashID(task), modelName, budget, string(b), now())
	_ = s.trace(task, modelName, budget, p, false)
	for _, c := range p.Selected {
		if strings.HasPrefix(c.ID, "node:") {
			continue
		}
		_, _ = s.DB.Exec(`UPDATE memories SET reuse_count=reuse_count+1,last_accessed_at=? WHERE id=? AND repo_id=?`, now(), c.ID, s.RepoID)
	}
	return p, nil
}

func profile(name string) router.ModelProfile {
	for _, p := range router.Profiles() {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return router.ModelProfile{InputPerM: 1.5, CachedInputPerM: 0.15, OutputPerM: 6}
}
func (s *Service) trace(task, modelName string, budget int, p model.ContextPlan, hit bool) error {
	b, _ := json.Marshal(p)
	wi, _ := s.CurrentWorkItem()
	var wid any
	if wi != nil {
		wid = wi.ID
	}
	_, e := s.DB.Exec(`INSERT INTO context_traces(repo_id,work_item_id,task,model,budget,selected_tokens,estimated_cost,cache_hit,decision_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, s.RepoID, wid, task, modelName, budget, p.SelectedTokens, p.EstimatedCost, boolInt(hit), string(b), now())
	return e
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Service) Invalidate(id string) error {
	_, e := s.DB.Exec(`UPDATE memories SET invalidated_at_revision=?,updated_at=? WHERE id=? AND repo_id=?`, s.Repo.Revision, now(), id, s.RepoID)
	return e
}
func (s *Service) InvalidateByGitChange() error {
	// Conservative rule: memories backed only by inference/observation remain usable but stale;
	// source-backed and commit-backed claims are revalidated against the current revision by the next plan.
	_, e := s.DB.Exec(`UPDATE memories SET updated_at=? WHERE repo_id=? AND invalidated_at_revision IS NULL AND authority IN ('source','commit')`, now(), s.RepoID)
	return e
}

func (s *Service) LatestTrace() (map[string]any, error) {
	rows, e := s.DB.Query(`SELECT task,COALESCE(model,''),budget,selected_tokens,estimated_cost,cache_hit,decision_json,created_at FROM context_traces WHERE repo_id=? ORDER BY id DESC LIMIT 1`, s.RepoID)
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return map[string]any{}, nil
	}
	r := rows[0]
	return map[string]any{"task": r[0], "model": r[1], "budget": r[2], "selected_tokens": r[3], "estimated_cost": r[4], "cache_hit": r[5], "decision_json": r[6], "created_at": r[7]}, nil
}
func (s *Service) Resume() (map[string]any, error) {
	wi, e := s.CurrentWorkItem()
	if e != nil {
		return nil, e
	}
	ss, e := s.LatestSession()
	if e != nil {
		return nil, e
	}
	tr, e := s.LatestTrace()
	if e != nil {
		return nil, e
	}
	return map[string]any{"repository": s.Repo, "work_item": wi, "latest_session": ss, "latest_trace": tr}, nil
}
func (s *Service) Handoff(task, target string, budget int) (map[string]any, error) {
	p, e := s.Plan(task, target, budget)
	if e != nil {
		return nil, e
	}
	return map[string]any{"target_model": target, "repository": s.Repo, "task": task, "selected_tokens": p.SelectedTokens, "stable_prefix": p.StablePrefix, "variable_context": p.VariableContext, "context": p.Selected}, nil
}

func (s *Service) Stats() (map[string]any, error) {
	r := map[string]any{"repo": s.Repo.Path, "revision": s.Repo.Revision, "worktree_hash": s.Repo.WorktreeHash, "branch": s.Repo.Branch}
	qs := map[string]string{"memories": "SELECT COUNT(*) FROM memories WHERE repo_id=?", "nodes": "SELECT COUNT(*) FROM nodes WHERE repo_id=?", "edges": "SELECT COUNT(*) FROM edges WHERE src_id IN (SELECT id FROM nodes WHERE repo_id=?)", "events": "SELECT COUNT(*) FROM events WHERE repo_id=?", "traces": "SELECT COUNT(*) FROM context_traces WHERE repo_id=?", "cache_entries": "SELECT COUNT(*) FROM context_cache WHERE repo_id=?"}
	for k, q := range qs {
		arg := s.RepoID
		rows, e := s.DB.Query(q, arg)
		if e != nil {
			return nil, e
		}
		if len(rows) > 0 {
			r[k], _ = strconv.Atoi(rows[0][0])
		}
	}
	// Aggregate the current savings signals.
	rows, e := s.DB.Query(`SELECT COALESCE(SUM(selected_tokens),0),COALESCE(SUM(CASE WHEN cache_hit=1 THEN 1 ELSE 0 END),0),COUNT(*) FROM context_traces WHERE repo_id=?`, s.RepoID)
	if e == nil && len(rows) > 0 {
		sel, _ := strconv.Atoi(rows[0][0])
		hits, _ := strconv.Atoi(rows[0][1])
		cnt, _ := strconv.Atoi(rows[0][2])
		r["planned_tokens_total"] = sel
		r["cache_hit_traces"] = hits
		r["trace_count"] = cnt
	}
	return r, nil
}

func (s *Service) RenderPlan(p model.ContextPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ContextOS Context Plan\nTask: %s\nModel: %s\nBudget: %d tokens\nSelected: %d tokens\nEstimated cost: $%.6f\nCache hit: %v\n\n", p.Task, p.Model, p.Budget, p.SelectedTokens, p.EstimatedCost, p.CacheHit)
	if len(p.StablePrefix) > 0 {
		b.WriteString("STABLE PREFIX\n")
		for _, c := range p.StablePrefix {
			fmt.Fprintf(&b, "- [%s] %s (%d tokens)\n  %s\n", c.Kind, c.ID, c.Tokens, c.Content)
		}
	}
	if len(p.VariableContext) > 0 {
		b.WriteString("\nVARIABLE CONTEXT\n")
		for _, c := range p.VariableContext {
			fmt.Fprintf(&b, "- [%s] %s (%d tokens)\n  %s\n", c.Kind, c.ID, c.Tokens, c.Content)
		}
	}
	return b.String()
}

func (s *Service) Route(task string, budget int) map[string]any {
	p := router.Recommend(task, budget)
	return map[string]any{
		"recommended_model":        p.Name,
		"quality":                  p.Quality,
		"context_tokens":           p.ContextTokens,
		"input_per_million":        p.InputPerM,
		"cached_input_per_million": p.CachedInputPerM,
		"reason":                   "heuristic cost/complexity policy; replace with learned policy after trace collection",
	}
}

func DefaultDBPath() string {
	if _, err := os.Stat(".contextos"); err == nil {
		return filepath.Join(".contextos", "context.db")
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		p := filepath.Join(home, ".contextos")
		if err := os.MkdirAll(p, 0700); err == nil {
			return filepath.Join(p, "context.db")
		}
	}
	_ = os.MkdirAll(".contextos", 0700)
	return filepath.Join(".contextos", "context.db")
}
