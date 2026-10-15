# ContextOS — Architectural & Operational Context Manual

ContextOS is a local-first, model-neutral context runtime designed for software engineering workflows using AI coding agents (Claude Code, Cursor, Codex, Gemini CLI, Antigravity). It eliminates context loss across laptop restarts, agent switches, repository revisions, and long-running multi-turn tasks while systematically cutting input token spend, API cost, and inference latency.

---

## 1. System Architecture

```
                                  +-----------------------+
                                  |   AI Coding Agents    |
                                  | Claude/Cursor/Codex/..|
                                  | Gemini / Antigravity  |
                                  +-----------+-----------+
                                              |
                   +--------------------------+--------------------------+
                   | (Hook Interception / CLI Execution / MCP JSON-RPC)  |
                   v                                                     v
          +------------------+                                +--------------------+
          |  ctx CLI / UI    |                                |  MCP Server Engine |
          | (Terminal / Web) |                                | (contextd stdio)   |
          +--------+---------+                                +---------+----------+
                   |                                                    |
                   +--------------------------+-------------------------+
                                              |
                                              v
                              +-------------------------------+
                              | 7-Stage Hybrid Retrieval      |
                              | - Exact Path & Symbol Match   |
                              | - Entity Lookup & Ingestion   |
                              | - Lexical BM25 & Block-Max    |
                              | - Locality-Sensitive HashSem  |
                              | - Graph Centrality Expansion  |
                              | - Reciprocal Rank Fusion      |
                              +---------------+---------------+
                                              |
                                              v
                              +-------------------------------+
                              |    ASC-1 Context Allocator    |
                              |  - Multiplicative Gating      |
                              |  - Marginal Density Knapsack  |
                              |  - (1-1/e) Singleton Rescue   |
                              |  - Residual Fill Packing      |
                              |  - KV-Cache Stable Prefixing  |
                              +---------------+---------------+
                                              |
                                              v
                              +-------------------------------+
                              | Test-Time Compute Optimizer   |
                              |  - Minimum-Sufficient Compute |
                              |  - AST Deterministic Bypass   |
                              |  - VOI Uncertainty Classifier |
                              |  - Calibrated Effort Tiers    |
                              +---------------+---------------+
                                              |
                     +------------------------+------------------------+
                     v                                                 v
          +-----------------------+                         +----------------------+
          |   Git State Engine    |                         | Pluggable Store & Svc|
          |  - Revision & Branch  |                         |  - SQLite (WAL+FTS5) |
          |  - Diff & Invalidation|                         |  - FileStore(Pure Go)|
          |  - AST Code Graph     |                         |  - Multi-Tenant SaaS |
          +-----------------------+                         +----------------------+
```

### Core Packages & Directory Map

| Package / Directory | Primary Responsibility | Key Files |
|---|---|---|
| [`internal/retrieval`](file:///Users/rohitshukla/Desktop/ContextOS/internal/retrieval) | 7-stage authoritative hybrid retrieval pipeline, Block-Max WAND dynamic pruning, quotient-space path canonicalization | [`engine.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/retrieval/engine.go), [`retrieval_test.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/retrieval/retrieval_test.go) |
| [`internal/allocator`](file:///Users/rohitshukla/Desktop/ContextOS/internal/allocator) | 6-pass budget-constrained context optimization algorithm (ASC-1) | [`allocator.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/allocator/allocator.go), [`allocator_test.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/allocator/allocator_test.go) |
| [`internal/compute`](file:///Users/rohitshukla/Desktop/ContextOS/internal/compute) | Test-time reasoning control, minimum-sufficient compute ($R^*$), Value of Information (VOI) routing, deterministic AST bypass | [`optimizer.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/compute/optimizer.go), [`compute_test.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/compute/compute_test.go) |
| [`internal/saas`](file:///Users/rohitshukla/Desktop/ContextOS/internal/saas) | Multi-tenant SaaS proxy (`contextos-proxy`), token metering, rate limiting, Stripe-compatible billing statements | [`proxy.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/saas/proxy.go), [`metering.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/saas/metering.go) |
| [`internal/ui`](file:///Users/rohitshukla/Desktop/ContextOS/internal/ui) | Local web UI dashboard (`ctx ui`), real-time telemetry, memory hub, allocation playground | [`server.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/ui/server.go) |
| [`internal/indexer`](file:///Users/rohitshukla/Desktop/ContextOS/internal/indexer) | AST parsing, incremental Git change detection, symbol extraction, call graph construction | [`indexer.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/indexer/indexer.go), [`incremental.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/indexer/incremental.go) |
| [`internal/providers`](file:///Users/rohitshukla/Desktop/ContextOS/internal/providers) | Model provider abstractions and asymmetric KV-cache pricing models (Anthropic, OpenAI, Google, DeepSeek, Local) | [`provider.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/providers/provider.go) |
| [`internal/model`](file:///Users/rohitshukla/Desktop/ContextOS/internal/model) | Canonical domain entities, memory structures, candidate envelopes, context plans | [`types.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/model/types.go) |
| [`internal/store`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store) | Pluggable persistence layer: SQLiteStore (WAL+FTS5) and FileStore (Pure-Go stdlib), bidirectional migration (`ctx migrate`), and opt-in pruning | [`store.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/store.go), [`sqlite_store.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/sqlite_store.go), [`file_store.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/file_store.go), [`migrate.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/migrate.go) |
| [`internal/db`](file:///Users/rohitshukla/Desktop/ContextOS/internal/db) | Low-level SQLite driver with build-tag isolation (`//go:build cgo` and `sqlite_nocgo.go` stubs) | [`sqlite.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/db/sqlite.go), [`sqlite_nocgo.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/db/sqlite_nocgo.go) |
| [`internal/textutil`](file:///Users/rohitshukla/Desktop/ContextOS/internal/textutil) | Robertson-Sparck Jones BM25, Locality-Sensitive HashSemantic, token estimation | [`textutil.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/textutil/textutil.go), [`textutil_test.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/textutil/textutil_test.go) |
| [`internal/gitidx`](file:///Users/rohitshukla/Desktop/ContextOS/internal/gitidx) | Git working tree inspection, HEAD commit hash resolution, branch detection | [`scanner.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/gitidx/scanner.go) |
| [`internal/mcp`](file:///Users/rohitshukla/Desktop/ContextOS/internal/mcp) | Model Context Protocol JSON-RPC server (13 tools, 5 resources) | [`server.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/mcp/server.go) |
| [`internal/hook`](file:///Users/rohitshukla/Desktop/ContextOS/internal/hook) | Hook ingestion, normalization, and context injection handlers for Claude, Cursor, Codex, Gemini, Antigravity | [`handler.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/hook/handler.go) |
| [`internal/integrations`](file:///Users/rohitshukla/Desktop/ContextOS/internal/integrations) | Automated installer for hooks, MCP configuration, and agent guidelines | [`integrations.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/integrations/integrations.go) |
| [`internal/bench`](file:///Users/rohitshukla/Desktop/ContextOS/internal/bench) | Benchmarking engine (`ctx bench`, `ctxbench`) for longitudinal, ablation, and real-world evaluation | [`bench.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/bench/bench.go) |
| [`internal/verification`](file:///Users/rohitshukla/Desktop/ContextOS/internal/verification) | System verification, invariant auditing, and integrity validation (`ctx audit`) | [`audit.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/verification/audit.go) |
| [`cmd/ctx`](file:///Users/rohitshukla/Desktop/ContextOS/cmd/ctx) | User and agent command line interface binary entry point | [`main.go`](file:///Users/rohitshukla/Desktop/ContextOS/cmd/ctx/main.go) |
| [`cmd/contextd`](file:///Users/rohitshukla/Desktop/ContextOS/cmd/contextd) | Background context daemon & MCP standard-input/output engine | [`main.go`](file:///Users/rohitshukla/Desktop/ContextOS/cmd/contextd/main.go) |

---

## 2. Memory Model & Epistemological Taxonomy

Rather than unstructured text lumps or naive vector dumps, ContextOS classifies all memories into strongly typed knowledge entities:

### Memory Kinds (`model.Memory.Kind`)

1. **`decision`** (`kindBoost = 1.00`): Architectural, structural, or dependency choices ("Use outbox pattern for Kafka transactions").
2. **`failure`** (`kindBoost = 0.98`): Negative knowledge postmortems ("Direct DB commit before Kafka publish caused lost events").
3. **`constraint`** (`kindBoost = 0.96`): Hard project invariants ("Must retain Go 1.23 standard library compatibility").
4. **`state`** (`kindBoost = 0.88`): Active working progress ("Migrated orders table; pending index backfill").
5. **`observation`** (`kindBoost = 0.76`): Noted runtime behavior or test output.
6. **`code`** (`kindBoost = 0.74`): Critical symbol signatures, interface contracts, and AST slices.
7. **`fact`** (`kindBoost = 0.65`): General domain facts and documentation references.

### Authority Hierarchy (`model.Memory.Authority`)

To guarantee that code agents do not hallucinate over unverified notes or prioritize low-confidence inferences over user instructions, ContextOS enforces strict authority stratification:

| Authority Level | Numeric Weight | Selection Rule |
|---|:---:|---|
| **`user` / `explicit`** | **1.00** | Explicit operator directives; absolute highest precedence. |
| **`test`** | **0.99** | Mechanically validated by passing unit/integration test suites. |
| **`source`** | **0.97** | Mechanically extracted from active repository source code / AST. |
| **`commit`** | **0.94** | Extracted from committed Git log messages and PR metadata. |
| **`doc`** | **0.86** | Project markdown documentation, specs, and README files. |
| **`inference`** | **0.55** | Model-generated speculation. **Automatically rejected (`auth < 0.60`)** |
| *default / unset* | **0.45** | Unattributed text blobs. **Automatically rejected (`auth < 0.60`)** |

### Confidence Gating (`model.Memory.Confidence`)

The `Confidence` field ($c \in [0.0, 1.0]$) gates the composite score multiplicatively:

$$
W_{\text{conf}}(c) = \begin{cases}
1.00 & \text{if } c \le 0 \quad (\text{backward compatibility / unstated}) \\
\max(0.10, \, c) & \text{if } c > 0
\end{cases}
$$

This dampens uncertain memories without completely blinding the system to tentative discoveries.

### Temporal Validity & Git Invalidation

Every engineering memory is tagged with `ValidFromRevision` and `InvalidatedAtRevision`.
- If `InvalidatedAtRevision` matches or precedes the current Git commit, `hardStale = true` $\implies \text{Density} = -\infty$ (immediate drop).
- If `ValidFromRevision != CurrentRevision`, a soft freshness penalty ($25\%$) is assessed, allowing historic decisions to compete until superseded.

---

## 3. The 7-Stage Authoritative Hybrid Retrieval Pipeline

Before context candidates enter the optimization allocator, ContextOS executes an authoritative 7-stage retrieval pipeline designed to prevent hallucination, eliminate duplicate worktree contamination, and guarantee bounded latency ($O(T(|V_q| + |E_q|) + k)$):

```
Query q + Repo State S
        │
        ├─► [ Stage 1: Exact Path Matching ]  (Theorem 2: Recall@1 = 1.0 on file basenames)
        │
        ├─► [ Stage 2: Symbol AST Resolution ] (Deterministic $0 LLM compute bypass)
        │
        ├─► [ Stage 3: Entity Lookup ]        (Canonical ID / Hash resolution)
        │
        ├─► [ Stage 4: Lexical Matching ]     (BM25 with Block-Max WAND dynamic pruning)
        │
        ├─► [ Stage 5: Semantic Hashing ]     (64-bit Locality-Sensitive Cosine Proxy)
        │
        ├─► [ Stage 6: Graph Expansion ]      (Candidate-local graph neighborhood walk)
        │
        └─► [ Stage 7: Reciprocal Rank Fusion] (RRF k=60 + Multiplicative Authority Gating)
```

1. **Stage 1 — Exact Path Matching (`exact_path`)**:
   Matches absolute paths and basename paths deterministically against the repository working tree. If an exact file is targeted, it is retrieved with score $1.0$ unconditionally.
2. **Stage 2 — Symbol AST Resolution (`symbol`)**:
   Resolves code symbols (functions, types, interfaces, methods) directly from the AST symbol index. Symbol queries bypass LLM reasoning completely, returning exact definition slices and signatures at zero inference cost.
3. **Stage 3 — Entity Lookup (`entity`)**:
   Direct primary-key and hash-addressed memory lookups ($id(m) = H(\text{content}, \text{rev}, \text{loc})$).
4. **Stage 4 — Lexical Retrieval (`lexical`)**:
   Robertson-Sparck Jones BM25 probabilistic scoring with length normalization and **Block-Max WAND (BMW)** upper-bound dynamic score pruning, skipping non-competitive postings blocks to achieve sub-millisecond retrieval (0.91ms P50).
5. **Stage 5 — Semantic Hashing (`semantic`)**:
   Locality-sensitive cosine similarity computed over 64-bit feature hashed projections without heavy ONNX/neural dependencies.
6. **Stage 6 — Graph Neighborhood Expansion (`expansion`)**:
   Performs bounded $k$-hop expansion across AST call graphs, import graphs, and memory links:
   $$\text{Complexity} = O(T(|V_q| + |E_q|) + k)$$
7. **Stage 7 — Fusion & Quotient Deduplication (`rerank`)**:
   Combines rank lists via Reciprocal Rank Fusion ($k=60$) and collapses cross-worktree duplicates into canonical quotient-space units ($\text{WCR} = 0$).

---

## 4. The 6-Pass Context Optimization Pipeline (ASC-1)

When an agent requests a context plan (`ctx plan -task "..." -budget B`), ContextOS passes retrieved candidates into the 6-pass allocator executed in $O(n \log n)$ time:

```
Candidates from Retrieval
    │
    ▼
[ Pass 1: Individual Scoring ]
    │  - BM25 Lexical + HashSemantic + Graph Centrality
    │  - Authority & Staleness Filters (Hard Rejections → -Inf)
    ▼
[ Pass 2: Reciprocal Rank Fusion ]
    │  - Fused Score = RRF(k=60) × Authority × Freshness × W_conf(Confidence)
    ▼
[ Pass 3: Marginal-Utility Greedy Packing ]
    │  - Density = Score_i / Tokens_i
    │  - Pack highest density candidates until Budget B reached
    ▼
[ Pass 4: (1 - 1/e) Singleton Rescue ]
    │  - Compare Greedy Set score vs max single valid item score
    │  - If Best Singleton > Sum(Greedy), rescue singleton
    ▼
[ Pass 5: Optimal Residual Fill Pass ]
    │  - Pack remaining budget slack with next-highest density items
    ▼
[ Pass 6: KV-Cache Topology Partitioning ]
    │  - Stable Prefix: decisions, constraints, code, facts (Cached)
    │  - Variable Suffix: state, observations, volatile context
    ▼
Final ContextPlan
```

1. **Pass 1 — Multi-Signal Scoring**:
   - BM25 lexical score $lex_i \in [0, 1]$ with length normalization against candidate corpus average length $\bar{L}$.
   - Locality-sensitive cosine proxy $sem_i \in [0, 1]$ via 64-bit word feature hashing.
   - Graph structural centrality proxy derived from file path and package namespace depth.
2. **Pass 2 — Reciprocal Rank Fusion (RRF)**:
   - Rank sorting along Semantic, Lexical, and TaskAffinity lists:

$$
\text{RRF}(i) = \sum_{S \in \{\text{sem}, \, \text{lex}, \, \text{aff}\}} \frac{1}{60 + \text{rank}_S(i)}
$$

   - Multiplicative gating:

$$
\text{Score}_i = \text{RRF}(i) \cdot \text{Authority}_i \cdot \text{Freshness}_i \cdot W_{\text{conf}}(\text{Confidence}_i)
$$

3. **Pass 3 — Marginal-Utility Greedy Knapsack**:
   - Candidates sorted descending by marginal density: $\rho_i = \text{Score}_i / \text{Tokens}_i$.
   - Greedy accumulation while $\sum \text{Tokens} \le B$.
4. **Pass 4 — Singleton Rescue (Chvátal / Sviridenko Guarantee)**:
   - Evaluates:

$$
m^* = \arg\max_{j : \text{Tokens}_j \le B \land \text{Valid}(j)} \text{Score}_j
$$

   - If $\text{Score}_{m^*} > \sum_{j \in S_{\text{greedy}}} \text{Score}_j$, replace $S_{\text{greedy}}$ with $\{m^*\}$.
   - Restores theoretical $(1 - 1/e) \approx 0.632$ worst-case performance guarantee for 0/1 knapsack problems.
5. **Pass 5 — Residual Fill Pass**:
   - George & Kim greedy residue packing: scans remaining density-ordered candidates to fill any token budget holes left after greedy or singleton selection.
6. **Pass 6 — Prompt-Cache Topology Partitioning**:
   - Partitions selected candidates into:
     - `StablePrefix`: High-invariance items (`decision`, `constraint`, `code`, `fact`) guaranteed to produce prefix cache hits in Claude/OpenAI/Gemini prompt caches.
     - `VariableContext`: Dynamic items (`state`, `observation`, scratchpad) placed at the end of the prompt window.

---

## 5. Adaptive Compute Optimization & Reasoning Control

Uncontrolled test-time reasoning models (e.g. o1/o3-mini, Claude 3.7 Thinking) frequently burn thousands of reasoning tokens on simple fact lookups or spin in loops on under-specified problems. ContextOS implements **Minimum-Sufficient Compute ($R^*$)**:

### The Compute Optimization Problem

$$
\begin{aligned}
\min_{m \in \mathcal{M}, \, e \in \mathcal{E}} \quad & \mathbb{E}\left[ C_{\text{E2E}}(m, e \mid q) \right] \\
\text{subject to} \quad & Q_{\text{LCB}}(m, e \mid q) \ge Q_{\text{min}}
\end{aligned}
$$

where $C_{\text{E2E}}$ is the combined retrieval, context input, reasoning output, and completion cost, and $Q_{\text{LCB}}$ is the lower confidence bound of task completion quality.

### Value of Information (VOI) & Uncertainty Classification

ContextOS classifies incoming developer tasks into two orthogonal regimes before model dispatch:
1. **Information-Limited Tasks**: The task requires codebase facts, symbol locations, or architecture rules. Increasing thinking tokens cannot solve missing information. ContextOS injects high-density context and clamps reasoning effort to `minimal` or `low`.
2. **Reasoning-Limited Tasks**: The code is already localized, but complex logic, algorithmic invariants, or distributed concurrency are involved. ContextOS allocates higher reasoning effort (`medium` to `high`).

### Calibrated Effort Tiers

| Effort Level | Reasoning Budget | Target Task Profile | Typical Cost |
|---|:---:|---|:---:|
| **`minimal`** | 0 tokens (bypassed) | AST symbol lookup, syntax edits, direct fact recall | $0.0000 |
| **`low`** | 1,024 tokens | Localized bugfixes, unit test creation, small refactors | $0.0015 |
| **`medium`** | 8,192 tokens | Multi-module feature implementations, API modifications | $0.0080 |
| **`high`** | 16,384 tokens | Deep architectural refactoring, subtle concurrency bugs | $0.0160 |
| **`maximum`** | 32,768 tokens | Formal algorithm design, critical cryptographic protocols | $0.0320 |

---

## 6. Multi-Tenant Enterprise SaaS Infrastructure

ContextOS includes a production-grade multi-tenant gateway (`contextos-proxy`) that transparently intercepts AI coding agent API traffic, applies real-time context injection and cache stabilization, and tracks billable token economics:

```
AI Coding Agents (Claude / Cursor / Codex)
                    │
                    ▼
       ┌─────────────────────────┐
       │   contextos-proxy       │
       │  - API Key Auth & Tenancy│
       │  - Rate Limiter (Token) │
       │  - Dual-Tier Metering   │
       │  - Savings Accounting   │
       └────────────┬────────────┘
                    │
                    ▼
       Upstream Providers (Anthropic, OpenAI, Gemini)
```

### Key Enterprise Features:
- **Tenant Isolation**: Secure namespace partitioning for repositories, memories, and work items.
- **Dual-Tier Billing Counters**: Distinguishes between standard input/output tokens billed by upstream providers and tokens saved/pruned by ContextOS.
- **Stripe-Compatible Invoicing**: Generates verified billing statements detailing baseline costs, actual spend, and net dollar savings per organization.
- **SLA Safeguards**: Adaptive timeout throttling down-scales context budget when approaching latency deadlines.

---

## 7. Model Context Protocol (MCP) Surface

ContextOS exposes an authoritative stdio MCP server (`contextd -mcp`) offering **13 specialized tools** and **5 canonical resources**:

### 13 MCP Tools:
1. `context_compute_plan`: Calculate the optimal minimum-sufficient reasoning effort and model choice for a given task.
2. `context_plan`: Build an authoritative, token-bounded context plan partitioned into stable prefix and dynamic tail.
3. `context_search`: Perform hybrid lexical, semantic, and graph-centrality search across repository knowledge.
4. `context_remember`: Persist a durable engineering decision, architectural constraint, or failure postmortem.
5. `context_invalidate`: Invalidate obsolete engineering decisions upon repository refactoring.
6. `context_resume`: Reconstitute active project state, recent decisions, open work items, and past pitfalls.
7. `context_handoff`: Export compact, evidence-backed context tailored for switching agents.
8. `context_route`: Classify task difficulty and route to optimal model tier.
9. `context_work_start`: Open a new tracked work item with goal and acceptance criteria.
10. `context_event`: Log agent activity, tool calls, and lifecycle events.
11. `context_session_start`: Initialize a monitored agent session.
12. `context_trace`: Record execution trajectory and counterfactual reward feedback.
13. `context_stats`: Retrieve real-time memory counts, cache hit ratios, and cumulative savings.

### 5 Canonical Resources:
- `context://repo/current`: Current repository identity, HEAD revision, branch, and worktree status.
- `context://work-item/current`: Active engineering work item, target files, and constraints.
- `context://memory/relevant`: Real-time candidate memory items matching current work state.
- `context://session/latest`: Telemetry and turn history of the most recent agent session.
- `context://trace/latest`: Counterfactual reward traces and performance metrics.

---

## 8. Operational CLI Workflows

### Setup a Repository
```bash
cd /path/to/my-project
ctx setup
```
Initializes storage in `~/.contextos/context.db` (or file store), scans Git revision state, indexes AST symbols, and installs hooks and MCP configs for Claude Code (`~/.claude`), Cursor (`.cursor`), Codex (`.codex`), Gemini (`.gemini`), and Antigravity (`.agents`).

### Query Knowledge & Symbols via 7-Stage Retrieval
```bash
ctx retrieve -query "allocator.go" -mode bmw -limit 10
```
Runs the 7-stage hybrid retrieval pipeline directly against repository code and memory graph with sub-millisecond Block-Max WAND dynamic pruning.

### Build a Token-Bounded Context Plan
```bash
ctx plan -task "Implement Redis failover with redlock" -budget 4000 -render
```
Runs the 6-pass allocator, prints candidate breakdown, density metrics, token allocations, and displays the assembled prompt divided into Stable Prefix and Variable Context.

### Resume State Across Sessions or Restarts
```bash
ctx resume -repo .
```
Prints active branch, HEAD revision, open work item, recent decisions, past failures to avoid, and the recommended next step.

### Launch the Local Web Dashboard
```bash
ctx ui -addr :8080 -open
```
Starts the local real-time telemetry dashboard, visual graph explorer, and interactive allocation playground.

### Record Durable Decisions and Negative Knowledge
```bash
# Record an architectural decision
ctx remember -kind decision -authority user \
  -content "Use optimistic concurrency via version column in orders table"

# Record a failure (negative knowledge)
ctx remember -kind failure -authority test \
  -content "Pessimistic locking caused deadlocks under 50 concurrent workers"
```

### Hand Off Work Between Different AI Agents
```bash
ctx handoff -task "Finish distributed lock unit tests" -target codex -budget 4000
```
Exports compact, evidence-backed context directly formatted for the target agent's system prompt.

### Switching Storage Engines (`ctx migrate`)
ContextOS provides seamless bidirectional data migration between SQLite and FileStore:
```bash
# Migrate existing SQLite database to zero-DB FileStore
ctx migrate -to file

# Migrate FileStore data back to SQLite
ctx migrate -to sqlite
```

### Benchmark and Invariant Auditing
```bash
# Run the longitudinal benchmark suite
ctx bench -longitudinal -generations 10 -budget 2048

# Run system verification and mathematical invariant audit
ctx audit
```

### Ephemeral Storage Management & Garbage Collection (`ctx gc`)
Storage growth from high-frequency execution traces and cached plans is strictly opt-in:
```bash
# Dry run: view expired traces and cache entries eligible for pruning
ctx gc -repo . -keep-days 30 -dry-run

# Execute garbage collection
ctx gc -repo . -keep-days 30

# Opt-in background pruning during normal context planning
ctx plan -task "refactor checkout" -auto-prune
```

---

## 9. Verification & Test Suite

The test suite enforces rigorous mathematical and architectural invariants. Run all checks via:

```bash
make test          # Runs all unit and integration tests across packages
go vet ./...       # Static analysis and linting
make all           # Compiles cmd/ctx and cmd/contextd binaries
```

### Test Suite Breakdown:
- **`internal/retrieval`**:
  - *Exact Retrieval Completeness*: Validates $\text{Recall@1} = 1.0$ for exact paths and symbols.
  - *Block-Max WAND Pruning*: Validates score invariance between BMW dynamic pruning and exhaustive BM25 scans.
  - *Quotient Canonicalization*: Enforces Worktree Contamination Rate $\text{WCR} = 0$.
- **`internal/compute`**:
  - *Minimum-Sufficient Compute*: Tests $R^*$ optimization against synthetic and real task difficulty matrices.
  - *Deterministic AST Bypass*: Verifies $0.00 compute cost and microsecond latency for pure symbol resolution.
  - *Effort Floor & Saturation*: Verifies calibrated stopping at marginal utility knees.
- **`internal/saas`**:
  - *Multi-Tenant Isolation*: Verifies cross-tenant data separation and API key authentication.
  - *Token Metering & Rate Limiting*: Validates token counter precision and 429 rate limit triggers.
  - *Invoicing & Savings Ledger*: Reconciles baseline vs cached/pruned dollar economics.
- **`internal/ui`**:
  - *Dashboard Assets*: Verifies embedded frontend delivery and API endpoints (`/api/status`, `/api/memories`, `/api/sessions`).
- **`internal/allocator` (30 Tests)**:
  - *Edge Cases*: Zero budget, negative budget, empty candidate list, all-rejected inputs, exact fit, single-item overflow.
  - *Scoring Bounds*: Low-authority hard rejection ($<0.6$), explicit invalidation rejection ($-\infty$ density), confidence dampening ($0.95$ vs $0.30$), revision freshness penalties.
  - *Mathematical Properties*: RRF rank monotonicity, commutativity, singleton rescue superiority, fill-pass budget saturation.
  - *Structural Invariants*: `SelectedTokens == sum(Selected.Tokens)`, `SelectedTokens <= Budget`, `Selected == StablePrefix ∪ VariableContext`.
- **`internal/store` (Contract & Migration Tests)**:
  - *Unified Contract*: Identical repository lifecycle, node graph indexing, memory storage/retrieval, work item tracking, session lifecycle, lifecycle events, and trace recording across both `SQLiteStore` and `FileStore`.
  - *Bidirectional Migration*: Validates zero-loss roundtrip migration (`SQLite -> FileStore -> SQLite`) preserving memory IDs, work items, agent sessions, and execution events.
  - *Pure Go Compatibility*: Full test pass under both `CGO_ENABLED=1` and pure-Go `CGO_ENABLED=0`.
- **`internal/textutil` (28 Tests)**:
  - *BM25*: Full match, disjoint terms, length normalization penalty for verbose docs, score bounds $\in [0, 1]$.
  - *HashSemantic*: Self-similarity $= 1.0$, symmetric similarity, cosine range $[0, 1]$.
  - *Token Estimation*: Sub-word approximations, whitespace handling, positive nonzero guarantees.
