# ContextOS

**Persistent, budget-aware context for AI coding agents.**

ContextOS is a local-first context runtime that keeps engineering state across sessions, agents, repository revisions, and machine restarts. It builds minimum-sufficient context packages under a token budget — so your agents remember decisions, avoid past failures, and stay efficient.

Works with **Claude Code**, **Cursor**, **Codex**, **Gemini CLI**, and **Antigravity** out of the box.

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

> **Storage engines**: ContextOS supports two storage backends:
> - **SQLite** (default): High-performance embedded DB with WAL mode and FTS5.
> - **FileStore** (`-storage file` or `CONTEXTOS_STORAGE=file`): Zero-dependency, pure Go standard library storage engine (zero CGO, zero external libraries). Ideal when SQLite or CGO is unavailable.
>
> Default database path is `~/.contextos/context.db` (SQLite) or `~/.contextos/data` (FileStore). Override via `-db PATH` or `CONTEXTOS_DB=/path/to/storage`.

---

## What It Does

| Capability | Detail |
|---|---|
| **Persistent memory** | Facts, decisions, constraints, failures survive across sessions |
| **Git-aware** | Tracks revisions, branches, worktree state; auto-invalidates stale context |
| **Code indexing** | Extracts symbols, types, functions + builds a reference graph |
| **7-Stage Hybrid Retrieval** | Exact path, symbol AST, entity, BM25, semantic hash, causal expansion & RRF rerank |
| **Budget allocation** | Greedy marginal-utility allocator stays within token limits |
| **Prompt-cache aware** | Stable prefix / variable context ordering for provider cache hits |
| **Compute optimization** | Capability-preserving optimizer, reasoning effort levels (minimal→maximum) & thinking budgets |
| **Context caching** | Identical (revision, task, model, budget) → cached plan, zero recompute |
| **Hook capture** | Records prompts, tool calls, failures automatically per agent |
| **Cross-agent handoff** | Transfer context between models/agents with `ctx handoff` |
| **Model routing** | Recommends model by task complexity, budget, and capability envelope |
| **MCP server** | 13 tools + 5 resources over JSON-RPC stdio |
| **Web UI dashboard** | Local visual dashboard with token savings simulator and graph index viewer |
| **Enterprise SaaS** | Multi-tenant isolation, rate limiting, API keys, token metering & Stripe-ready invoicing |

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

### 7-stage hybrid retrieval with candidate inspection
```bash
# Retrieve top ranked evidence candidates with execution trace
ctx retrieve -repo . -task "Fix duplicate payment processing" -top 20 -trace -format md
```

### Launch local real-time Web UI Dashboard
```bash
# Start visual dashboard on localhost:8765
ctx ui -repo . -port 8765
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

### Route to the best model and compute envelope
```bash
ctx route -task "distributed lock migration" -budget 8000
```

### Run correctness & benchmark gates
```bash
ctx bench correctness -suite standard -format table
```

### Switch storage engines (Migrate SQLite ↔ FileStore)
Transfer all repositories, AST nodes, memories, work items, sessions, events, and traces without data loss:
```bash
# Transfer SQLite data to zero-DB FileStore
ctx migrate -to file

# Transfer FileStore data back to SQLite
ctx migrate -to sqlite

# Specify custom source and destination paths
ctx migrate -from sqlite -to file -from-path ~/.contextos/context.db -to-path ~/.contextos/data
```

### Storage Garbage Collection (Opt-In Pruning)
```bash
# Dry run to see what expired entries would be pruned
ctx gc -repo . -keep-days 30 -dry-run

# Run garbage collection
ctx gc -repo . -keep-days 30

# Enable automated pruning during context planning (opt-in feature flag)
ctx plan -task "refactor storage" -auto-prune
# Or set CONTEXTOS_AUTO_PRUNE=1
```

---

## Full CLI Reference

```
ctx init|index     -repo PATH                          Index repository symbols
ctx remember       -repo PATH -kind K -content '...'   Persist a memory
ctx plan           -repo PATH -task '...' [-model M]   Build context plan
                   [-budget N] [-render] [-auto-prune]
ctx retrieve       -repo PATH -task '...' [-top N]     7-stage hybrid candidate retrieval
                   [-trace] [-format md|json]
ctx resume         -repo PATH                          Recover active work state
ctx handoff        -repo PATH -task '...' -target M    Cross-agent handoff
ctx invalidate     -repo PATH -id MEMORY_ID            Invalidate a memory
ctx gc             -repo PATH [-keep-days N] [-dry-run]Garbage collect expired cache & traces
ctx migrate        -to file|sqlite [-from file|sqlite] Transfer data between storage engines
ctx work           -repo PATH -title "..."             Start a work item
ctx session        -repo PATH -agent NAME              Start an agent session
ctx event          -repo PATH -event TYPE -payload J   Record an event
ctx stats          -repo PATH                          Show statistics
ctx route          -task "..." -budget N               Recommend a model & effort level
ctx ui             -repo PATH [-port 8765]             Launch real-time web UI dashboard
ctx bench          [correctness|retrieval|mse]         Run correctness & retrieval benchmarks
ctx audit          -repo PATH [-format md|json]        Audit admission and exclusion universe
ctx report         -repo PATH [-format md|json]        Generate evaluation/benchmark report
ctx publish        -repo PATH [-output FILE]           Publish empirical test results to markdown
ctx install        -repo PATH -agent claude|cursor|    Install for one agent
                   codex|gemini|antigravity|all
ctx uninstall      -repo PATH [-agent NAME]            Uninstall agent integrations
ctx setup          -repo PATH                          Index + install all
ctx hook           -agent NAME -event TYPE < stdin     Process hook event
ctx completion     [bash|zsh]                          Generate shell completion script

Storage Options & Feature Flags:
  -storage sqlite|file    Choose storage engine (or CONTEXTOS_STORAGE)
  -auto-prune             Opt-in automated storage pruning (or CONTEXTOS_AUTO_PRUNE=1)
  -db PATH                Custom SQLite DB or file store directory
```

---

## MCP Server

The MCP server runs locally over stdio — no network, no API keys:

```bash
# Start standalone MCP server for a repo
ctx -repo /path/to/repo -mcp

# Or via the daemon binary directly
contextd -repo /path/to/repo -mcp
```

### Supported Tools (13)

| Tool | Description |
|---|---|
| `context_plan` | Select minimum-sufficient, cache-aware context under a token budget with adaptive timeout mitigation |
| `context_remember` | Persist durable engineering knowledge (decisions, constraints, failures, patterns) |
| `context_resume` | Recover active repository/work state and recent context |
| `context_invalidate` | Invalidate a stale or superseded memory at the current revision |
| `context_search` | Search durable engineering memory and repository context with hybrid BM25 + semantic matching |
| `context_compute_plan` | Generate provider-neutral adaptive compute plan (reasoning effort, token budget, model routing, verification tier) |
| `context_handoff` | Produce structured cross-agent handoff context |
| `context_work_start` | Start tracking a persistent engineering work item |
| `context_session_start` | Start an agent session with associated metadata |
| `context_event` | Record an agent/tool lifecycle event |
| `context_stats` | Display runtime statistics and token allocation metrics |
| `context_trace` | Show the latest context allocation plan and decisions |
| `context_route` | Recommend an optimal model and reasoning effort using task complexity and budget |

### Supported Resources (5)

| URI | Content |
|---|---|
| `context://memory/relevant` | Top durable engineering memories relevant to the recent context |
| `context://repo/current` | Current repository metadata, git revision, and active branch |
| `context://session/latest` | Most recent agent session state and history |
| `context://trace/latest` | Most recent context allocation trace, candidate scores, and decisions |
| `context://work-item/current` | Active engineering work item details |

---

## Agent Integrations

`ctx setup` automatically configures hooks + MCP for all supported agents:

| Agent | Hooks File | MCP Config | Rules / Context Guidance |
|---|---|---|---|
| **Claude Code** | `.claude/settings.local.json` | `.mcp.json` | — |
| **Cursor** | `.cursor/hooks.json` | `.cursor/mcp.json` | `.cursor/rules/contextos.mdc` |
| **Codex** | `.codex/hooks.json` | `.codex/config.toml` | — |
| **Gemini CLI** | `.gemini/settings.json` | `.gemini/settings.json` | — |
| **Antigravity** | `.agents/hooks.json` | `.agents/mcp_config.json` | `.agents/rules/contextos.md` |

Install for a single agent:

```bash
ctx install -repo . -agent antigravity
```

All installations are idempotent — running setup twice won't duplicate entries.

---

## Architecture

```
Claude / Cursor / Codex / Gemini / Antigravity
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

### Authoritative 7-Stage Hybrid Retrieval Pipeline

ContextOS uses an inspectable 7-stage evidence retrieval pipeline with end-to-end stage telemetry:

```
Task Query ──► 1. Exact Path Match    ──► Pinpointed file and line ranges
           ──► 2. AST Symbol Nav       ──► Types, interfaces, functions & methods
           ──► 3. Entity Resolution    ──► Package & identifier resolution
           ──► 4. Lexical (BM25)       ──► Term-frequency keyword matches
           ──► 5. Semantic Hash        ──► Offline deterministic embedding similarity
           ──► 6. Causal Expansion    ──► Callers, callees, dependency graph traversal
           ──► 7. RRF Fusion & Rerank  ──► Reciprocal Rank Fusion candidate scoring
```

The scoring combines:
- **Semantic similarity** — hashed deterministic similarity (zero dependencies, offline)
- **Lexical overlap** — BM25 token intersection
- **Authority** — user > test > source > commit > doc > inference
- **Freshness** — revision-aware invalidation + staleness penalty
- **Memory kind** — decisions > failures > constraints > state > observations > code > facts
- **Evidence** — provenance-backed memories score higher
- **Reuse** — frequently-used context is prioritized
- **Cache value** — prompt-cache topology optimization

### Compute Optimization & Adaptive Reasoning Control

ContextOS actively solves the **minimum-sufficient compute** optimization problem:

$$\min E[C_{E2E}] \quad \text{subject to} \quad Q_{LCB} \ge Q_{min}$$

Where $Q_{LCB}$ is the empirical Lower Confidence Bound of task capability.

- **Dynamic Effort Negotiation**: Resolves provider-neutral effort levels (`minimal` $\rightarrow$ `low` $\rightarrow$ `medium` $\rightarrow$ `high` $\rightarrow$ `maximum`) mapped to thinking token budgets across models (OpenAI o1/o3, Claude 3.7 Sonnet extended thinking, Gemini 2.5 Pro).
- **Value of Information (VOI) Accounting**: Evaluates marginal utility $\Delta U$ against token expenditure before escalating inference compute.
- **Deterministic Bypass**: T0 deterministic tasks bypass expensive LLM calls completely (costing $\approx \$0.00005$ with sub-millisecond latency).

---

## Building

**Requirements**: Go 1.23+.

```bash
make          # Build all binaries to bin/ (SQLite support enabled)
make test     # Run all tests
make bench    # Run synthetic policy benchmark
make install-user  # Install to ~/.local/bin
make clean    # Remove build artifacts
```

The default build links system SQLite3 through CGO. On macOS, Apple clang works automatically. On Linux, install `libsqlite3-dev`.

### Zero-Dependency / Pure-Go Build (No CGO, No SQLite)
ContextOS can be built completely without CGO or external libraries for sandboxed or minimal environments:
```bash
CGO_ENABLED=0 go build -o bin/ctx ./cmd/ctx
CGO_ENABLED=0 go build -o bin/contextd ./cmd/contextd
```
When built with `CGO_ENABLED=0`, ContextOS defaults to the pure-Go **FileStore** engine automatically.

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

Given a task $q$, target model $\mathcal{A}$, and success threshold $\tau$, find the smallest context set $C^*$ such that:

$$
C^* = \arg\min_C \text{Tokens}(C) \quad \text{subject to} \quad P(\text{success} \mid C, q, \mathcal{A}) \ge \tau
$$

The current implementation uses our inspectable, mathematically-grounded 6-pass deterministic baseline (BM25 + RRF + Singleton Rescue + Fill Pass + KV Prefix Partitioning). The architecture is designed so the allocator can become a learned marginal-utility policy trained from real longitudinal traces — without changing storage or MCP interfaces.

See [`docs/CONTEXT.md`](docs/CONTEXT.md) for the architecture manual, [`docs/RESEARCH.md`](docs/RESEARCH.md) for formal mathematical theory and bounds, [`docs/ASC-1-SPEC.md`](docs/ASC-1-SPEC.md) for the specification, [`docs/EVALUATION.md`](docs/EVALUATION.md) for the evaluation framework and empirical benchmarks, [`docs/DASHBOARD.md`](docs/DASHBOARD.md) for the dashboard efficiency metrics glossary & deployment guide, and [`docs/integrations.md`](docs/integrations.md) for provider integration details.

---

## License

Licensed under the **Business Source License 1.1 (BSL 1.1)**:
- **Free** for personal, academic, and local developer use with AI coding agents.
- **Commercial & Hosted / SaaS** use requires a commercial license from the author.
- Converts to **Apache 2.0** on January 1, 2030.

See [`LICENSE`](LICENSE) for full terms or commercial inquiries.

