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
	"contextos/internal/config"
	"contextos/internal/db"
	"contextos/internal/extractor"
	"contextos/internal/gitidx"
	"contextos/internal/graph"
	"contextos/internal/indexer"
	"contextos/internal/model"
	"contextos/internal/planning"
	"contextos/internal/router"
	"contextos/internal/state"
	"contextos/internal/store"
	"contextos/internal/textutil"
)

// Options configures storage engine, runtime feature flags, timeouts, and adaptive context settings for Service.
type Options struct {
	StorageType     string        // "sqlite" (default) or "file" (zero-dependency pure Go)
	AutoPrune       bool          // opt-in automatic storage pruning during planning/indexing
	Timeout         time.Duration // Query timeout (e.g. 500ms). If exceeded or close, throttles context
	DefaultBudget   int           // Default context budget in tokens (e.g. 4000)
	MinBudget       int           // Minimum context budget floor when adaptive timeout mitigates deadline (e.g. 500)
	AdaptiveTimeout bool          // Automatically lower context budget / shed stages if time is running out
	RetrievalMode   string        // "baseline", "indexed", "adaptive"
}

// Service is the primary ContextOS runtime orchestrator.
type Service struct {
	Store           store.Store
	DB              *db.DB // Retained for backward compatibility when SQLiteStore is active
	Repo            gitidx.Repo
	RepoID          string
	AutoPrune       bool
	Timeout         time.Duration
	DefaultBudget   int
	MinBudget       int
	AdaptiveTimeout bool
	RetrievalMode   string
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

	// Load repository-level config (.contextos/config.toml) if present
	repoCfg, _ := config.Load(r.Path)

	timeout := opts.Timeout
	if timeout == 0 {
		if tStr := os.Getenv("CONTEXTOS_TIMEOUT"); tStr != "" {
			if d, err := time.ParseDuration(tStr); err == nil {
				timeout = d
			}
		} else if msStr := os.Getenv("CONTEXTOS_TIMEOUT_MS"); msStr != "" {
			if ms, err := strconv.Atoi(msStr); err == nil && ms > 0 {
				timeout = time.Duration(ms) * time.Millisecond
			}
		} else if repoCfg.Retrieval.TimeoutMs > 0 {
			timeout = time.Duration(repoCfg.Retrieval.TimeoutMs) * time.Millisecond
		}
	}

	defaultBudget := opts.DefaultBudget
	if defaultBudget <= 0 {
		if bStr := os.Getenv("CONTEXTOS_BUDGET"); bStr != "" {
			if b, err := strconv.Atoi(bStr); err == nil && b > 0 {
				defaultBudget = b
			}
		} else if repoCfg.Allocator.DefaultBudget > 0 {
			defaultBudget = repoCfg.Allocator.DefaultBudget
		}
	}
	if defaultBudget <= 0 {
		defaultBudget = 4000
	}

	minBudget := opts.MinBudget
	if minBudget <= 0 {
		if mbStr := os.Getenv("CONTEXTOS_MIN_BUDGET"); mbStr != "" {
			if mb, err := strconv.Atoi(mbStr); err == nil && mb > 0 {
				minBudget = mb
			}
		} else if repoCfg.Allocator.MinBudgetTokens > 0 {
			minBudget = repoCfg.Allocator.MinBudgetTokens
		}
	}
	if minBudget <= 0 {
		minBudget = 500
	}

	adaptiveTimeout := opts.AdaptiveTimeout
	if !adaptiveTimeout {
		atEnv := os.Getenv("CONTEXTOS_ADAPTIVE_TIMEOUT")
		if atEnv != "" {
			adaptiveTimeout = atEnv == "1" || strings.EqualFold(atEnv, "true")
		} else if repoCfg.Retrieval.AdaptiveTimeout {
			adaptiveTimeout = true
		} else if timeout > 0 {
			adaptiveTimeout = true // default true when timeout is set
		}
	}

	retrievalMode := strings.ToLower(opts.RetrievalMode)
	if retrievalMode == "" {
		retrievalMode = strings.ToLower(os.Getenv("CONTEXTOS_RETRIEVAL_MODE"))
	}
	if retrievalMode == "" && repoCfg.Retrieval.Mode != "" {
		retrievalMode = strings.ToLower(repoCfg.Retrieval.Mode)
	}
	if retrievalMode == "" {
		retrievalMode = "adaptive"
	}

	return &Service{
		Store:           st,
		DB:              d,
		Repo:            r,
		RepoID:          repoID,
		AutoPrune:       autoPrune,
		Timeout:         timeout,
		DefaultBudget:   defaultBudget,
		MinBudget:       minBudget,
		AdaptiveTimeout: adaptiveTimeout,
		RetrievalMode:   retrievalMode,
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
	idx := indexer.New(s.Store, s.Repo.Path, s.RepoID)
	_, err := idx.Index(false, s.Repo.Revision)
	return err
}

func (s *Service) IndexFull() error {
	if e := s.RefreshRepo(); e != nil {
		return e
	}
	idx := indexer.New(s.Store, s.Repo.Path, s.RepoID)
	_, err := idx.Index(true, s.Repo.Revision)
	return err
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

func (s *Service) computeGraphScores(task string) map[string]float64 {
	nodes, err := s.Store.ListNodes(s.RepoID)
	if err != nil || len(nodes) == 0 {
		return nil
	}
	g := graph.New(graph.DefaultConfig())
	for _, n := range nodes {
		g.AddNode(&graph.Node{
			ID:    n.ID,
			Name:  n.Name,
			Kind:  n.Kind,
			Path:  n.Path,
			Lines: n.EndLine - n.StartLine + 1,
		})
	}
	fileByBase := map[string]string{}
	for _, n := range nodes {
		if n.Kind == "file" {
			fileByBase[n.Name] = n.ID
			fileByBase[n.Path] = n.ID
		}
	}
	for _, n := range nodes {
		if n.Kind != "file" {
			continue
		}
		p := filepath.Join(s.Repo.Path, filepath.FromSlash(n.Path))
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		txt := string(b)
		for base, dst := range fileByBase {
			if base == n.Name || base == n.Path {
				continue
			}
			if strings.Contains(txt, base) {
				edgeKind := "import"
				if strings.HasSuffix(n.Path, "_test.go") || strings.Contains(n.Path, "test") {
					edgeKind = "test-reference"
				}
				g.AddEdge(n.ID, dst, edgeKind)
			}
		}
	}

	seeds := g.ExtractSeeds(task, nil, nil)
	ppr := g.ComputePPR(seeds)
	scores := make(map[string]float64, len(nodes))
	for _, n := range nodes {
		scores[n.ID] = g.CompositeScore(n.ID, ppr, nil)
		if n.Path != "" {
			scores[n.Path] = scores[n.ID]
		}
	}
	return scores
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
	events, err := s.Store.ListEvents(sessionID, 50)
	if err != nil {
		return err
	}
	pipeline := extractor.NewPipeline(s.Repo.Revision)
	existing, _ := s.Store.ListMemories(s.RepoID, 500)

	for _, r := range events {
		claims := pipeline.ExtractFromText(r.Payload, r.EventType, sessionID, s.Repo.Path)
		for _, claim := range claims {
			res := extractor.ResolveConflicts(existing, claim, s.Repo.Revision)
			for _, upd := range res.UpdatedExisting {
				_ = s.Store.ImportMemory(s.RepoID, upd, nil)
			}
			if !res.IsDuplicate {
				_ = s.Store.ImportMemory(s.RepoID, res.NewMemory, nil)
				existing = append(existing, res.NewMemory)
			}
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
	// PR.md PR-04: Replace O(N) full repository scan with indexed candidate generation
	nodes, err := s.Store.SearchCodeCandidates(s.RepoID, task, "", limit*2)
	if err != nil || len(nodes) == 0 {
		nodes, err = s.Store.ListNodes(s.RepoID)
		if err != nil {
			return nil, err
		}
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
	startTime := time.Now()
	if budget <= 0 {
		budget = s.DefaultBudget
	}
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
			prof := profile(modelName)
			p.EstimatedCost = float64(p.SelectedTokens) * prof.CachedInputPerM / 1e6
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

	// Adaptive Timeout mitigation:
	// If timeout is configured and elapsed time > 50% of timeout, dynamically lower
	// context budget to MinBudget (e.g. 500 tokens) and skip heavy graph traversals
	// to prevent timeouts while returning decision-sufficient context.
	effectiveBudget := budget
	adaptiveThrottled := false
	if s.Timeout > 0 && s.AdaptiveTimeout {
		elapsed := time.Since(startTime)
		if elapsed > s.Timeout/2 {
			effectiveBudget = s.MinBudget
			if effectiveBudget <= 0 {
				effectiveBudget = 500
			}
			adaptiveThrottled = true
		}
	}

	var graphScores map[string]float64
	if !adaptiveThrottled {
		graphScores = s.computeGraphScores(task)
	}

	p := allocator.Plan(allocator.Request{
		Task:         task,
		Budget:       effectiveBudget,
		Model:        modelName,
		RepoRevision: s.Repo.Revision,
		GraphScores:  graphScores,
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
	return router.GetProfile(name)
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

func (s *Service) Validate(id string) error {
	return s.Store.Validate(s.RepoID, id)
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
		"repo":             s.Repo.Path,
		"revision":         s.Repo.Revision,
		"worktree_hash":    s.Repo.WorktreeHash,
		"branch":           s.Repo.Branch,
		"auto_prune":       s.AutoPrune,
		"engine_version":   "ASC-1.4",
		"timeout_ms":       s.Timeout.Milliseconds(),
		"adaptive_timeout": s.AdaptiveTimeout,
		"default_budget":   s.DefaultBudget,
		"min_budget":       s.MinBudget,
		"retrieval_mode":   s.RetrievalMode,
	}

	// Storage engine detection
	switch s.Store.(type) {
	case *store.FileStore:
		r["storage_engine"] = "file"
	default:
		r["storage_engine"] = "sqlite"
	}

	mems, err := s.Store.ListMemories(s.RepoID, 10000)
	if err == nil {
		r["memories"] = len(mems)
	}
	nodes, err := s.Store.ListNodes(s.RepoID)
	if err == nil {
		r["nodes"] = len(nodes)
		// Count file vs symbol nodes
		fileNodes := 0
		symbolNodes := 0
		for _, n := range nodes {
			if n.Kind == "file" {
				fileNodes++
			} else {
				symbolNodes++
			}
		}
		r["file_nodes"] = fileNodes
		r["symbol_nodes"] = symbolNodes
	}
	edges, err := s.Store.ListEdges(s.RepoID)
	if err == nil {
		r["edges"] = len(edges)
	}
	tokens, hits, cnt, err := s.Store.TraceStats(s.RepoID)
	if err == nil {
		r["planned_tokens_total"] = tokens
		r["cache_hit_traces"] = hits
		r["trace_count"] = cnt
	}

	// Active engine features (reflects completed PRs and performance subsystems)
	r["features"] = []string{
		"portable-integrations",     // PR-01
		"content-addressed-worktree", // PR-02
		"graph-intelligence-ppr",    // PR-03
		"incremental-indexing",      // PR-04
		"positional-trigrams",       // PR-11..12
		"compressed-postings",       // PR-13..15
		"block-max-wand",            // PR-16..17
		"scope-localization",        // PR-18
		"candidate-fusion",          // PR-22..24
		"adaptive-timeout",          // Adaptive context throttling
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

// ComputePlan generates an adaptive compute strategy for a task.
func (s *Service) ComputePlan(task string, riskTarget float64, preferredProvider string) planning.ComputePlan {
	planner := planning.NewComputePlanner()
	return planner.Generate(task, riskTarget, preferredProvider)
}

// ExecutionPlan synthesizes both context allocation and adaptive compute planning.
func (s *Service) ExecutionPlan(task, modelName string, budget int, riskTarget float64, preferredProvider string) (planning.ExecutionPlan, error) {
	ctxPlan, err := s.Plan(task, modelName, budget)
	if err != nil {
		return planning.ExecutionPlan{}, err
	}
	cPlan := s.ComputePlan(task, riskTarget, preferredProvider)
	ds := state.NewDecisionState(task)
	return planning.BuildExecutionPlan(task, ctxPlan, cPlan, ds), nil
}

func DefaultDBPath() string {
	// Prefer global ~/.contextos/ first to avoid reading repo-local DBs
	// when the CWD happens to contain a .contextos/ directory (e.g. the
	// ContextOS source tree itself).
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		globalDB := filepath.Join(home, ".contextos", "context.db")
		if _, err := os.Stat(globalDB); err == nil {
			return globalDB
		}
		// Global dir doesn't exist yet — create it and use it
		p := filepath.Join(home, ".contextos")
		if err := os.MkdirAll(p, 0700); err == nil {
			return filepath.Join(p, "context.db")
		}
	}
	// Fallback to repo-local .contextos/ only if home is unavailable
	if _, err := os.Stat(".contextos"); err == nil {
		return filepath.Join(".contextos", "context.db")
	}
	_ = os.MkdirAll(".contextos", 0700)
	return filepath.Join(".contextos", "context.db")
}
