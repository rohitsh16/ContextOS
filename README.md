# ContextOS

**Persistent, budget-aware context for AI coding agents.**

ContextOS is a local-first context runtime that keeps engineering state across sessions, agents, repository revisions, and machine restarts. It builds minimum-sufficient context packages under a token budget — so your agents remember decisions, avoid past failures, and stay efficient.

Works with **Claude Code**, **Cursor**, **Codex**, and **Gemini CLI** out of the box.

---

## Quick Start

```bash
# 1. Build from source (requires Go 1.23+ and system SQLite3)
make

# 2. Install to ~/.local/bin
make install-user

# 3. Set up any repository — indexes code + installs hooks/MCP for all agents
cd /path/to/your/repo
ctx setup
```

That's it. Your agents now have persistent context.

> **Database location**: ContextOS stores state in `~/.contextos/context.db` by default. Override with `CONTEXTOS_DB=/path/to/db`.

---

## What It Does

| Capability | Detail |
|---|---|
| **Persistent memory** | Facts, decisions, constraints, failures survive across sessions |
| **Git-aware** | Tracks revisions, branches, worktree state; auto-invalidates stale context |
| **Code indexing** | Extracts symbols, types, functions + builds a reference graph |
| **Budget allocation** | Greedy marginal-utility allocator stays within token limits |
| **Prompt-cache aware** | Stable prefix / variable context ordering for provider cache hits |
| **Context caching** | Identical (revision, task, model, budget) → cached plan, zero recompute |
| **Hook capture** | Records prompts, tool calls, failures automatically per agent |
| **Cross-agent handoff** | Transfer context between models/agents with `ctx handoff` |
| **Model routing** | Recommends model by task complexity and budget |
| **MCP server** | 12 tools + 5 resources over JSON-RPC stdio |

---

## Everyday Usage

### Resume where you left off
```bash
ctx resume -repo .
```

### Plan context for a task
```bash
ctx plan -task "Fix duplicate payment processing" -budget 4000 -render
```

### Hand off to another agent
```bash
ctx handoff -task "Continue failover implementation" -target codex -budget 4000
```

### Store a decision
```bash
ctx remember -kind decision -authority user \
  -content "Use the outbox pattern because DB and Kafka are not atomic."
```

### Check stats
```bash
ctx stats -repo .
```

### Route to the best model
```bash
ctx route -task "distributed lock migration" -budget 8000
```

---

## Full CLI Reference

```
ctx init|index     -repo PATH                          Index repository symbols
ctx remember       -repo PATH -kind K -content '...'   Persist a memory
ctx plan           -repo PATH -task '...' [-model M]   Build context plan
                   [-budget N] [-render]
ctx resume         -repo PATH                          Recover work state
ctx handoff        -repo PATH -task '...' -target M    Cross-agent handoff
ctx invalidate     -repo PATH -id MEMORY_ID            Invalidate a memory
ctx work           -repo PATH -title "..."             Start a work item
ctx session        -repo PATH -agent NAME              Start an agent session
ctx event          -repo PATH -event TYPE -payload J   Record an event
ctx stats          -repo PATH                          Show statistics
ctx route          -task "..." -budget N               Recommend a model
ctx install        -repo PATH -agent claude|cursor|    Install for one agent
                   codex|gemini|all
ctx setup          -repo PATH                          Index + install all
ctx hook           -agent NAME -event TYPE < stdin     Process hook event
```

---

## MCP Server

The MCP server runs locally over stdio — no network, no API keys:

```bash
contextd -repo /path/to/repository -mcp
```

### Tools

| Tool | Description |
|---|---|
| `context_plan` | Select minimum-sufficient context under a token budget |
| `context_search` | Search durable engineering memory |
| `context_resume` | Recover latest work state |
| `context_handoff` | Produce cross-agent handoff context |
| `context_remember` | Persist engineering knowledge |
| `context_invalidate` | Invalidate a memory at current revision |
| `context_trace` | Show latest allocation trace |
| `context_stats` | Show ContextOS statistics |
| `context_work_start` | Start a persistent work item |
| `context_session_start` | Start an agent session |
| `context_event` | Record an agent/tool event |
| `context_route` | Recommend a model for task + budget |

### Resources

| URI | Description |
|---|---|
| `context://repo/current` | Current repository state |
| `context://work-item/current` | Active work item |
| `context://memory/relevant` | Relevant memories |
| `context://session/latest` | Latest agent session |
| `context://trace/latest` | Latest context trace |

---

## Agent Integrations

`ctx setup` automatically configures hooks + MCP for all supported agents:

| Agent | Hooks File | MCP Config |
|---|---|---|
| **Claude Code** | `.claude/settings.local.json` | `.mcp.json` |
| **Cursor** | `.cursor/hooks.json` | `.cursor/mcp.json` |
| **Codex** | `.codex/hooks.json` | `.codex/config.toml` |
| **Gemini CLI** | `.gemini/settings.json` | `.gemini/settings.json` |

Install for a single agent:

```bash
ctx install -repo . -agent claude
```

All installations are idempotent — running setup twice won't duplicate entries.

---

## Architecture

```
Claude / Cursor / Codex / Gemini
             |
        MCP + hooks
             |
             v
       +-----------+
       | ContextOS |
       +-----+-----+
             |
     +-------+--------+
     |                |
     v                v
Engineering      Cost / cache
state graph        policy
     |                |
     +-------+--------+
             v
     Candidate generation
             |
     Validity / authority
             |
     Marginal utility scoring
             |
     Budgeted greedy allocation
             |
     Stable prefix ordering
             |
         Model context
```

### Context Selection Pipeline

```
scope → validate → FTS/hybrid candidates → score → greedy budget allocation → order → cache
```

The scoring combines:
- **Semantic similarity** — hashed deterministic similarity (zero dependencies, offline)
- **Lexical overlap** — token intersection
- **Authority** — user > test > source > commit > doc > inference
- **Freshness** — revision-aware invalidation + staleness penalty
- **Memory kind** — decisions > failures > constraints > state > observations > code > facts
- **Evidence** — provenance-backed memories score higher
- **Reuse** — frequently-used context is prioritized
- **Cache value** — prompt-cache topology optimization

---

## Building

**Requirements**: Go 1.23+, system SQLite3 library, C compiler.

```bash
make          # Build all binaries to bin/
make test     # Run all tests
make bench    # Run synthetic policy benchmark
make install-user  # Install to ~/.local/bin
make clean    # Remove build artifacts
```

The core uses the system SQLite3 library through cgo. On macOS, the default Apple clang works. On Linux, gcc or clang with libsqlite3-dev.

---

## Binaries

| Binary | Purpose |
|---|---|
| `ctx` | CLI for all user-facing operations |
| `contextd` | MCP server daemon (stdio JSON-RPC) |
| `ctx-hook` | Standalone hook handler for agent integrations |
| `ctxbench` | Synthetic policy benchmark |

---

## Research Direction

The core problem is **minimum-sufficient context**:

> Given a task *q*, model *M*, and success threshold *τ*, find the smallest context set *C* such that *P(success | C, q, M) ≥ τ*.

The current implementation uses an inspectable deterministic baseline. The architecture is designed so the allocator can become a learned marginal-utility policy trained from real longitudinal traces — without changing storage or MCP interfaces.

See [`ASC-1-SPEC.md`](ASC-1-SPEC.md) for the formal problem definition, [`docs/EVALUATION.md`](docs/EVALUATION.md) for the evaluation plan, and [`docs/integrations.md`](docs/integrations.md) for provider integration details.

---

## License

Research prototype. See repository for terms.
