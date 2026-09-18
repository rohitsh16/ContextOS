---
name: contextos
description: Use ContextOS to manage persistent agent context, execute 6-pass token allocations, recall durable engineering decisions, and record session traces.
---

# ContextOS Agent Workflow

ContextOS provides deterministic, budget-bounded context management for autonomous AI workflows.

## CLI Quick Access

When working in this repository or any ContextOS-enabled workspace, you can directly run:

```bash
# Resume latest branch state, uncommitted changes, and active decisions
./bin/ctx resume

# Plan minimum-sufficient context under a strict token budget (e.g., 2000 tokens)
./bin/ctx plan -task "Fix database race condition" -budget 2000

# Remember key decisions or bug fixes
./bin/ctx remember -kind decision -authority user "Use sync.Mutex in DB wrapper for thread-safe SQLite operations"

# View allocation traces and cache token savings
./bin/ctx trace
./bin/ctx stats
```

## MCP Stdio Protocol

The MCP daemon runs over stdio via:
```bash
./bin/contextd -repo . -mcp
```
Exposing:
- `context_resume`
- `context_plan`
- `context_remember`
- `context_search`
- `context_invalidate`
- `context_trace`
- `context_stats`
- `context_work_start`
- `context_session_start`
- `context_event`
- `context_route`
- `context_handoff`
