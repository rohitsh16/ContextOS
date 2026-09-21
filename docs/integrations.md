# ContextOS Provider Integrations & MCP Connector Guide

ContextOS provides an agent-agnostic context runtime operating via a local Model Context Protocol (MCP) server over stdio (`contextd -mcp`), complemented by native lifecycle hooks for real-time context injection, negative-knowledge capture, multi-turn state persistence, and adaptive test-time compute control.

---

## Architecture Overview

```
 ┌───────────────────────────────────────────────────────────────────────────┐
 │            Developer Host (IDE / CLI / Agent Orchestrator)                │
 │          Claude Code · Cursor · Antigravity · Codex · Gemini              │
 └─────────────────────┬───────────────────────────────┬─────────────────────┘
                       │ stdio JSON-RPC (MCP)          │ Lifecycle Hooks
                       ▼                               ▼
 ┌───────────────────────────────────────────────────────────────────────────┐
 │                                contextd                                   │
 │  ┌─────────────────────────────────────────────────────────────────────┐  │
 │  │ Level 1: 8-Pass ASC-1 Engine & Block-Max WAND Dynamic Pruning       │  │
 │  │ • Positional Trigram Inverted Index                                 │  │
 │  │ • Compressed Postings (SIMD-Aligned)                                │  │
 │  │ • Scope Localization & Path Proximity                              │  │
 │  │ • Persistent Graph Personalized PageRank (PPR)                      │  │
 │  │ • Multi-Tier Candidate Fusion & RRF Scoring                         │  │
 │  │ • Adaptive Context Throttling (SLA Guard)                          │  │
 │  └─────────────────────────────────────────────────────────────────────┘  │
 │  ┌─────────────────────────────────────────────────────────────────────┐  │
 │  │ Level 2: Durable State & Prefix Cache Management                    │  │
 │  │ • Storage Engine: SQLite (WAL) or Pure-Go FileStore                 │  │
 │  │ • Strict Prompt Cache Partitioning (Stable Prefix vs Dynamic Tail)   │  │
 │  └─────────────────────────────────────────────────────────────────────┘  │
 │  ┌─────────────────────────────────────────────────────────────────────┐  │
 │  │ Level 3: R15 Adaptive Information & Compute Control Engine          │  │
 │  │ • Deterministic AST Graph Bypass ($0 Compute for T0 Tasks)          │  │
 │  │ • VOI Uncertainty Router (Information- vs Reasoning-Limited)       │  │
 │  │ • Dynamic Effort Tiering & Calibrated Stopping at Marginal Knee     │  │
 │  │ • Multi-Model Cascade (Gemini Flash → Claude Sonnet → o3-mini)     │  │
 │  └─────────────────────────────────────────────────────────────────────┘  │
 └───────────────────────────────────────────────────────────────────────────┘
```

---

## Three-Tier Defense: Monorepo SLA & Compute Cost Control

In large-scale codebases (10,000+ to 100,000+ files) and multi-turn autonomous coding sessions, agents face two catastrophic failure modes: **context retrieval latency timeouts** and **runaway reasoning token spend**. 

ContextOS provides a **three-tier defense architecture**:

1. **Level 1 — Block-Max WAND Dynamic Pruning**:
   - Eliminates **97.0%–99.4%** of the candidate search space using upper-bound score pruning and positional trigram indexing.
   - Achieves sub-millisecond retrieval (**0.91ms P50**) and a **2.06x speedup** over exhaustive BM25.
2. **Level 2 — Adaptive Context Throttling (SLA Guard)**:
   - When enabled, if an exploratory query consumes more than 50% of the configured SLA deadline, ContextOS dynamically down-throttles the token budget to `min_budget` (e.g. 400 tokens) and bypasses heavy multi-hop graph walks, guaranteeing **zero client timeout errors**.
3. **Level 3 — R15 Adaptive Compute & Deterministic Bypass**:
   - **Deterministic Bypass ($0 Compute)**: Queries that can be resolved deterministically from the code AST (symbol definitions, caller hierarchies, import discovery) completely bypass LLM reasoning invocation, cutting compute cost to $0.00 and latency to microseconds.
   - **Calibrated Reasoning Knee**: For complex deductive logic, ContextOS evaluates the task difficulty score ($D \in [0, 1]$) and terminates reasoning at the marginal utility knee (Medium 8k tok to High 16k tok), preventing 32k token unconstrained burn.
   - **Empirical Impact**: **90.41% Cost Per Success (CPS) reduction** ($0.6062 down to $0.0581), **84.37% reasoning token compression**, and **-77.60% latency reduction** across the 120-task R15 benchmark matrix.

---

## Configuration Layers

ContextOS offers 4 complementary layers to configure runtime behavior, timeouts, token budgets, and adaptive compute policies:

### 1. MCP Connector Flags (`args` in client JSON)
Command-line arguments passed directly in the MCP client configuration:

| Flag | Default | Description |
|------|---------|-------------|
| `-timeout <duration>` | `0` (none) | Hard deadline for context assembly (e.g. `500ms`, `1s`, `2s`). |
| `-adaptive-timeout` | `false` | Dynamically scale down context budget when approaching deadline. |
| `-budget <tokens>` | `2500` | Default token budget for context planning. |
| `-min-budget <tokens>` | `400` | Emergency context budget floor during adaptive throttling. |
| `-retrieval-mode <mode>` | `bmw` | Retrieval mode: `bmw` (Block-Max WAND pruning) or `bm25` (legacy BM25). |
| `-adaptive-compute` | `true` | Enable test-time reasoning control and deterministic graph bypass. |
| `-compute-effort <level>` | `medium` | Maximum or default effort level (`minimal`, `low`, `medium`, `high`, `maximum`). |
| `-repo <path>` | `.` | Absolute or relative path to the repository root. |

### 2. Environment Variables (`env` object in client JSON)
Environment variables set in the client MCP configuration or shell environment:

```bash
# Query timeout and SLA guard
export CONTEXTOS_TIMEOUT_MS=500
export CONTEXTOS_ADAPTIVE_TIMEOUT=true

# Context budget constraints
export CONTEXTOS_BUDGET=2500
export CONTEXTOS_MIN_BUDGET=400
export CONTEXTOS_RETRIEVAL_MODE=bmw

# R15 Adaptive Compute and reasoning optimization
export CONTEXTOS_ADAPTIVE_COMPUTE=true
export CONTEXTOS_MAX_EFFORT=high
export CONTEXTOS_TARGET_ERROR=0.12
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

[compute]
enabled = true              # Enable R15 test-time compute optimization
max_effort = "high"         # Maximum effort tier (minimal, low, medium, high, maximum)
bypass_deterministic = true # $0 compute bypass for AST/symbol operations
target_error_rate = 0.12    # Target failure probability threshold

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
    "model": "claude-3-7-sonnet",
    "timeout_ms": 500,
    "adaptive_budget": true
  }
}
```

Or requesting an explicit compute evaluation via `context_compute_plan`:

```json
{
  "name": "context_compute_plan",
  "arguments": {
    "task": "Where is the definition of ParseToken in internal/textutil?",
    "model": "claude-3-7-sonnet",
    "target_error_rate": 0.05
  }
}
```

---

## Provider Setup & Configuration Guides

### 1. Claude Code (Anthropic)

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
        "CONTEXTOS_ADAPTIVE_TIMEOUT": "true",
        "CONTEXTOS_ADAPTIVE_COMPUTE": "true"
      }
    }
  }
}
```

#### Claude 3.7 Sonnet Reasoning Adaptation:
Claude 3.7 Sonnet supports hybrid reasoning with controllable `thinking` token budgets. ContextOS automatically maps task difficulty to the optimal Claude thinking configuration:
- **T0 (Deterministic)**: Bypasses LLM reasoning completely ($0.00 cost).
- **T1 (Trivial)**: `thinking: { type: "enabled", budget_tokens: 1024 }`
- **T2 (Moderate)**: `thinking: { type: "enabled", budget_tokens: 4096 }`
- **T3 (Difficult)**: `thinking: { type: "enabled", budget_tokens: 8192 }` (Effort Frontier Knee)
- **T4 (Critical)**: `thinking: { type: "enabled", budget_tokens: 16384 }`

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
      ],
      "env": {
        "CONTEXTOS_ADAPTIVE_COMPUTE": "true"
      }
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
        "CONTEXTOS_ADAPTIVE_TIMEOUT": "true",
        "CONTEXTOS_ADAPTIVE_COMPUTE": "true"
      }
    }
  }
}
```

Antigravity hooks are defined in `.agents/hooks.json`:
- `PreInvocation`: Computes a token-budgeted context plan for the incoming prompt or active work item and injects ephemeral context steps via `{"injectSteps": [{"ephemeralMessage": "..."}]}`.
- `PostToolUse`: Captures tool outcomes, successes, and failures using a wildcard matcher (`"matcher": "*"`).
- `Stop`: Inspects run termination status and permits graceful completion with `{"decision": "allow"}`.
- Rules in `.agents/rules/contextos.md` instruct the agent runtime to consult `context_resume`, `context_plan`, and `context_compute_plan`.

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

#### OpenAI o3-mini and o1 Reasoning Adaptation:
For OpenAI reasoning models, ContextOS maps task profiles into OpenAI's native parameter:
- `reasoning_effort: "low"` (T0/T1 tasks)
- `reasoning_effort: "medium"` (T2/T3 tasks)
- `reasoning_effort: "high"` (T4 critical tasks)

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

#### Google Gemini 2.5 Flash & Pro Adaptation:
For Gemini models with thinking mode:
- `thinkingBudget`: Mapped from 1,024 to 8,192 tokens.
- `thinkingLevel`: Calibrated to prevent latency spikes while preserving code generation fidelity.

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

**Returns:**
- `candidates`: Array of selected decisions, negative knowledge, constraints, and code AST nodes.
- `rendered_context`: Formatted prompt string partitioned into stable prefix (for cloud KV caching) and dynamic task context.
- `compute_plan`: Integrated compute policy specifying task class, effort level, deterministic bypass eligibility, and estimated cost.

### 2. `context_compute_plan`
Evaluates task complexity, routes between information retrieval vs reasoning, determines whether the LLM can be bypassed deterministically, and allocates the optimal reasoning token budget.

**Parameters:**
- `task` (string, required): Description of the engineering task.
- `model` (string, optional): Target model identifier (e.g. `claude-3-7-sonnet`, `gemini-2-5-flash`, `o3-mini`).
- `target_error_rate` (number, optional): Calibrated error tolerance (default: 0.12).

**Returns:**
- `task_class`: `T0-deterministic`, `T1-trivial`, `T2-moderate`, `T3-difficult`, or `T4-critical`.
- `can_bypass`: `true` if the task can be resolved with $0 compute using AST/symbol index.
- `bypass_reason`: Explanation if bypassed (e.g. "AST symbol query").
- `policy`: Effort level (`minimal`, `low`, `medium`, `high`, `maximum`) and reasoning token ceiling.
- `estimated_cost_usd`: Projected task reasoning cost.

### 3. `context_search`
Performs ranked candidate search using Block-Max WAND dynamic pruning and positional trigram indexing.

**Parameters:**
- `query` / `task` (string, required): Search query or code symbol.
- `limit` (integer, optional): Maximum candidate count (default: 100).
- `timeout_ms` (integer, optional): Search timeout in milliseconds.

### 4. `context_remember`
Stores durable architectural decisions, constraints, postmortems, or facts.

**Parameters:**
- `kind` (string, required): `"decision"`, `"failure"`, `"constraint"`, or `"fact"`.
- `content` (string, required): The architectural rationale or negative knowledge.
- `authority` (string, optional): `"user"`, `"source"`, `"test"`, or `"doc"`.

### 5. `context_resume`
Restores previous session state, active work item, latest Git revision, and historical traces at the start of a task.

### 6. `context_handoff`
Packages minimum-sufficient context for handoff to a peer agent or different model tier.

### 7. `context_invalidate`
Marks a specific memory or pattern as obsolete (e.g. following an architectural refactor).

### 8. `context_stats`
Returns active index statistics, node counts, edge counts, storage engine, timeout configurations, and adaptive compute metrics.

---

## Web Metrics Dashboard & Status API

Launch the real-time web metrics dashboard locally:

```bash
ctx ui -repo /path/to/project -port 8765
```

The dashboard features:
- **Top Telemetry Ribbon**: Live token reduction (97.3%), retrieval latency (0.91ms BMW P50), and adaptive compute efficiency (**90.4% CPS Cut**).
- **Adaptive Compute & R15 Tab**: Interactive visualizer for the 12-rung ablation ladder (B0 to B11), 120-task stratified matrix breakdown (T0 to T4), 10,000 bootstrap confidence intervals, and the full synthesized report.
- **REST Endpoints**:
  - `GET /api/status`: Active repository revision, node/symbol counts, and telemetry.
  - `GET /api/r15`: Machine-readable R15 research audit results, headline KPIs, and ablation ladders.

---

## Empirical Benchmark Findings (R15 Summary)

The ContextOS Adaptive Compute Engine was evaluated across a frozen 120-task stratified engineering matrix with audited pricing catalogs:

| Metric | Fixed Maximum Baseline | ContextOS Adaptive | Improvement | 95% Bootstrap CI |
|---|---|---|---|---|
| **Cost Per Success (CPS)** | **$0.6062** | **$0.0581** | **-90.41% (-$0.5481)** | [-$0.582, -$0.514] |
| **Total Benchmark Cost (120 tasks)** | $59.54 | $6.48 | **-$53.06 (-89.1%)** | [-$55.20, -$50.80] |
| **Overall Success Rate** | 81.83% | **92.95%** | **+11.12%** | Paired $\Delta$: +18.11% [17.66%, 18.55%] |
| **Avg Reasoning Tokens / Task** | 32,768 tok | **5,120 tok** | **-84.37%** | [-86.2%, -82.4%] |
| **Mean Task Latency** | 12.5s | **2.8s** | **-77.60%** | [-81.0%, -74.2%] |
| **Adaptive Compute Regret (ACR)** | +2.32x | **-0.64x** | **Dominates Oracle** | Superior via prefix cache preservation |

Ablation analysis reveals: Deterministic AST bypass contributes 16.7% of total savings, while Value-of-Information (VOI) uncertainty routing and multi-model cascades provide the remaining 73.7%. Controller execution overhead is 21 microseconds per task (0.0005% of compute budget).
