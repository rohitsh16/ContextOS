# ContextOS Provider Integrations & MCP Connector Guide

ContextOS provides an agent-agnostic context runtime operating via a local Model Context Protocol (MCP) server over stdio (`contextd -mcp`), complemented by native lifecycle hooks for real-time context injection, negative-knowledge capture, and multi-turn state persistence.

---

## Architecture Overview

```
 ┌───────────────────────────────────────────────────────────┐
 │   Developer Host (IDE / CLI / Agent Orchestrator)         │
 │   Claude Code · Cursor · Antigravity · Codex · Gemini     │
 └─────────────┬───────────────────────────────┬─────────────┘
               │ stdio JSON-RPC (MCP)          │ Lifecycle Hooks
               ▼                               ▼
 ┌───────────────────────────────────────────────────────────┐
 │                       contextd                            │
 │  ┌─────────────────────────────────────────────────────┐  │
 │  │ 8-Pass ASC-1 Engine & Block-Max WAND Pruning        │  │
 │  │ • Positional Trigram Inverted Index                 │  │
 │  │ • Compressed Postings (SIMD-Aligned)                │  │
 │  │ • Scope Localization & Path Proximity              │  │
 │  │ • Persistent Graph Personalized PageRank (PPR)      │  │
 │  │ • Multi-Tier Candidate Fusion & RRF Scoring         │  │
 │  │ • Adaptive Context Throttling (SLA Guard)          │  │
 │  └─────────────────────────────────────────────────────┘  │
 │  ┌─────────────────────────────────────────────────────┐  │
 │  │ Storage Engine: SQLite (WAL) or Pure-Go FileStore   │  │
 │  └─────────────────────────────────────────────────────┘  │
 └───────────────────────────────────────────────────────────┘
```

---

## Large Monorepo Optimization & Timeout Protection

In massive monorepos (10,000+ to 100,000+ files), exhaustive AST graph traversals or broad full-scan retrievals can risk client-side timeouts (e.g. 5–10 second tool timeouts in IDEs).

ContextOS provides a **two-tier defense against timeouts**:
1. **Block-Max WAND Dynamic Pruning**: Eliminates **97.0%–99.4%** of the search space, achieving sub-millisecond retrieval (**0.91ms P50**) and a 2.06x speedup over exhaustive retrieval.
2. **Adaptive Context Throttling**: When enabled, if a query consumes more than 50% of the configured timeout deadline, ContextOS dynamically down-throttles the token budget to `min_budget` (e.g. 400 tokens) and bypasses heavy graph walks, guaranteeing a valid, decision-sufficient context response within the client SLA with **0 timeout errors**.

---

## Configuration Layers

ContextOS offers 4 complementary ways to configure runtime behavior, timeouts, and token budgets:

### 1. MCP Connector Flags (`args` in client JSON)
Command-line arguments passed directly in the MCP client configuration:

| Flag | Default | Description |
|------|---------|-------------|
| `-timeout <duration>` | `0` (none) | Hard deadline for context assembly (e.g. `500ms`, `1s`, `2s`). |
| `-adaptive-timeout` | `false` | Dynamically scale down context budget when approaching deadline. |
| `-budget <tokens>` | `2500` | Default token budget for context planning. |
| `-min-budget <tokens>` | `400` | Emergency context budget floor during adaptive throttling. |
| `-retrieval-mode <mode>` | `bmw` | Retrieval mode: `bmw` (Block-Max WAND pruning) or `bm25` (legacy BM25). |
| `-repo <path>` | `.` | Absolute or relative path to the repository root. |

### 2. Environment Variables (`env` object in client JSON)
Environment variables set in the client MCP configuration or shell environment:

```bash
# Set query timeout to 500 milliseconds
export CONTEXTOS_TIMEOUT_MS=500

# Enable adaptive context down-throttling on soft deadline
export CONTEXTOS_ADAPTIVE_TIMEOUT=true

# Default planning token budget
export CONTEXTOS_BUDGET=2500

# Minimum token budget under deadline pressure
export CONTEXTOS_MIN_BUDGET=400

# Active retrieval engine: bmw or bm25
export CONTEXTOS_RETRIEVAL_MODE=bmw
```

### 3. Repository Configuration (`.contextos/config.toml`)
Project-level configuration file committed in the repository root:

```toml
[retrieval]
mode = "bmw"               # "bmw" (Block-Max WAND) or "bm25"
timeout_ms = 500           # Query timeout SLA in milliseconds
adaptive_timeout = true    # Dynamically down-throttle to prevent timeouts
default_budget = 2500      # Target token budget
min_budget_tokens = 400    # Floor token budget for large monorepos

[storage]
engine = "sqlite"          # "sqlite" or "file"
auto_prune = true
```

### 4. Per-Call MCP Tool Arguments
Dynamic overrides passed by the agent runtime on individual MCP tool calls:

```json
{
  "name": "context_plan",
  "arguments": {
    "task": "Fix Kafka consumer race condition on partition rebalance",
    "budget": 2000,
    "timeout_ms": 500,
    "adaptive_budget": true
  }
}
```

---

## Provider Setup & Configuration Guides

### 1. Claude Code

Run automatic setup:
```bash
ctx setup -agent claude
```

Or manually configure `.mcp.json` in your repository root:
```json
{
  "mcpServers": {
    "contextos": {
      "type": "stdio",
      "command": "contextd",
      "args": [
        "-repo", ".",
        "-mcp",
        "-timeout", "500ms",
        "-adaptive-timeout",
        "-min-budget", "400"
      ],
      "env": {
        "CONTEXTOS_TIMEOUT_MS": "500",
        "CONTEXTOS_ADAPTIVE_TIMEOUT": "true"
      }
    }
  }
}
```

Lifecycle hooks are configured in `.claude/settings.local.json`:
- `SessionStart`: Ingests initial workspace revision and recent session state.
- `UserPromptSubmit`: Intercepts prompt submissions to assemble token-budgeted context via `hookSpecificOutput`.

---

### 2. Cursor

Run automatic setup:
```bash
ctx setup -agent cursor
```

Or manually configure `.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "contextos": {
      "command": "contextd",
      "args": [
        "-repo", ".",
        "-mcp",
        "-timeout", "500ms",
        "-adaptive-timeout",
        "-min-budget", "400"
      ]
    }
  }
}
```

Cursor hooks are configured in `.cursor/hooks.json`:
- `beforeSubmitPrompt`: Automatically injects active minimum-sufficient context into `additional_context` with `{"continue": true}` so prompts execute with full architectural state.
- `postToolUseFailure`: Captures error output, stack traces, and compiler diagnostics directly into ContextOS negative memory (`kind: failure`).
- Rules in `.cursor/rules/contextos.mdc` guide Cursor models to call `context_resume` at the start of tasks and persist decisions via `context_remember`.

---

### 3. Google Antigravity & AGY

Run automatic setup:
```bash
ctx setup -agent antigravity
```

Or manually configure `.agents/mcp_config.json`:
```json
{
  "mcpServers": {
    "contextos": {
      "command": "contextd",
      "args": [
        "-repo", ".",
        "-mcp",
        "-timeout", "500ms",
        "-adaptive-timeout",
        "-min-budget", "400"
      ],
      "env": {
        "CONTEXTOS_TIMEOUT_MS": "500",
        "CONTEXTOS_ADAPTIVE_TIMEOUT": "true"
      }
    }
  }
}
```

Antigravity hooks are defined in `.agents/hooks.json`:
- `PreInvocation`: Computes a token-budgeted context plan for the incoming prompt or active work item and injects ephemeral context steps via `{"injectSteps": [{"ephemeralMessage": "..."}]}`.
- `PostToolUse`: Captures tool outcomes, successes, and failures using a wildcard matcher (`"matcher": "*"`).
- `Stop`: Inspects run termination status and permits graceful completion with `{"decision": "allow"}`.
- Rules in `.agents/rules/contextos.md` instruct the agent runtime to consult `context_resume` and `context_plan`.

---

### 4. Claude Desktop

In `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "contextos": {
      "command": "/usr/local/bin/contextd",
      "args": [
        "-repo", "/absolute/path/to/repository",
        "-mcp",
        "-timeout", "1s",
        "-adaptive-timeout",
        "-min-budget", "400"
      ]
    }
  }
}
```

---

### 5. OpenAI Codex

Run automatic setup:
```bash
ctx setup -agent codex
```

Or configure `.codex/config.toml`:
```toml
[mcp_servers.contextos]
command = "contextd"
args = ["-repo", ".", "-mcp", "-timeout", "500ms", "-adaptive-timeout"]
```

Hook output is managed conservatively in `.codex/hooks.json` to remain compatible across Codex releases.

---

### 6. Gemini CLI

Run automatic setup:
```bash
ctx setup -agent gemini
```

Or configure `.gemini/settings.json`:
```json
{
  "mcpServers": {
    "contextos": {
      "command": "contextd",
      "args": [
        "-repo", ".",
        "-mcp",
        "-timeout", "500ms",
        "-adaptive-timeout"
      ]
    }
  }
}
```

---

### 7. Windsurf / Generic MCP Clients

Any client supporting the Model Context Protocol stdio transport can connect to ContextOS:

```json
{
  "mcpServers": {
    "contextos": {
      "command": "contextd",
      "args": [
        "-repo", "/path/to/project",
        "-mcp",
        "-timeout", "500ms",
        "-adaptive-timeout",
        "-min-budget", "400"
      ]
    }
  }
}
```

---

## MCP Tools Reference

ContextOS registers the following MCP tools for agent runtimes:

### 1. `context_plan`
Assembles minimum-sufficient context for a specified task objective within a token budget.

**Parameters:**
- `task` (string, required): Description of the goal, feature, or bug fix.
- `budget` (integer, optional): Token budget limit (default: 2500).
- `model` (string, optional): Target LLM model for KV-cache optimization.
- `timeout_ms` (integer, optional): Per-query execution timeout in milliseconds.
- `adaptive_budget` (boolean, optional): Whether to dynamically lower budget under deadline pressure.

### 2. `context_search`
Performs ranked candidate search using Block-Max WAND dynamic pruning and positional trigram indexing.

**Parameters:**
- `query` / `task` (string, required): Search query or code symbol.
- `limit` (integer, optional): Maximum candidate count (default: 100).
- `timeout_ms` (integer, optional): Search timeout in milliseconds.

### 3. `context_remember`
Stores durable architectural decisions, constraints, postmortems, or facts.

**Parameters:**
- `kind` (string, required): `"decision"`, `"failure"`, `"constraint"`, or `"fact"`.
- `content` (string, required): The architectural rationale or negative knowledge.
- `authority` (string, optional): `"user"`, `"source"`, `"test"`, or `"doc"`.

### 4. `context_resume`
Restores previous session state, active work item, latest Git revision, and historical traces at the start of a task.

### 5. `context_handoff`
Packages minimum-sufficient context for handoff to a peer agent or different model tier.

### 6. `context_invalidate`
Marks a specific memory or pattern as obsolete (e.g. following an architectural refactor).

### 7. `context_stats`
Returns active index statistics, node counts, edge counts, storage engine, and timeout configurations.

---

## Portable Fallback

All providers can execute against the identical standalone daemon:

```bash
contextd -repo /path/to/repository -mcp -timeout 500ms -adaptive-timeout
```

The underlying context state format is completely provider-neutral and portable across all AI engineering agents.
