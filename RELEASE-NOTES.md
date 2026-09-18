# ContextOS 0.6.0 final research prototype

## Included

- local SQLite/WAL persistence
- Git revision + working-tree cache identity
- repository/file/symbol indexing
- lightweight dependency/reference graph
- structured engineering memory and evidence
- FTS5 + deterministic hybrid scoring baseline
- Adaptive Sufficient Context (ASC) budgeted allocator
- stable-prefix/variable-context prompt layout
- persistent context-plan cache
- automatic hook capture
- context injection for supported Claude/Cursor/Gemini hook events
- Codex durable capture + MCP context access
- project MCP installation for Claude/Cursor/Codex/Gemini
- cross-agent handoff and resume
- context traces and synthetic benchmark
- unit/regression tests

## Validation

The release was validated with:

```text
go test ./...        PASS
go vet ./...         PASS
make                 PASS
real Git repo demo   PASS
MCP initialize       PASS
hook injection       PASS
cache reuse          PASS
worktree cache invalidation PASS
installer idempotence PASS
```

The bundled binaries are Linux x86-64 builds. Build from source for macOS/Windows/other architectures.

## Research status

This is a research prototype, not a production enterprise service. The largest remaining research component is replacing deterministic context scoring with a learned marginal-utility policy trained from real longitudinal coding-agent traces.
