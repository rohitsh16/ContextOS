package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/model"
	"contextos/internal/textutil"
)

type repoRecord struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Name         string `json:"name"`
	Revision     string `json:"revision"`
	Branch       string `json:"branch"`
	WorktreeHash string `json:"worktree_hash"`
	IndexedAt    string `json:"indexed_at"`
}

type memoryRecord struct {
	model.Memory
	RepoID     string   `json:"repo_id"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	Provenance []string `json:"provenance,omitempty"`
}

type cacheRecord struct {
	CacheKey     string `json:"cache_key"`
	RepoID       string `json:"repo_id"`
	Revision     string `json:"revision"`
	WorktreeHash string `json:"worktree_hash"`
	TaskHash     string `json:"task_hash"`
	Model        string `json:"model"`
	Budget       int    `json:"budget"`
	PlanJSON     string `json:"plan_json"`
	HitCount     int    `json:"hit_count"`
	LastHitAt    string `json:"last_hit_at"`
	CreatedAt    string `json:"created_at"`
}

// FileStore is a pure-Go, zero-dependency storage engine using structured JSON and JSONL files.
// It requires NO CGO, NO SQLite, and NO external database engine.
type FileStore struct {
	mu      sync.RWMutex
	baseDir string

	repos     map[string]repoRecord     // key: path
	revisions map[string]bool           // key: repoID + "|" + rev
	nodes     map[string][]NodeRecord   // key: repoID
	edges     map[string][]EdgeRecord   // key: repoID
	memories  map[string][]memoryRecord // key: repoID
	workItems map[string][]model.WorkItem
	sessions  map[string][]model.Session
	caches    map[string]cacheRecord // key: cacheKey
}

// NewFileStore initializes or loads a FileStore in the given directory.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".contextos", "data")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir file store %s: %w", dir, err)
	}
	fs := &FileStore{
		baseDir:   dir,
		repos:     make(map[string]repoRecord),
		revisions: make(map[string]bool),
		nodes:     make(map[string][]NodeRecord),
		edges:     make(map[string][]EdgeRecord),
		memories:  make(map[string][]memoryRecord),
		workItems: make(map[string][]model.WorkItem),
		sessions:  make(map[string][]model.Session),
		caches:    make(map[string]cacheRecord),
	}
	_ = fs.load()
	return fs, nil
}

func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.save()
}

func (fs *FileStore) load() error {
	readJSON(filepath.Join(fs.baseDir, "repositories.json"), &fs.repos)
	readJSON(filepath.Join(fs.baseDir, "revisions.json"), &fs.revisions)
	readJSON(filepath.Join(fs.baseDir, "nodes.json"), &fs.nodes)
	readJSON(filepath.Join(fs.baseDir, "edges.json"), &fs.edges)
	readJSON(filepath.Join(fs.baseDir, "memories.json"), &fs.memories)
	readJSON(filepath.Join(fs.baseDir, "work_items.json"), &fs.workItems)
	readJSON(filepath.Join(fs.baseDir, "sessions.json"), &fs.sessions)
	readJSON(filepath.Join(fs.baseDir, "cache.json"), &fs.caches)
	return nil
}

func (fs *FileStore) save() error {
	writeJSON(filepath.Join(fs.baseDir, "repositories.json"), fs.repos)
	writeJSON(filepath.Join(fs.baseDir, "revisions.json"), fs.revisions)
	writeJSON(filepath.Join(fs.baseDir, "nodes.json"), fs.nodes)
	writeJSON(filepath.Join(fs.baseDir, "edges.json"), fs.edges)
	writeJSON(filepath.Join(fs.baseDir, "memories.json"), fs.memories)
	writeJSON(filepath.Join(fs.baseDir, "work_items.json"), fs.workItems)
	writeJSON(filepath.Join(fs.baseDir, "sessions.json"), fs.sessions)
	writeJSON(filepath.Join(fs.baseDir, "cache.json"), fs.caches)
	return nil
}

func readJSON(path string, dst any) {
	b, err := os.ReadFile(path)
	if err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, dst)
	}
}

func writeJSON(path string, src any) {
	b, err := json.MarshalIndent(src, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, b, 0644)
	}
}

func (fs *FileStore) GetOrCreateRepo(path, name, revision, branch, worktreeHash string) (string, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if r, ok := fs.repos[path]; ok {
		return r.ID, nil
	}
	id := fmt.Sprintf("%d", len(fs.repos)+1)
	fs.repos[path] = repoRecord{
		ID:           id,
		Path:         path,
		Name:         name,
		Revision:     revision,
		Branch:       branch,
		WorktreeHash: worktreeHash,
		IndexedAt:    now(),
	}
	_ = fs.save()
	return id, nil
}

func (fs *FileStore) UpdateRepo(repoID, revision, branch, worktreeHash string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for p, r := range fs.repos {
		if r.ID == repoID {
			r.Revision = revision
			r.Branch = branch
			r.WorktreeHash = worktreeHash
			r.IndexedAt = now()
			fs.repos[p] = r
			_ = fs.save()
			return nil
		}
	}
	return nil
}

func (fs *FileStore) AddRevision(repoID, revision, branch string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	key := repoID + "|" + revision
	fs.revisions[key] = true
	return nil
}

func (fs *FileStore) SaveNodesAndEdges(repoID string, files []gitidx.SourceFile, syms []gitidx.Symbol, edges []EdgeRecord) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	inDeg := make(map[string]int)
	outDeg := make(map[string]int)
	fanIn := make(map[string]map[string]bool)
	fanOut := make(map[string]map[string]bool)
	testCounts := make(map[string]int)

	for _, e := range edges {
		outDeg[e.SrcID]++
		inDeg[e.DstID]++
		if fanIn[e.DstID] == nil {
			fanIn[e.DstID] = make(map[string]bool)
		}
		fanIn[e.DstID][e.SrcID] = true
		if fanOut[e.SrcID] == nil {
			fanOut[e.SrcID] = make(map[string]bool)
		}
		fanOut[e.SrcID][e.DstID] = true
		if strings.Contains(e.Kind, "test") || strings.Contains(e.SrcID, "test") {
			testCounts[e.DstID]++
		}
	}

	maxDeg := 1
	for _, d := range inDeg {
		if d > maxDeg {
			maxDeg = d
		}
	}

	var nodes []NodeRecord
	for _, f := range files {
		nodeID := hashID("file|" + f.Path)
		ind := inDeg[nodeID]
		oud := outDeg[nodeID]
		nodes = append(nodes, NodeRecord{
			ID:            nodeID,
			RepoID:        repoID,
			Kind:          "file",
			Path:          f.Path,
			Name:          filepath.Base(f.Path),
			StartLine:     1,
			EndLine:       f.Lines,
			ContentHash:   f.Hash,
			InDegree:      ind,
			OutDegree:     oud,
			FanIn:         len(fanIn[nodeID]),
			FanOut:        len(fanOut[nodeID]),
			TestCount:     testCounts[nodeID],
			Centrality:    float64(ind+oud) / float64(maxDeg+1),
		})
	}
	for _, s := range syms {
		nodeID := hashID(s.Kind + "|" + s.Path + "|" + s.Name)
		ind := inDeg[nodeID]
		oud := outDeg[nodeID]
		nodes = append(nodes, NodeRecord{
			ID:            nodeID,
			RepoID:        repoID,
			Kind:          s.Kind,
			Path:          s.Path,
			Name:          s.Name,
			StartLine:     s.Start,
			EndLine:       s.End,
			Signature:     s.Signature,
			ContentHash:   s.Hash,
			InDegree:      ind,
			OutDegree:     oud,
			FanIn:         len(fanIn[nodeID]),
			FanOut:        len(fanOut[nodeID]),
			TestCount:     testCounts[nodeID],
			Centrality:    float64(ind+oud) / float64(maxDeg+1),
		})
	}
	fs.nodes[repoID] = nodes
	fs.edges[repoID] = edges
	_ = fs.save()
	return nil
}

func (fs *FileStore) ListNodes(repoID string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return append([]NodeRecord(nil), fs.nodes[repoID]...), nil
}

func (fs *FileStore) LookupNodesByIDs(repoID string, nodeIDs []string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	idSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		idSet[id] = true
	}
	var out []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if idSet[n.ID] {
			out = append(out, n)
		}
	}
	return out, nil
}

func (fs *FileStore) CountNodes(repoID string) (int, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return len(fs.nodes[repoID]), nil
}

func (fs *FileStore) LookupSymbol(repoID string, name string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	var out []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if strings.EqualFold(n.Name, name) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (fs *FileStore) LookupQualifiedSymbol(repoID string, qualifiedName string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	parts := strings.Split(qualifiedName, ".")
	symName := qualifiedName
	pkgOrType := ""
	if len(parts) >= 2 {
		pkgOrType = strings.ToLower(parts[len(parts)-2])
		symName = parts[len(parts)-1]
	}
	var out []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if strings.EqualFold(n.Name, symName) {
			if pkgOrType == "" || strings.Contains(strings.ToLower(n.Path), pkgOrType) || strings.Contains(strings.ToLower(n.Signature), pkgOrType) {
				out = append(out, n)
			}
		}
	}
	return out, nil
}

// LookupExactPath returns nodes whose path exactly matches (R17 Phase 3).
func (fs *FileStore) LookupExactPath(repoID string, path string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	var out []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if n.Path == path {
			out = append(out, n)
		}
	}
	return out, nil
}

// LookupBasename returns file nodes whose name exactly matches the basename (R17 Phase 3).
func (fs *FileStore) LookupBasename(repoID string, basename string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	basenameLow := strings.ToLower(basename)
	var out []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if n.Kind == "file" && strings.ToLower(n.Name) == basenameLow {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		// Fallback: any node whose path ends with the basename
		for _, n := range fs.nodes[repoID] {
			if strings.ToLower(n.Name) == basenameLow {
				out = append(out, n)
			}
		}
	}
	return out, nil
}

func (fs *FileStore) LookupPath(repoID string, path string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	var out []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if n.Path == path || strings.Contains(n.Path, path) {
			out = append(out, n)
		}
	}
	return out, nil
}


func (fs *FileStore) LookupPackage(repoID string, pkg string) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	var out []NodeRecord
	pkgLow := strings.ToLower(pkg)
	for _, n := range fs.nodes[repoID] {
		if strings.Contains(strings.ToLower(n.Path), pkgLow) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (fs *FileStore) SearchCodeCandidates(repoID string, query string, scope string, limit int) ([]NodeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if limit <= 0 {
		limit = 150
	}
	nodes := fs.nodes[repoID]
	if len(nodes) == 0 {
		return nil, nil
	}
	classified := textutil.ClassifyQueryTokens(query)
	prioritizedTokens := textutil.ExtractPrioritizedTokens(query)
	if len(prioritizedTokens) == 0 {
		if len(nodes) > limit {
			return append([]NodeRecord(nil), nodes[:limit]...), nil
		}
		return append([]NodeRecord(nil), nodes...), nil
	}

	type scored struct {
		n  NodeRecord
		sc float64
	}
	var tmp []scored
	scopeLow := strings.ToLower(scope)

	for _, n := range nodes {
		if scope != "" && !strings.Contains(strings.ToLower(n.Path), scopeLow) {
			continue
		}
		content := fmt.Sprintf("%s %s %s %s", n.Kind, n.Name, n.Signature, n.Path)
		sc := 0.6*textutil.HashSemantic(query, content) + 0.4*textutil.Overlap(query, content)

		// R18.1 §8 Priority matching boosts
		baseName := filepath.Base(n.Path)
		for _, ct := range classified {
			switch ct.Class {
			case textutil.ClassPathFilename:
				if strings.EqualFold(baseName, filepath.Base(ct.Text)) || strings.EqualFold(n.Path, ct.Text) {
					sc += 1.5
				}
			case textutil.ClassSymbol:
				if strings.EqualFold(n.Name, ct.Text) {
					sc += 1.2
				}
			case textutil.ClassIdentifier, textutil.ClassEntity:
				if strings.EqualFold(n.Name, ct.Text) {
					sc += 0.8
				}
			}
		}

		if strings.Contains(strings.ToLower(query), strings.ToLower(n.Name)) {
			sc += 0.35
		}
		if sc > 0.05 || len(tmp) < limit {
			tmp = append(tmp, scored{n: n, sc: sc})
		}
	}

	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].sc > tmp[j].sc
	})
	if len(tmp) > limit {
		tmp = tmp[:limit]
	}

	out := make([]NodeRecord, len(tmp))
	for i, s := range tmp {
		out[i] = s.n
	}
	return out, nil
}

func (fs *FileStore) ListEdges(repoID string) ([]EdgeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return append([]EdgeRecord(nil), fs.edges[repoID]...), nil
}

func (fs *FileStore) LookupAdjacentEdges(repoID string, nodeIDs []string) ([]EdgeRecord, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	idSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		idSet[id] = true
	}
	var out []EdgeRecord
	for _, e := range fs.edges[repoID] {
		if idSet[e.SrcID] || idSet[e.DstID] {
			out = append(out, e)
		}
	}
	return out, nil
}


func (fs *FileStore) UpdateNodesAndEdges(repoID string, files []gitidx.SourceFile, syms []gitidx.Symbol, deletedPaths []string, edges []EdgeRecord) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	deletedMap := make(map[string]bool)
	for _, p := range deletedPaths {
		deletedMap[p] = true
	}
	for _, f := range files {
		deletedMap[f.Path] = true
	}

	var surviving []NodeRecord
	for _, n := range fs.nodes[repoID] {
		if !deletedMap[n.Path] {
			surviving = append(surviving, n)
		}
	}

	for _, f := range files {
		surviving = append(surviving, NodeRecord{
			ID:          hashID("file|" + f.Path),
			RepoID:      repoID,
			Kind:        "file",
			Path:        f.Path,
			Name:        filepath.Base(f.Path),
			StartLine:   1,
			EndLine:     f.Lines,
			ContentHash: f.Hash,
		})
	}
	for _, s := range syms {
		surviving = append(surviving, NodeRecord{
			ID:          hashID(s.Kind + "|" + s.Path + "|" + s.Name),
			RepoID:      repoID,
			Kind:        s.Kind,
			Path:        s.Path,
			Name:        s.Name,
			StartLine:   s.Start,
			EndLine:     s.End,
			Signature:   s.Signature,
			ContentHash: s.Hash,
		})
	}
	fs.nodes[repoID] = surviving

	if edges != nil {
		fs.edges[repoID] = edges
	}
	_ = fs.save()
	return nil
}

func (fs *FileStore) Remember(repoID string, mem model.Memory, provenance []string) (model.Memory, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

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

	rec := memoryRecord{
		Memory: model.Memory{
			ID:                    id,
			Kind:                  kind,
			Content:               mem.Content,
			Scope:                 scope,
			ValidFromRevision:     mem.ValidFromRevision,
			InvalidatedAtRevision: mem.InvalidatedAtRevision,
			Authority:             auth,
			Confidence:            mem.Confidence,
			TokenCost:             tok,
			ReuseCount:            mem.ReuseCount,
			LastAccessedAt:        mem.LastAccessedAt,
			Source:                mem.Source,
		},
		RepoID:     repoID,
		CreatedAt:  tm,
		UpdatedAt:  tm,
		Provenance: provenance,
	}
	if rec.Source == "" {
		rec.Source = "memory:" + kind
	}

	list := fs.memories[repoID]
	found := false
	for i, m := range list {
		if m.ID == id {
			list[i] = rec
			found = true
			break
		}
	}
	if !found {
		list = append(list, rec)
	}
	fs.memories[repoID] = list
	_ = fs.save()

	return rec.Memory, nil
}

func (fs *FileStore) ImportMemory(repoID string, mem model.Memory, provenance []string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	tm := now()
	tok := mem.TokenCost
	if tok <= 0 {
		tok = textutil.EstimateTokens(mem.Content)
	}

	rec := memoryRecord{
		Memory:     mem,
		RepoID:     repoID,
		CreatedAt:  tm,
		UpdatedAt:  tm,
		Provenance: provenance,
	}
	rec.TokenCost = tok
	if rec.Source == "" {
		rec.Source = "memory:" + mem.Kind
	}

	list := fs.memories[repoID]
	found := false
	for i, m := range list {
		if m.ID == mem.ID {
			list[i] = rec
			found = true
			break
		}
	}
	if !found {
		list = append(list, rec)
	}
	fs.memories[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) ListMemories(repoID string, limit int) ([]model.Memory, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	list := fs.memories[repoID]
	sort.Slice(list, func(i, j int) bool {
		return list[i].UpdatedAt > list[j].UpdatedAt
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	out := make([]model.Memory, len(list))
	for i, m := range list {
		out[i] = m.Memory
	}
	return out, nil
}

func (fs *FileStore) SearchMemories(repoID string, task string, limit int) ([]model.Memory, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	list := fs.memories[repoID]
	terms := textutil.Tokens(task)
	if len(terms) == 0 {
		return fs.ListMemories(repoID, limit)
	}

	type scored struct {
		m model.Memory
		s float64
	}
	var scoredList []scored
	for _, m := range list {
		sc := textutil.BM25Score(task, m.Content, 0)
		if sc > 0 || len(terms) == 0 {
			scoredList = append(scoredList, scored{m: m.Memory, s: sc})
		}
	}
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].s > scoredList[j].s
	})
	if limit > 0 && len(scoredList) > limit {
		scoredList = scoredList[:limit]
	}
	out := make([]model.Memory, len(scoredList))
	for i, s := range scoredList {
		out[i] = s.m
	}
	return out, nil
}

func (fs *FileStore) Invalidate(repoID, id, currentRevision string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.memories[repoID]
	for i := range list {
		if list[i].ID == id {
			list[i].InvalidatedAtRevision = currentRevision
			list[i].UpdatedAt = now()
			break
		}
	}
	fs.memories[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) Validate(repoID, id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.memories[repoID]
	for i := range list {
		if list[i].ID == id {
			list[i].InvalidatedAtRevision = ""
			list[i].UpdatedAt = now()
			break
		}
	}
	fs.memories[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) InvalidateByGitChange(repoID string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.memories[repoID]
	for i := range list {
		if list[i].InvalidatedAtRevision == "" && (list[i].Authority == "source" || list[i].Authority == "commit") {
			list[i].UpdatedAt = now()
		}
	}
	fs.memories[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) IncrementMemoryReuse(repoID, id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.memories[repoID]
	for i := range list {
		if list[i].ID == id {
			list[i].ReuseCount++
			list[i].LastAccessedAt = now()
			break
		}
	}
	fs.memories[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) CurrentWorkItem(repoID string) (*model.WorkItem, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	list := fs.workItems[repoID]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].Status == "active" {
			wi := list[i]
			return &wi, nil
		}
	}
	return nil, nil
}

func (fs *FileStore) SetWorkItem(repoID string, title string, branch string) (*model.WorkItem, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	tm := now()
	list := fs.workItems[repoID]
	for i := range list {
		if list[i].Status == "active" {
			list[i].Status = "paused"
			list[i].UpdatedAt = tm
		}
	}
	wi := model.WorkItem{
		ID:        hashID(title + "|" + tm),
		Title:     strings.TrimSpace(title),
		Status:    "active",
		Branch:    branch,
		CreatedAt: tm,
		UpdatedAt: tm,
	}
	list = append(list, wi)
	fs.workItems[repoID] = list
	_ = fs.save()
	return &wi, nil
}

func (fs *FileStore) ListWorkItems(repoID string) ([]model.WorkItem, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return append([]model.WorkItem(nil), fs.workItems[repoID]...), nil
}

func (fs *FileStore) ImportWorkItem(repoID string, item model.WorkItem) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.workItems[repoID]
	found := false
	for i, wi := range list {
		if wi.ID == item.ID {
			list[i] = item
			found = true
			break
		}
	}
	if !found {
		list = append(list, item)
	}
	fs.workItems[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) StartSession(repoID, sessionID, workID, agent string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	sess := model.Session{
		ID:        sessionID,
		Agent:     agent,
		WorkItem:  workID,
		StartedAt: now(),
	}
	fs.sessions[repoID] = append(fs.sessions[repoID], sess)
	_ = fs.save()
	return nil
}

func (fs *FileStore) EndSession(repoID, sessionID string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.sessions[repoID]
	for i := range list {
		if list[i].ID == sessionID {
			list[i].EndedAt = now()
			break
		}
	}
	fs.sessions[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) LatestSession(repoID string) (*model.Session, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	list := fs.sessions[repoID]
	if len(list) == 0 {
		return nil, nil
	}
	s := list[len(list)-1]
	return &s, nil
}

func (fs *FileStore) ListSessions(repoID string) ([]model.Session, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if repoID == "" || repoID == "all" {
		var all []model.Session
		for _, list := range fs.sessions {
			all = append(all, list...)
		}
		return all, nil
	}
	return append([]model.Session(nil), fs.sessions[repoID]...), nil
}

func (fs *FileStore) ImportSession(repoID string, sess model.Session) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	list := fs.sessions[repoID]
	found := false
	for i, s := range list {
		if s.ID == sess.ID {
			list[i] = sess
			found = true
			break
		}
	}
	if !found {
		list = append(list, sess)
	}
	fs.sessions[repoID] = list
	_ = fs.save()
	return nil
}

func (fs *FileStore) AddEvent(repoID, sessionID, eventType string, payload string) error {
	ev := EventRecord{
		SessionID: sessionID,
		RepoID:    repoID,
		EventType: eventType,
		Payload:   payload,
		CreatedAt: now(),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(fs.baseDir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func newJSONLScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	// Allow lines up to 16MB for large traces and event payloads (default is only 64KB)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return scanner
}

func (fs *FileStore) ListEvents(sessionID string, limit int) ([]EventRecord, error) {
	if limit <= 0 {
		limit = 30
	}
	f, err := os.Open(filepath.Join(fs.baseDir, "events.jsonl"))
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var matched []EventRecord
	scanner := newJSONLScanner(f)
	for scanner.Scan() {
		var ev EventRecord
		if json.Unmarshal(scanner.Bytes(), &ev) == nil {
			if sessionID == "" || ev.SessionID == sessionID {
				matched = append(matched, ev)
			}
		}
	}
	if len(matched) > limit {
		matched = matched[len(matched)-limit:]
	}
	// Reverse to order by DESC
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	return matched, nil
}

func (fs *FileStore) SessionEventStats(sessionID string) (totalEvents int, invocations int, models map[string]int, err error) {
	models = make(map[string]int)
	f, err := os.Open(filepath.Join(fs.baseDir, "events.jsonl"))
	if err != nil {
		return 0, 0, models, nil
	}
	defer f.Close()

	scanner := newJSONLScanner(f)
	for scanner.Scan() {
		var rec EventRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if sessionID == "" || rec.SessionID == sessionID {
			totalEvents++
			if rec.EventType == "PreInvocation" {
				invocations++
			}
			var m map[string]any
			if json.Unmarshal([]byte(rec.Payload), &m) == nil {
				if mn, ok := m["modelName"].(string); ok && mn != "" {
					models[mn]++
				}
			}
		}
	}
	return totalEvents, invocations, models, nil
}

func (fs *FileStore) AddTrace(trace ContextTraceRecord) error {
	trace.CreatedAt = now()
	b, err := json.Marshal(trace)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(fs.baseDir, "traces.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func (fs *FileStore) LatestTrace(repoID string) (map[string]any, error) {
	f, err := os.Open(filepath.Join(fs.baseDir, "traces.jsonl"))
	if err != nil {
		return map[string]any{}, nil
	}
	defer f.Close()

	var last ContextTraceRecord
	found := false
	scanner := newJSONLScanner(f)
	for scanner.Scan() {
		var tr ContextTraceRecord
		if json.Unmarshal(scanner.Bytes(), &tr) == nil {
			if repoID == "" || tr.RepoID == repoID {
				last = tr
				found = true
			}
		}
	}
	if !found {
		return map[string]any{}, nil
	}
	hit := "0"
	if last.CacheHit {
		hit = "1"
	}
	return map[string]any{
		"task":            last.Task,
		"model":           last.Model,
		"budget":          fmt.Sprintf("%d", last.Budget),
		"selected_tokens": fmt.Sprintf("%d", last.SelectedTokens),
		"estimated_cost":  fmt.Sprintf("%f", last.EstimatedCost),
		"cache_hit":       hit,
		"decision_json":   last.DecisionJSON,
		"created_at":      last.CreatedAt,
	}, nil
}

func (fs *FileStore) ListTraces(repoID string, limit int) ([]ContextTraceRecord, error) {
	f, err := os.Open(filepath.Join(fs.baseDir, "traces.jsonl"))
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	var matched []ContextTraceRecord
	scanner := newJSONLScanner(f)
	for scanner.Scan() {
		var tr ContextTraceRecord
		if json.Unmarshal(scanner.Bytes(), &tr) == nil {
			if repoID == "" || tr.RepoID == repoID {
				matched = append(matched, tr)
			}
		}
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

func (fs *FileStore) TraceStats(repoID string) (int, int, int, error) {
	f, err := os.Open(filepath.Join(fs.baseDir, "traces.jsonl"))
	if err != nil {
		return 0, 0, 0, nil
	}
	defer f.Close()

	tokens, hits, count := 0, 0, 0
	scanner := newJSONLScanner(f)
	for scanner.Scan() {
		var tr ContextTraceRecord
		if json.Unmarshal(scanner.Bytes(), &tr) == nil {
			if repoID == "" || tr.RepoID == repoID {
				count++
				tokens += tr.SelectedTokens
				if tr.CacheHit {
					hits++
				}
			}
		}
	}
	return tokens, hits, count, nil
}

func (fs *FileStore) GetCache(key, repoID, revision, worktreeHash string) (string, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	c, ok := fs.caches[key]
	if !ok || c.RepoID != repoID || c.Revision != revision || c.WorktreeHash != worktreeHash {
		return "", nil
	}
	return c.PlanJSON, nil
}

func (fs *FileStore) PutCache(key, repoID, revision, worktreeHash, task, modelName string, budget int, planJSON string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.caches[key] = cacheRecord{
		CacheKey:     key,
		RepoID:       repoID,
		Revision:     revision,
		WorktreeHash: worktreeHash,
		TaskHash:     hashID(task),
		Model:        modelName,
		Budget:       budget,
		PlanJSON:     planJSON,
		HitCount:     0,
		CreatedAt:    now(),
	}
	_ = fs.save()
	return nil
}

func (fs *FileStore) IncrementCacheHit(key string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if c, ok := fs.caches[key]; ok {
		c.HitCount++
		c.LastHitAt = now()
		fs.caches[key] = c
		_ = fs.save()
	}
	return nil
}

func (fs *FileStore) Prune(opts PruneOptions) (PruneReport, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	var rep PruneReport
	rep.DryRun = opts.DryRun

	if opts.CacheTTL <= 0 {
		opts.CacheTTL = 7 * 24 * time.Hour
	}
	cacheCutoff := time.Now().UTC().Add(-opts.CacheTTL).Format(time.RFC3339Nano)

	// 1. Prune cache entries
	var keptCaches = make(map[string]cacheRecord)
	for k, c := range fs.caches {
		if c.RepoID == opts.RepoID && (c.Revision != opts.CurrentRevision || c.CreatedAt < cacheCutoff) {
			rep.CacheEntriesPruned++
			if opts.DryRun {
				keptCaches[k] = c
			}
		} else {
			keptCaches[k] = c
		}
	}
	if !opts.DryRun {
		fs.caches = keptCaches
	}

	// 2. Prune traces
	maxTraces := opts.MaxTraces
	if maxTraces <= 0 {
		maxTraces = 500
	}
	tracesFile := filepath.Join(fs.baseDir, "traces.jsonl")
	if f, err := os.Open(tracesFile); err == nil {
		var allTraces []ContextTraceRecord
		scanner := newJSONLScanner(f)
		for scanner.Scan() {
			var tr ContextTraceRecord
			if json.Unmarshal(scanner.Bytes(), &tr) == nil {
				allTraces = append(allTraces, tr)
			}
		}
		_ = f.Close()

		if len(allTraces) > maxTraces {
			rep.TracesPruned = len(allTraces) - maxTraces
			if !opts.DryRun {
				kept := allTraces[len(allTraces)-maxTraces:]
				if outF, err := os.Create(tracesFile); err == nil {
					for _, tr := range kept {
						if b, err := json.Marshal(tr); err == nil {
							_, _ = outF.Write(append(b, '\n'))
						}
					}
					_ = outF.Close()
				}
			}
		}
	}

	// 3. Prune events
	if opts.EventTTL <= 0 {
		opts.EventTTL = 30 * 24 * time.Hour
	}
	eventCutoff := time.Now().UTC().Add(-opts.EventTTL).Format(time.RFC3339Nano)
	eventsFile := filepath.Join(fs.baseDir, "events.jsonl")
	if f, err := os.Open(eventsFile); err == nil {
		var keptEvents []EventRecord
		scanner := newJSONLScanner(f)
		for scanner.Scan() {
			var ev EventRecord
			if json.Unmarshal(scanner.Bytes(), &ev) == nil {
				if ev.RepoID == opts.RepoID && ev.CreatedAt < eventCutoff {
					rep.EventsPruned++
				} else {
					keptEvents = append(keptEvents, ev)
				}
			}
		}
		_ = f.Close()

		if !opts.DryRun && rep.EventsPruned > 0 {
			if outF, err := os.Create(eventsFile); err == nil {
				for _, ev := range keptEvents {
					if b, err := json.Marshal(ev); err == nil {
						_, _ = outF.Write(append(b, '\n'))
					}
				}
				_ = outF.Close()
			}
		}
	}

	// 4. Stale memories pruning (only if opted in)
	if opts.PruneStaleMemories {
		if opts.StaleTTL <= 0 {
			opts.StaleTTL = 60 * 24 * time.Hour
		}
		staleCutoff := time.Now().UTC().Add(-opts.StaleTTL).Format(time.RFC3339Nano)
		var keptMemories []memoryRecord
		for _, m := range fs.memories[opts.RepoID] {
			if m.InvalidatedAtRevision != "" && m.ReuseCount == 0 && m.UpdatedAt < staleCutoff {
				rep.MemoriesPruned++
			} else {
				keptMemories = append(keptMemories, m)
			}
		}
		if !opts.DryRun && rep.MemoriesPruned > 0 {
			fs.memories[opts.RepoID] = keptMemories
		}
	}

	if !opts.DryRun {
		_ = fs.save()
	}

	return rep, nil
}
