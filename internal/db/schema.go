package db

const Schema = `
PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS schema_meta(k TEXT PRIMARY KEY,v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS repositories(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 path TEXT NOT NULL UNIQUE,
 name TEXT NOT NULL,
 revision TEXT NOT NULL,
 branch TEXT NOT NULL,
 worktree_hash TEXT NOT NULL DEFAULT '',
 indexed_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS revisions(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 repo_id INTEGER NOT NULL,
 revision TEXT NOT NULL,
 branch TEXT NOT NULL,
 observed_at TEXT NOT NULL,
 UNIQUE(repo_id,revision),
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS work_items(
 id TEXT PRIMARY KEY,
 repo_id INTEGER NOT NULL,
 title TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'active',
 branch TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS memories(
 id TEXT PRIMARY KEY,
 repo_id INTEGER NOT NULL,
 work_item_id TEXT,
 kind TEXT NOT NULL,
 content TEXT NOT NULL,
 scope TEXT NOT NULL DEFAULT 'repo',
 valid_from_revision TEXT,
 invalidated_at_revision TEXT,
 authority TEXT NOT NULL,
 confidence REAL NOT NULL,
 token_cost INTEGER NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 reuse_count INTEGER NOT NULL DEFAULT 0,
 last_accessed_at TEXT,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE,
 FOREIGN KEY(work_item_id) REFERENCES work_items(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS evidence(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 memory_id TEXT NOT NULL,
 source TEXT NOT NULL,
 source_type TEXT NOT NULL,
 revision TEXT,
 FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS claims(
 id TEXT PRIMARY KEY,
 memory_id TEXT NOT NULL,
 claim TEXT NOT NULL,
 authority TEXT NOT NULL,
 confidence REAL NOT NULL,
 created_at TEXT NOT NULL,
 FOREIGN KEY(memory_id) REFERENCES memories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS nodes(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 repo_id INTEGER NOT NULL,
 kind TEXT NOT NULL,
 path TEXT NOT NULL,
 name TEXT NOT NULL,
 start_line INTEGER,
 end_line INTEGER,
 signature TEXT,
 content_hash TEXT,
 UNIQUE(repo_id, kind, path, name, start_line),
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS edges(
 src_id INTEGER NOT NULL,
 dst_id INTEGER NOT NULL,
 kind TEXT NOT NULL,
 UNIQUE(src_id,dst_id,kind),
 FOREIGN KEY(src_id) REFERENCES nodes(id) ON DELETE CASCADE,
 FOREIGN KEY(dst_id) REFERENCES nodes(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS sessions(
 id TEXT PRIMARY KEY,
 repo_id INTEGER NOT NULL,
 work_item_id TEXT,
 agent TEXT NOT NULL,
 started_at TEXT NOT NULL,
 ended_at TEXT,
 summary TEXT,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE,
 FOREIGN KEY(work_item_id) REFERENCES work_items(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS events(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 session_id TEXT,
 repo_id INTEGER,
 event_type TEXT NOT NULL,
 payload TEXT NOT NULL,
 created_at TEXT NOT NULL,
 FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE SET NULL,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS context_traces(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 repo_id INTEGER,
 work_item_id TEXT,
 task TEXT NOT NULL,
 model TEXT,
 budget INTEGER NOT NULL,
 selected_tokens INTEGER NOT NULL,
 estimated_cost REAL NOT NULL,
 cache_hit INTEGER NOT NULL DEFAULT 0,
 decision_json TEXT NOT NULL,
 created_at TEXT NOT NULL,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE,
 FOREIGN KEY(work_item_id) REFERENCES work_items(id) ON DELETE SET NULL
);
CREATE TABLE IF NOT EXISTS context_cache(
 cache_key TEXT PRIMARY KEY,
 repo_id INTEGER NOT NULL,
 revision TEXT NOT NULL,
 worktree_hash TEXT NOT NULL DEFAULT '',
 task_hash TEXT NOT NULL,
 model TEXT,
 budget INTEGER NOT NULL,
 plan_json TEXT NOT NULL,
 hit_count INTEGER NOT NULL DEFAULT 0,
 last_hit_at TEXT,
 created_at TEXT NOT NULL,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS policy_events(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 repo_id INTEGER,
 event_type TEXT NOT NULL,
 payload TEXT NOT NULL,
 created_at TEXT NOT NULL,
 FOREIGN KEY(repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_mem_repo ON memories(repo_id,kind,updated_at);
CREATE INDEX IF NOT EXISTS idx_mem_valid ON memories(repo_id,invalidated_at_revision,valid_from_revision);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(memory_id UNINDEXED, content, kind UNINDEXED);
CREATE INDEX IF NOT EXISTS idx_nodes_repo ON nodes(repo_id,path,kind,name);
CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_id,kind);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_id,kind);
CREATE INDEX IF NOT EXISTS idx_events_repo ON events(repo_id,created_at);
CREATE INDEX IF NOT EXISTS idx_cache_repo ON context_cache(repo_id,revision);
`
