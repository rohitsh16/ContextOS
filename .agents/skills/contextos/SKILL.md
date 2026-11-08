---
name: contextos
description: Use ContextOS to manage persistent agent context, execute 6-pass token allocations, recall durable engineering decisions, generate adaptive compute plans, and record session traces.
---

# ContextOS Agent Workflow & Reference

ContextOS provides budget-bounded context management, adaptive compute routing, and persistent engineering memory for AI coding agents.

## Core Capabilities

- **Evidence Admission & Provenance**: Strictly bounds retrieval to authoritative source files and tests, excluding build artifacts, caches, vendored code, and agent worktrees.
- **Evidence Graph & Minimum Sufficient Context**: Links symbols and memories into a dependency graph, using marginal-utility knapsack packing to pick only the essential context within budget.
- **Claim Verification & Contradiction Detection**: Flags conflicting statements (e.g. outdated architectural assumptions) and gates answers against verified code evidence.
- **Exact Path & Symbol Lookups**: Targeting exact filenames (e.g. `planner.go`) or function names bypasses heuristic search for instant 100% precision.
- **Adaptive Compute Planning**: Recommends reasoning effort, token budgets, and verification tiers based on task complexity.
- **Prefix Caching**: Structures prompts into stable prefixes (decisions, constraints, types) to maximize LLM KV-cache hits.

---

## MCP Tools Reference (`contextos`)

The ContextOS MCP daemon runs over stdio via:
```bash
./bin/contextd -repo . -mcp
```

### 1. `context_resume`
Recovers active repository state, active git branch, uncommitted diffs, open work items, and high-authority engineering decisions.
- **Arguments**: None.

### 2. `context_plan`
Selects minimum-sufficient, cache-aware context under a strict token budget.
- **Arguments**:
  - `task` (string, required): Task description or query. Can include explicit file paths or symbols.
  - `budget` (integer, optional): Token budget ceiling (default: 4000).
  - `model` (string, optional): Target model name (e.g. `gemini-2.5-flash`, `claude-3-5-sonnet`, `local`).
  - `timeout_ms` (integer, optional): Soft deadline in milliseconds.
  - `adaptive_budget` (boolean, optional): Enable budget floor adaptation if deadline approaches.

### 3. `context_compute_plan`
Generates an adaptive compute plan with recommended reasoning effort, verification tier, and model routing.
- **Arguments**:
  - `task` (string, required): Description of the engineering task.
  - `risk_target` (number, optional): Target failure risk tolerance (0.0 to 1.0).
  - `preferred_provider` (string, optional): Preferred model family (`google`, `anthropic`, `local`).

### 4. `context_remember`
Persists durable engineering knowledge, architectural choices, and failure modes across sessions.
- **Arguments**:
  - `kind` (string, required): Category: `"decision"`, `"failure"`, `"workflow"`, or `"learning"`.
  - `content` (string, required): Descriptive text of the decision or learned constraint.
  - `authority` (string, optional): Authority level: `"user"`, `"verified"`, or `"agent"` (default: `"agent"`).

### 5. `context_search`
Searches durable engineering memory and repository symbol context with exact + semantic fallback.
- **Arguments**:
  - `task` (string, required): Search query or keywords.
  - `limit` (integer, optional): Maximum results to return (default: 10).
  - `timeout_ms` (integer, optional): Timeout in milliseconds.

### 6. `context_invalidate`
Invalidates a stale memory at the current git commit revision, suppressing it from future plans while preserving historical audit logs.
- **Arguments**:
  - `id` (string, required): Unique ID of the memory to invalidate.

### 7. `context_route`
Recommends the optimal model based on task complexity, verification tier, and token budget.
- **Arguments**:
  - `task` (string, required): Engineering task prompt.
  - `budget` (integer, optional): Token budget.

### 8. `context_handoff`
Produces a compact, self-contained handoff context payload for cross-agent or cross-model task transitions.
- **Arguments**:
  - `task` (string, required): Task description.
  - `target_model` (string, optional): Destination agent or model architecture.
  - `budget` (integer, optional): Token budget for the handoff state.

### 9. `context_work_start`
Starts a persistent engineering work item tracked across agent sessions.
- **Arguments**:
  - `title` (string, required): Title/goal of the work item.

### 10. `context_session_start`
Initializes a tracked agent session tied to a work item.
- **Arguments**:
  - `agent` (string, optional): Agent identifier (e.g. `antigravity`).
  - `work_item` (string, optional): Associated work item ID.

### 11. `context_event`
Records an agent, tool, or lifecycle event into the persistent audit trail.
- **Arguments**:
  - `event_type` (string, required): Event classification (e.g. `tool_call`, `milestone`, `error`).
  - `payload` (string, required): JSON string or descriptive payload.
  - `session_id` (string, optional): Session identifier.

### 12. `context_stats`
Displays runtime statistics: cache hit rates, token savings, active memory counts, and symbol count.
- **Arguments**: None.

### 13. `context_trace`
Displays the latest context allocation trace, multi-pass candidate selections, and token breakdown.
- **Arguments**: None.

---

## MCP Resources

ContextOS exposes real-time contextual resources via the `context://` scheme:
- `context://memory/relevant`: Top durable memories relevant to recent context.
- `context://repo/current`: Current repository metadata, git branch, and revision.
- `context://session/latest`: Most recent agent session state and history.
- `context://trace/latest`: Most recent context allocation plan and decisions.
- `context://work-item/current`: Active engineering work item details.

---

## CLI Command Suite (`ctx`)

```bash
# 1. Recover active work state and branch decisions
ctx resume -repo .

# 2. Plan minimum-sufficient context under a token budget
ctx plan -repo . -task "Refactor exact path retrieval" -budget 3500

# 3. Route task and generate adaptive compute recommendations
ctx route -task "Fix race condition in SQLite transaction pool" -budget 4000

# 4. Persist durable decisions or dead ends
ctx remember -repo . -kind decision -authority user -content "Use sync.RWMutex for graph degree caching"
ctx remember -repo . -kind failure -authority agent -content "Do not use unbuffered channels for hook dispatch"

# 5. Search durable memories and repository symbols
ctx search -repo . -task "planner soundness"

# 6. Start persistent work item
ctx work -repo . -title "R17 Verification and Invariant Testing"

# 7. Check runtime statistics and cache efficiency
ctx stats -repo .

# 8. Audit admissible evidence universe vs excluded artifacts
ctx audit -repo .

# 9. Run empirical correctness benchmark
ctx bench correctness -suite all

# 10. Launch real-time web UI dashboard (default port 8765)
ctx ui -repo . -port 8765
```

---

## Standard Agent Workflows

### Workflow 1: Resuming & Orienting
1. Call `context_resume` (or run `ctx resume -repo .`).
2. Review uncommitted git changes, active branch, and recalled decisions before proposing changes.

### Workflow 2: Targeted Code Modifications
1. Call `context_plan` with specific file paths or basenames (e.g. `internal/retrieval/planner.go`).
2. Exact retrieval guarantees direct index resolution without symbol dilution.
3. Review selected nodes and proceed with edits.

### Workflow 3: Documenting Discoveries & Avoiding Regressions
1. When an architectural choice is finalized, call `context_remember` (`kind: "decision"`).
2. When a bug investigation reveals an invalid approach or breaking attempt, record it with `context_remember` (`kind: "failure"`).
3. If an older decision is superseded, call `context_invalidate` with its memory ID.
