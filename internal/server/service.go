package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"contextos/internal/allocator"
	"contextos/internal/db"
	"contextos/internal/gitidx"
	"contextos/internal/model"
	"contextos/internal/router"
	"contextos/internal/store"
	"contextos/internal/textutil"
)

// Options configures storage engine and runtime feature flags for Service.
type Options struct {
	StorageType string // "sqlite" (default) or "file" (zero-dependency pure Go)
	AutoPrune   bool   // opt-in automatic storage pruning during planning/indexing
}

// Service is the primary ContextOS runtime orchestrator.
type Service struct {
	Store     store.Store
	DB        *db.DB // Retained for backward compatibility when SQLiteStore is active
	Repo      gitidx.Repo
	RepoID    string
	AutoPrune bool
}

// New creates a new Service using default options.
func New(dbPath, repoPath string) (*Service, error) {
	return NewWithOptions(dbPath, repoPath, Options{})
}

// NewWithOptions initializes Service with specific storage and feature flag options.
func NewWithOptions(dbPath, repoPath string, opts Options) (*Service, error) {
	r, err := gitidx.Detect(repoPath)
	if err != nil {
		return nil, err
	}

	storageType := strings.ToLower(opts.StorageType)
	if storageType == "" {
		storageType = strings.ToLower(os.Getenv("CONTEXTOS_STORAGE"))
	}
	if storageType == "" && (strings.HasPrefix(dbPath, "file:") || filepath.Ext(dbPath) == ".json" || dbPath == "file") {
		storageType = "file"
	}

	var st store.Store
	var d *db.DB

	if storageType == "file" {
		dir := dbPath
		if dir == "file" || dir == "" || filepath.Ext(dir) != "" {
			dir = filepath.Join(filepath.Dir(dbPath), "data")
		}
		var e error
		st, e = store.NewFileStore(dir)
		if e != nil {
			return nil, e
		}
	} else {
		sqlStore, e := store.NewSQLiteStore(dbPath)
		if e != nil {
			// If SQLite cannot open (e.g. no CGO or missing libsqlite3) and user did not explicitly force sqlite, fall back to pure-Go FileStore
			if storageType == "" {
				dir := filepath.Join(filepath.Dir(dbPath), "data")
				var fe error
				st, fe = store.NewFileStore(dir)
				if fe != nil {
					return nil, fmt.Errorf("sqlite init failed (%w) and file store fallback failed (%v)", e, fe)
				}
			} else {
				return nil, e
			}
		} else {
			st = sqlStore
			d = sqlStore.DB
		}
	}

	repoID, err := st.GetOrCreateRepo(r.Path, r.Name, r.Revision, r.Branch, r.WorktreeHash)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	_ = st.AddRevision(repoID, r.Revision, r.Branch)

	autoPrune := opts.AutoPrune || os.Getenv("CONTEXTOS_AUTO_PRUNE") == "1" || strings.EqualFold(os.Getenv("CONTEXTOS_AUTO_PRUNE"), "true")

	return &Service{
		Store:     st,
		DB:        d,
		Repo:      r,
		RepoID:    repoID,
		AutoPrune: autoPrune,
	}, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Service) Close() {
	if s != nil && s.Store != nil {
		_ = s.Store.Close()
	}
}

func (s *Service) RefreshRepo() error {
	old := s.Repo
	r, e := gitidx.Detect(s.Repo.Path)
	if e != nil {
		return e
	}
	s.Repo = r
	e = s.Store.UpdateRepo(s.RepoID, r.Revision, r.Branch, r.WorktreeHash)
	if e == nil {
		_ = s.Store.AddRevision(s.RepoID, r.Revision, r.Branch)
	}
	if e == nil && (old.Revision != r.Revision || old.Branch != r.Branch) {
		_ = s.InvalidateByGitChange()
	}
	if s.AutoPrune {
		_, _ = s.Store.Prune(store.DefaultPruneOptions(s.RepoID, s.Repo.Revision))
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
	files, _ := gitidx.ListSourceFiles(s.Repo.Path)

	// Save nodes and collect file references
	if err := s.Store.SaveNodesAndEdges(s.RepoID, files, syms, nil); err != nil {
		return err
	}

	// Lightweight deterministic import/dependency edges
	_ = s.buildEdges()
	return nil
}

func (s *Service) buildEdges() error {
	nodes, err := s.Store.ListNodes(s.RepoID)
	if err != nil {
		return err
	}
	fileByBase := map[string]string{}
	for _, r := range nodes {
		if r.Kind == "file" {
			fileByBase[r.Name] = r.ID
			fileByBase[r.Path] = r.ID
		}
	}
	var edges []store.EdgeRecord
	for _, r := range nodes {
		if r.Kind != "file" {
			continue
		}
		p := filepath.Join(s.Repo.Path, filepath.FromSlash(r.Path))
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		txt := string(b)
		for base, dst := range fileByBase {
			if base == r.Name || base == r.Path {
				continue
			}
			if strings.Contains(txt, base) {
				edges = append(edges, store.EdgeRecord{SrcID: r.ID, DstID: dst, Kind: "references"})
			}
		}
	}
	if len(edges) > 0 {
		_ = s.Store.SaveNodesAndEdges(s.RepoID, nil, nil, edges)
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

	mem := model.Memory{
		ID:                id,
		Kind:              kind,
		Content:           content,
		Scope:             scope,
		ValidFromRevision: s.Repo.Revision,
		Authority:         authority,
		Confidence:        confidence,
		TokenCost:         t,
	}
	return s.Store.Remember(s.RepoID, mem, provenance)
}

func hashID(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])[:20]
}

func (s *Service) CurrentWorkItem() (*model.WorkItem, error) {
	return s.Store.CurrentWorkItem(s.RepoID)
}

func (s *Service) StartWorkItem(title string) (model.WorkItem, error) {
	if strings.TrimSpace(title) == "" {
		return model.WorkItem{}, fmt.Errorf("work item title is required")
	}
	wi, err := s.Store.SetWorkItem(s.RepoID, title, s.Repo.Branch)
	if err != nil {
		return model.WorkItem{}, err
	}
	return *wi, nil
}

func (s *Service) StartSession(agent, workID string) (model.Session, error) {
	if agent == "" {
		agent = "unknown"
	}
	tm := now()
	id := hashID(s.RepoID + "|session|" + agent + "|" + tm)
	if err := s.Store.StartSession(s.RepoID, id, workID, agent); err != nil {
		return model.Session{}, err
	}
	return model.Session{ID: id, Agent: agent, WorkItem: workID, StartedAt: tm}, nil
}

func (s *Service) EndSession(id string) error {
	return s.Store.EndSession(s.RepoID, id)
}

func (s *Service) LatestSession() (*model.Session, error) {
	return s.Store.LatestSession(s.RepoID)
}

func (s *Service) RecordEvent(sessionID, eventType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.Store.AddEvent(s.RepoID, sessionID, eventType, string(b))
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
		if events, err := s.Store.ListEvents(ev.SessionID, 1); err == nil && len(events) > 0 {
			ss, _ = s.LatestSession()
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

func flattenPayload(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func (s *Service) ConsolidateHeuristics(sessionID string) error {
	events, err := s.Store.ListEvents(sessionID, 30)
	if err != nil {
		return err
	}
	workID := ""
	if sess, err := s.LatestSession(); err == nil && sess != nil {
		workID = sess.WorkItem
	}
	for _, r := range events {
		p := r.Payload
		lp := strings.ToLower(p)
		if strings.Contains(lp, "we decided") || strings.Contains(lp, "decision:") || strings.Contains(lp, "use outbox") || strings.Contains(lp, "use kafka") {
			txt := p
			if len(txt) > 700 {
				txt = txt[:700] + "…"
			}
			_, _ = s.Remember("decision", txt, "inference", "repo", workID, 0.72, []string{"session_event|" + r.EventType})
		}
	}
	return nil
}

func (s *Service) SearchCandidates(task string, limit int) ([]model.Memory, error) {
	mems, err := s.Store.SearchMemories(s.RepoID, task, limit)
	if err != nil {
		return nil, err
	}
	out := make([]model.Memory, 0, len(mems))
	for _, m := range mems {
		if strings.HasPrefix(m.Content, "User task: ") {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Service) CodeMemories(task string, limit int) ([]model.Memory, error) {
	if limit <= 0 {
		limit = 150
	}
	nodes, err := s.Store.ListNodes(s.RepoID)
	if err != nil {
		return nil, err
	}
	type scored struct {
		m     model.Memory
		score float64
	}
	tmp := make([]scored, 0, len(nodes))
	for _, r := range nodes {
		content := fmt.Sprintf("%s %s %s %s:%d", r.Kind, r.Name, r.Signature, r.Path, r.StartLine)
		sc := 0.6*textutil.HashSemantic(task, content) + 0.4*textutil.Overlap(task, content)
		if sc < 0.05 && task != "" {
			continue
		}
		tmp = append(tmp, scored{
			m: model.Memory{
				ID:                "node:" + r.ID,
				Kind:              "code",
				Content:           content,
				Scope:             "repo",
				ValidFromRevision: s.Repo.Revision,
				Authority:         "source",
				Confidence:        1.0,
				TokenCost:         textutil.EstimateTokens(content),
				Source:            "repository",
				Location:          r.Path,
			},
			score: sc,
		})
	}
	sort.Slice(tmp, func(i, j int) bool { return tmp[i].score > tmp[j].score })
	if len(tmp) > limit {
		tmp = tmp[:limit]
	}
	out := make([]model.Memory, len(tmp))
	for i, x := range tmp {
		out[i] = x.m
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

	// Check context plan cache
	cachedJSON, err := s.Store.GetCache(key, s.RepoID, s.Repo.Revision, s.Repo.WorktreeHash)
	if err == nil && cachedJSON != "" {
		var p model.ContextPlan
		if json.Unmarshal([]byte(cachedJSON), &p) == nil {
			_ = s.Store.IncrementCacheHit(key)
			p.CacheHit = true
			p.CreatedAt = time.Now().UTC()
			_ = s.trace(task, modelName, budget, p, true)
			return p, nil
		}
	}

	ms, err := s.SearchCandidates(task, 200)
	if err != nil {
		return model.ContextPlan{}, err
	}
	codes, err := s.CodeMemories(task, 100)
	if err != nil {
		return model.ContextPlan{}, err
	}
	ms = append(ms, codes...)

	p := allocator.Plan(allocator.Request{
		Task:         task,
		Budget:       budget,
		Model:        modelName,
		RepoRevision: s.Repo.Revision,
	}, ms)
	p.Model = modelName
	p.CreatedAt = time.Now().UTC()

	prof := profile(modelName)
	p.EstimatedCost = float64(p.SelectedTokens) * prof.InputPerM / 1e6

	b, _ := json.Marshal(p)
	_ = s.Store.PutCache(key, s.RepoID, s.Repo.Revision, s.Repo.WorktreeHash, task, modelName, budget, string(b))
	_ = s.trace(task, modelName, budget, p, false)

	for _, c := range p.Selected {
		if strings.HasPrefix(c.ID, "node:") {
			continue
		}
		_ = s.Store.IncrementMemoryReuse(s.RepoID, c.ID)
	}

	// Opt-in automatic pruning if enabled
	if s.AutoPrune {
		_, _ = s.Store.Prune(store.DefaultPruneOptions(s.RepoID, s.Repo.Revision))
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
	wid := ""
	if wi != nil {
		wid = wi.ID
	}
	return s.Store.AddTrace(store.ContextTraceRecord{
		RepoID:         s.RepoID,
		WorkItemID:     wid,
		Task:           task,
		Model:          modelName,
		Budget:         budget,
		SelectedTokens: p.SelectedTokens,
		EstimatedCost:  p.EstimatedCost,
		CacheHit:       hit,
		DecisionJSON:   string(b),
	})
}

func (s *Service) Invalidate(id string) error {
	return s.Store.Invalidate(s.RepoID, id, s.Repo.Revision)
}

func (s *Service) InvalidateByGitChange() error {
	return s.Store.InvalidateByGitChange(s.RepoID)
}

func (s *Service) LatestTrace() (map[string]any, error) {
	return s.Store.LatestTrace(s.RepoID)
}

func (s *Service) Resume() (map[string]any, error) {
	wi, err := s.CurrentWorkItem()
	if err != nil {
		return nil, err
	}
	ss, err := s.LatestSession()
	if err != nil {
		return nil, err
	}
	tr, err := s.LatestTrace()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"repository":     s.Repo,
		"work_item":      wi,
		"latest_session": ss,
		"latest_trace":   tr,
	}, nil
}

func (s *Service) Handoff(task, target string, budget int) (map[string]any, error) {
	p, err := s.Plan(task, target, budget)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"target_model":     target,
		"repository":       s.Repo,
		"task":             task,
		"selected_tokens":  p.SelectedTokens,
		"stable_prefix":    p.StablePrefix,
		"variable_context": p.VariableContext,
		"context":          p.Selected,
	}, nil
}

func (s *Service) Stats() (map[string]any, error) {
	r := map[string]any{
		"repo":          s.Repo.Path,
		"revision":      s.Repo.Revision,
		"worktree_hash": s.Repo.WorktreeHash,
		"branch":        s.Repo.Branch,
		"auto_prune":    s.AutoPrune,
	}
	mems, err := s.Store.ListMemories(s.RepoID, 10000)
	if err == nil {
		r["memories"] = len(mems)
	}
	nodes, err := s.Store.ListNodes(s.RepoID)
	if err == nil {
		r["nodes"] = len(nodes)
	}
	tokens, hits, cnt, err := s.Store.TraceStats(s.RepoID)
	if err == nil {
		r["planned_tokens_total"] = tokens
		r["cache_hit_traces"] = hits
		r["trace_count"] = cnt
	}
	return r, nil
}

// GC executes a garbage collection pass across the active store.
func (s *Service) GC(opts store.PruneOptions) (store.PruneReport, error) {
	if opts.RepoID == "" {
		opts.RepoID = s.RepoID
	}
	if opts.CurrentRevision == "" {
		opts.CurrentRevision = s.Repo.Revision
	}
	return s.Store.Prune(opts)
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
