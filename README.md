# ContextOS

**Persistent, budget-aware memory for AI coding agents.**

ContextOS is a local context runtime that keeps state across sessions, agents, and git revisions. It works out of the box with **Claude Code**, **Cursor**, **Codex**, **Gemini CLI**, and **Antigravity**.

---

## Why ContextOS?

Today's AI coding assistants are powerful, but they have a fundamental limitation: **they have amnesia**.

1. **Lost context across sessions**: Every time you start a new chat, restart your IDE, or switch models, your agent starts from a blank slate. You have to re-explain your architecture and project conventions over and over.
2. **Repeated mistakes**: If an agent tries an approach that breaks your build or fails a test, it will happily try that same broken approach again in the next session.
3. **Bloated prompts and wasted money**: Agents typically dump entire files into their context window. This burns through your token budget, slows down response times, and increases API costs.
4. **Agent lock-in**: Context discovered in Cursor doesn't carry over when you switch to Claude Code or Codex.

**ContextOS solves this locally on your machine.** It indexes your code, tracks architectural decisions and past failures, and builds tightly packed, budget-bounded context packets that keep your agents fast, cheap, and aligned with your project.

---

## Key Features

- **Persistent Memory**: Decisions, constraints, active task state, and past failures survive across sessions, IDE restarts, and model changes.
- **Git Synchronization**: ContextOS tracks git branches and commit hashes. When code changes, outdated memories are automatically invalidated so agents never act on stale assumptions.
- **AST Symbol Indexing**: Extracts functions, types, and interfaces into a reference graph. Agents can query exact symbol definitions without reading whole files.
- **7-Stage Hybrid Retrieval**: Combines exact path matching, symbol lookup, entity resolution, BM25 text search, semantic hashing, call graph expansion, and rank fusion to find the best evidence.
- **Token Budget Allocation**: Packs context into a strict token budget (e.g., 4,000 tokens) using marginal-utility knapsack optimization, ensuring you never exceed model limits.
- **Prompt Cache Friendly**: Partitions context into a stable prefix (decisions, invariants, definitions) and dynamic task state. This maximizes prompt-cache hits with providers like Anthropic and OpenAI, cutting latency and costs.
- **Cross-Agent Handoff**: Pass tasks between tools with `ctx handoff` — start in Cursor, continue in Claude Code, and pick up in Antigravity without losing state.
- **Local MCP Server**: Exposes 13 tools and 5 resources over standard input/output (stdio JSON-RPC). No cloud accounts or external API keys needed.
- **Local Web Dashboard**: A built-in web UI (`ctx ui`) to inspect your memory graph, simulate token savings, and monitor active sessions.
- **Dual Storage Engines**: Uses embedded SQLite by default, or a pure-Go FileStore with zero dependencies and no CGO.

---

## Quick Start

### 1. Build & Install

**Prerequisites**: Go 1.23+ and a C compiler (for SQLite).

```bash
# Clone the repository
git clone https://github.com/your-username/ContextOS.git
cd ContextOS

# Build binaries (bin/ctx, bin/contextd, bin/ctx-hook, bin/ctxbench)
make

# Install `ctx` to ~/.local/bin
make install-user
```

> **Pure-Go Build (Zero Dependencies / No CGO)**:
> If you don't have SQLite or CGO available, you can build ContextOS as a pure-Go binary:
> ```bash
> CGO_ENABLED=0 go build -o bin/ctx ./cmd/ctx
> CGO_ENABLED=0 go build -o bin/contextd ./cmd/contextd
> ```
> ContextOS will automatically use the built-in **FileStore** engine.

### 2. Set Up Your Repository

Navigate to any codebase and run:

```bash
cd /path/to/your/project
ctx setup
```

`ctx setup` handles everything in one shot:
1. Indexes your repository's code symbols and git revision state.
2. Initializes the local database at `~/.contextos/context.db`.
3. Automatically detects and configures MCP servers and lifecycle hooks for Claude Code, Cursor, Codex, Gemini CLI, and Antigravity.

Your agents now have persistent, budget-aware memory.

---

## Everyday Usage

### Resume where you left off
Recover active branch state, open work items, recent decisions, and known gotchas:
```bash
ctx resume -repo .
```

### Plan context for a task
Assemble the most relevant code definitions and memories within a strict token budget:
```bash
ctx plan -task "Fix race condition in webhook dispatcher" -budget 4000 -render
```

### Search symbols and project memory
Run 7-stage hybrid search across code symbols, documentation, and stored decisions:
```bash
ctx retrieve -task "webhook retry policy" -top 10
```

### Record an architectural decision
Teach your agents a design rule so they follow it consistently:
```bash
ctx remember -kind decision -authority user \
  -content "Use the outbox pattern for Kafka publishing because database and message broker transactions are not atomic."
```

### Record a failure to prevent repeating mistakes
Save negative knowledge (bugs or failed attempts) so agents avoid the same trap:
```bash
ctx remember -kind failure -authority test \
  -content "Unbuffered channels in the hook dispatcher caused deadlocks under bursty traffic."
```

### Hand off work to another agent
Transfer active context cleanly when switching between models or IDEs:
```bash
ctx handoff -task "Implement Redis failover tests" -target claude -budget 4000
```

### Open the local visual dashboard
Launch the web interface to view memories, symbol graphs, and token savings:
```bash
ctx ui -port 8765
```

### Check repository statistics
View indexed symbol counts, stored memories, and cache efficiency:
```bash
ctx stats -repo .
```

### Switch storage backends
Migrate your data between SQLite and the zero-dependency FileStore without data loss:
```bash
# SQLite -> FileStore
ctx migrate -to file

# FileStore -> SQLite
ctx migrate -to sqlite
```

### Storage cleanup (Opt-in garbage collection)
Prune expired execution traces and cached plans:
```bash
# Dry run to see what would be pruned
ctx gc -repo . -keep-days 30 -dry-run

# Run cleanup
ctx gc -repo . -keep-days 30
```

---

## Agent Integrations

When you run `ctx setup`, ContextOS registers its MCP server and lifecycle hooks for all supported agents on your machine:

| Agent | Hooks Configuration | MCP Server Config | Rules / Context Guidance |
|---|---|---|---|
| **Claude Code** | `.claude/settings.local.json` | `.mcp.json` | — |
| **Cursor** | `.cursor/hooks.json` | `.cursor/mcp.json` | `.cursor/rules/contextos.mdc` |
| **Codex** | `.codex/hooks.json` | `.codex/config.toml` | — |
| **Gemini CLI** | `.gemini/settings.json` | `.gemini/settings.json` | — |
| **Antigravity** | `.agents/hooks.json` | `.agents/mcp_config.json` | `.agents/rules/contextos.md` |

To install for a specific agent only:
```bash
ctx install -agent cursor
```

All installations are idempotent — running setup multiple times will never duplicate entries or overwrite custom configurations.

---

## MCP Server

ContextOS includes a standard Model Context Protocol (MCP) server that runs locally over stdio:

```bash
# Run via CLI
ctx -repo /path/to/repo -mcp

# Or directly via the daemon
contextd -repo /path/to/repo -mcp
```

### Tools (13)

| Tool | What It Does |
|---|---|
| `context_plan` | Selects minimum-sufficient, cache-aware context under a token budget. |
| `context_remember` | Stores an architectural decision, constraint, failure, or fact. |
| `context_resume` | Recovers active repository state, recent decisions, and open work items. |
| `context_invalidate` | Invalidates an outdated memory when code changes supersede it. |
| `context_search` | Searches code symbols and memories using hybrid BM25 and semantic matching. |
| `context_compute_plan` | Recommends reasoning effort, token budgets, and verification tiers for a task. |
| `context_handoff` | Produces a structured handoff payload for switching between agents. |
| `context_route` | Recommends the optimal model based on task difficulty and budget. |
| `context_work_start` | Starts tracking a persistent engineering work item across sessions. |
| `context_session_start` | Initializes a tracked agent session tied to a work item. |
| `context_event` | Records tool invocations, milestones, and lifecycle events. |
| `context_stats` | Returns runtime statistics, cache hit rates, and token savings. |
| `context_trace` | Inspects the latest context allocation decisions and candidate scores. |

### Resources (5)

| Resource URI | Description |
|---|---|
| `context://repo/current` | Current git repository metadata, branch name, and commit hash. |
| `context://work-item/current` | Active engineering work item and task objectives. |
| `context://memory/relevant` | High-priority memories matching the current task. |
| `context://session/latest` | Most recent agent session state and history. |
| `context://trace/latest` | Latest context allocation trace and candidate scores. |

---

## How It Works Under the Hood

```
   Coding Agent (Claude / Cursor / Codex / Gemini / Antigravity)
                               │
                          MCP / Hooks
                               ▼
                        ┌─────────────┐
                        │  ContextOS  │
                        └──────┬──────┘
                               │
            ┌──────────────────┴──────────────────┐
            ▼                                     ▼
   7-Stage Hybrid Retrieval              6-Pass Budget Packing
   • Exact path matching                 • Multi-signal scoring
   • AST symbol resolution               • Rank fusion (RRF)
   • Entity primary key lookup           • Greedy knapsack packing
   • Lexical search (BM25)               • Singleton rescue guarantee
   • Locality-sensitive semantic hash    • Residual fill pass
   • Call graph expansion                • Stable prefix ordering
   • Rank fusion & deduplication                  │
            │                                     ▼
            └─────────────────────────► Cached Model Context
                                        (Stable Prefix + Tail)
```

### 1. The 7-Stage Hybrid Retrieval Pipeline
1. **Exact Path Match**: Direct lookups for specific files and line numbers.
2. **AST Symbol Resolution**: Pulls function signatures, structs, and interface definitions directly from the AST.
3. **Entity Lookup**: Direct hash-addressed retrieval for stored memory items.
4. **Lexical Search (BM25)**: Keyword search with length normalization to find relevant references.
5. **Semantic Hashing**: Fast, offline 64-bit cosine similarity matching without external embedding APIs.
6. **Graph Expansion**: Traverses callers, callees, and imports to capture connected code dependencies.
7. **Rank Fusion (RRF)**: Merges all ranking signals into a single score and deduplicates candidates across git trees.

### 2. The 6-Pass Context Packing Pipeline
1. **Candidate Scoring**: Evaluates relevance, authority, freshness, and confidence for every candidate.
2. **Reciprocal Rank Fusion**: Combines signals and applies hard filters (stale or unverified notes are discarded).
3. **Greedy Knapsack Selection**: Packs items in descending order of utility per token until the budget is filled.
4. **Singleton Rescue**: Ensures high-value single items that fit the budget are never crowded out.
5. **Residual Fill**: Uses any remaining token budget to include next-best supporting context.
6. **Stable Prefix Partitioning**: Organizes the prompt so that invariant knowledge (decisions, interfaces) sits at the front, maximizing LLM prompt-cache hits.

---

## Storage Options

ContextOS supports two persistence backends:

- **SQLite** (Default): Embedded database with Write-Ahead Logging (WAL) and Full-Text Search (FTS5). High performance for concurrent reads and writes.
- **FileStore**: Pure-Go standard library implementation with zero external dependencies and zero CGO. Reads and writes plain JSON files. Perfect for locked-down or containerized environments.

Default storage path is `~/.contextos/context.db` (SQLite) or `~/.contextos/data` (FileStore). Override via `-db PATH` or the `CONTEXTOS_DB` environment variable.

---

## Documentation

- [`docs/CONTEXT.md`](docs/CONTEXT.md) — Comprehensive architecture manual and design details.
- [`docs/MEMORY_HUB_GUIDE.md`](docs/MEMORY_HUB_GUIDE.md) — Practical guide to writing effective constraints, facts, and decisions.
- [`docs/EVALUATION.md`](docs/EVALUATION.md) — Empirical benchmarks, token savings measurements, and evaluation results.
- [`docs/RESEARCH.md`](docs/RESEARCH.md) — Mathematical formulations, formal bounds, and optimization proofs.
- [`docs/DREAM_RSI.md`](docs/DREAM_RSI.md) — Offline replay simulation and recursive self-improvement specification.

---

## CLI Reference

```
ctx init|index     -repo PATH                          Index repository code symbols
ctx remember       -repo PATH -kind K -content '...'   Save an engineering memory
ctx plan           -repo PATH -task '...' [-budget N]  Build an optimized context plan
ctx retrieve       -repo PATH -task '...' [-top N]     Run 7-stage candidate retrieval
ctx resume         -repo PATH                          Recover active branch & work state
ctx handoff        -repo PATH -task '...' -target M    Prepare context for another agent
ctx invalidate     -repo PATH -id MEMORY_ID            Invalidate an obsolete memory
ctx gc             -repo PATH [-keep-days N]           Garbage collect expired traces
ctx migrate        -to file|sqlite                     Transfer data between storage engines
ctx work           -repo PATH -title "..."             Start a tracked work item
ctx session        -repo PATH -agent NAME              Start an agent session
ctx event          -repo PATH -event TYPE -payload J   Record a lifecycle event
ctx stats          -repo PATH                          Show memory and cache statistics
ctx route          -task "..." -budget N               Recommend a model and effort level
ctx ui             -repo PATH [-port 8765]             Start the local web UI dashboard
ctx bench          [correctness|retrieval]             Run benchmark suites
ctx audit          -repo PATH                          Audit admitted vs excluded files
ctx install        -repo PATH -agent NAME              Install hooks and MCP for an agent
ctx setup          -repo PATH                          Index repo and configure all agents
ctx completion     [bash|zsh]                          Generate shell completion scripts
```

---

## License

Licensed under the **Business Source License 1.1 (BSL 1.1)**:
- **Free** for personal, academic, and local developer use with AI coding agents.
- **Commercial & Hosted / SaaS** use requires a commercial license from the authors.
- Converts to **Apache 2.0** on January 1, 2030.

See [`LICENSE`](LICENSE) for full terms.
