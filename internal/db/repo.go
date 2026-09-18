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
	} {
		_, _ = d.Exec(q)
	}
	// Backfill the accelerator for databases created before FTS existed.
	_, _ = d.Exec(`INSERT OR IGNORE INTO memory_fts(memory_id,content,kind) SELECT id,content,kind FROM memories`)
	return nil
}
