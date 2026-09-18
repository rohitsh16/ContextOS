# ContextOS v0.6 implementation checkpoint

This package is the current research prototype.

## Implemented

1. Persistent SQLite state.
2. Repository and Git revision/worktree fingerprinting.
3. File and symbol indexing.
4. Lightweight repository reference graph.
5. Work items and agent sessions.
6. Automatic hook ingestion.
7. Structured memories + evidence.
8. FTS5 prefiltering.
9. Deterministic lexical + hashed-semantic scoring.
10. Authority/freshness filtering.
11. Token-budgeted ASC allocation.
12. Stable/variable context layout.
13. Persistent context cache.
14. MCP server.
15. Claude/Cursor/Codex/Gemini installers.
16. Cross-agent handoff.
17. Resume + trace + statistics.
18. Synthetic benchmark.
19. Regression tests for persistence, cache invalidation, hooks and installation idempotence.

## Important limitations

- The semantic scorer is deterministic and local; it is not a neural embedding model.
- Memory extraction is intentionally conservative/heuristic in v0.6.
- Git invalidation uses repository revision/worktree fingerprints for context-cache correctness; semantic claim revalidation remains conservative.
- Provider model pricing profiles are illustrative reference values, not billing truth. Update `internal/router/router.go` before using `context_route` as an accounting source.
- Provider hook schemas change over time; the installers target the currently documented formats but should be validated after CLI upgrades.

## Suggested developer flow

```bash
make
make install-user
cd /path/to/repo
ctx setup
ctx resume
```

For debugging:

```bash
ctx plan -task "..." -budget 4000 -render
ctx trace
ctx stats
```

For an explicit MCP server:

```bash
contextd -repo /path/to/repo -mcp
```
