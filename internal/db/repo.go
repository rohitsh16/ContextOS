package db

func Init(d *DB) error {
	if err := d.ExecScript(Schema); err != nil {
		return err
	}
	// Lightweight compatibility migrations for databases created by v0/v0.2.
	for _, q := range []string{
		`ALTER TABLE context_traces ADD COLUMN work_item_id TEXT`,
		`ALTER TABLE context_cache ADD COLUMN task_hash TEXT`,
		`ALTER TABLE context_cache ADD COLUMN model TEXT`,
		`ALTER TABLE context_cache ADD COLUMN budget INTEGER`,
		`ALTER TABLE context_cache ADD COLUMN worktree_hash TEXT`,
		`ALTER TABLE repositories ADD COLUMN worktree_hash TEXT`,
		`ALTER TABLE memories ADD COLUMN last_accessed_at TEXT`,
		`ALTER TABLE nodes ADD COLUMN in_degree INTEGER DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN out_degree INTEGER DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN fan_in INTEGER DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN fan_out INTEGER DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN test_count INTEGER DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN package_degree INTEGER DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN centrality REAL DEFAULT 0.0`,
	} {
		_, _ = d.Exec(q)
	}
	_, _ = d.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_id)`)
	_, _ = d.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_id)`)
	_, _ = d.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_repo_name ON nodes(repo_id, name)`)
	_, _ = d.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_repo_path ON nodes(repo_id, path)`)
	// Backfill the accelerator for databases created before FTS existed.
	_, _ = d.Exec(`INSERT OR IGNORE INTO memory_fts(memory_id,content,kind) SELECT id,content,kind FROM memories`)
	return nil
}
