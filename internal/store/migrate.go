package store

import (
	"fmt"

	"contextos/internal/gitidx"
)

// MigrationReport summarizes the results of transferring data between two stores.
type MigrationReport struct {
	SourceType        string `json:"source_type"`
	DestinationType   string `json:"destination_type"`
	RepoID            string `json:"repo_id"`
	RepoPath          string `json:"repo_path"`
	MemoriesMigrated  int    `json:"memories_migrated"`
	NodesMigrated     int    `json:"nodes_migrated"`
	WorkItemsMigrated int    `json:"work_items_migrated"`
	SessionsMigrated  int    `json:"sessions_migrated"`
	EventsMigrated    int    `json:"events_migrated"`
	TracesMigrated    int    `json:"traces_migrated"`
}

// Migrate transfers all data for the specified repository from src Store to dst Store.
// Supports transferring between SQLiteStore and FileStore bidirectionally.
func Migrate(src Store, dst Store, repo gitidx.Repo) (*MigrationReport, error) {
	if src == nil || dst == nil {
		return nil, fmt.Errorf("source and destination stores must not be nil")
	}

	srcType := "sqlite"
	if _, ok := src.(*FileStore); ok {
		srcType = "file"
	}
	dstType := "file"
	if _, ok := dst.(*SQLiteStore); ok {
		dstType = "sqlite"
	}

	// 1. Get or create repo in destination
	srcRepoID, err := src.GetOrCreateRepo(repo.Path, repo.Name, repo.Revision, repo.Branch, repo.WorktreeHash)
	if err != nil {
		return nil, fmt.Errorf("lookup source repo: %w", err)
	}

	dstRepoID, err := dst.GetOrCreateRepo(repo.Path, repo.Name, repo.Revision, repo.Branch, repo.WorktreeHash)
	if err != nil {
		return nil, fmt.Errorf("create destination repo: %w", err)
	}
	_ = dst.UpdateRepo(dstRepoID, repo.Revision, repo.Branch, repo.WorktreeHash)
	_ = dst.AddRevision(dstRepoID, repo.Revision, repo.Branch)

	rep := &MigrationReport{
		SourceType:      srcType,
		DestinationType: dstType,
		RepoID:          dstRepoID,
		RepoPath:        repo.Path,
	}

	// 2. Transfer Nodes
	nodes, err := src.ListNodes(srcRepoID)
	if err == nil && len(nodes) > 0 {
		var files []gitidx.SourceFile
		var syms []gitidx.Symbol
		for _, n := range nodes {
			if n.Kind == "file" {
				files = append(files, gitidx.SourceFile{Path: n.Path, Lines: n.EndLine, Hash: n.ContentHash})
			} else {
				syms = append(syms, gitidx.Symbol{
					Path:      n.Path,
					Kind:      n.Kind,
					Name:      n.Name,
					Start:     n.StartLine,
					End:       n.EndLine,
					Signature: n.Signature,
					Hash:      n.ContentHash,
				})
			}
		}
		_ = dst.SaveNodesAndEdges(dstRepoID, files, syms, nil)
		rep.NodesMigrated = len(nodes)
	}

	// 3. Transfer Memories
	mems, err := src.ListMemories(srcRepoID, 100000)
	if err == nil && len(mems) > 0 {
		for _, m := range mems {
			if err := dst.ImportMemory(dstRepoID, m, nil); err == nil {
				rep.MemoriesMigrated++
			}
		}
	}

	// 4. Transfer WorkItems
	items, err := src.ListWorkItems(srcRepoID)
	if err == nil && len(items) > 0 {
		for _, wi := range items {
			if err := dst.ImportWorkItem(dstRepoID, wi); err == nil {
				rep.WorkItemsMigrated++
			}
		}
	}

	// 5. Transfer Sessions and Events
	sessions, err := src.ListSessions(srcRepoID)
	if err == nil && len(sessions) > 0 {
		for _, s := range sessions {
			if err := dst.ImportSession(dstRepoID, s); err == nil {
				rep.SessionsMigrated++
			}
			events, err := src.ListEvents(s.ID, 10000)
			if err == nil && len(events) > 0 {
				for _, ev := range events {
					if err := dst.AddEvent(dstRepoID, ev.SessionID, ev.EventType, ev.Payload); err == nil {
						rep.EventsMigrated++
					}
				}
			}
		}
	}

	// 6. Transfer Traces
	traces, err := src.ListTraces(srcRepoID, 10000)
	if err == nil && len(traces) > 0 {
		for _, tr := range traces {
			tr.RepoID = dstRepoID
			if err := dst.AddTrace(tr); err == nil {
				rep.TracesMigrated++
			}
		}
	}

	return rep, nil
}
