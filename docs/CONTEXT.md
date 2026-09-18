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
          |  ctx CLI Tool    |                                |  MCP Server Engine |
          | (User/Script UI) |                                | (contextd stdio)   |
          +--------+---------+                                +---------+----------+
                   |                                                    |
                   +--------------------------+-------------------------+
                                              |
                                              v
                              +-------------------------------+
                              |    ASC-1 Context Allocator    |
                              |  - BM25 & HashSemantic Pass   |
                              |  - Reciprocal Rank Fusion     |
                              |  - Greedy Density Packing     |
                              |  - (1-1/e) Singleton Rescue   |
                              |  - Residual Fill Pass         |
                              |  - KV-Cache Stable Prefixing  |
                              +---------------+---------------+
                                              |
                     +------------------------+------------------------+
                     v                                                 v
          +-----------------------+                         +----------------------+
          |   Git State Engine    |                         | Pluggable Store Engine|
          |  - Revision & Branch  |                         |  - SQLite (WAL+FTS5) |
          |  - Diff & Invalidation|                         |  - FileStore(Pure Go)|
          |  - Graph Centrality   |                         |  - Bidirectional Migr|
          +-----------------------+                         +----------------------+
```

### Core Packages & Directory Map

| Package / Directory | Primary Responsibility | Key Files |
|---|---|---|
| [`internal/allocator`](file:///Users/rohitshukla/Desktop/ContextOS/internal/allocator) | 6-pass budget-constrained context optimization algorithm (ASC-1) | [`allocator.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/allocator/allocator.go), [`allocator_test.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/allocator/allocator_test.go) |
| [`internal/model`](file:///Users/rohitshukla/Desktop/ContextOS/internal/model) | Canonical domain entities, memory structures, candidate envelopes, context plans | [`types.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/model/types.go) |
| [`internal/store`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store) | Pluggable persistence layer: SQLiteStore (WAL+FTS5) and FileStore (Pure-Go stdlib), bidirectional migration (`ctx migrate`), and opt-in pruning | [`store.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/store.go), [`sqlite_store.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/sqlite_store.go), [`file_store.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/file_store.go), [`migrate.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/store/migrate.go) |
| [`internal/db`](file:///Users/rohitshukla/Desktop/ContextOS/internal/db) | Low-level SQLite driver with build-tag isolation (`//go:build cgo` and `sqlite_nocgo.go` stubs) | [`sqlite.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/db/sqlite.go), [`sqlite_nocgo.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/db/sqlite_nocgo.go) |
| [`internal/textutil`](file:///Users/rohitshukla/Desktop/ContextOS/internal/textutil) | Robertson-Sparck Jones BM25, Locality-Sensitive HashSemantic, token estimation | [`textutil.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/textutil/textutil.go), [`textutil_test.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/textutil/textutil_test.go) |
| [`internal/gitidx`](file:///Users/rohitshukla/Desktop/ContextOS/internal/gitidx) | Git working tree inspection, HEAD commit hash resolution, branch detection | [`scanner.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/gitidx/scanner.go) |
| [`internal/mcp`](file:///Users/rohitshukla/Desktop/ContextOS/internal/mcp) | Model Context Protocol JSON-RPC server (12 tools, 5 resources) | [`server.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/mcp/server.go) |
| [`internal/hook`](file:///Users/rohitshukla/Desktop/ContextOS/internal/hook) | Hook ingestion, normalization, and context injection handlers for Claude, Cursor, Codex, Gemini, Antigravity | [`handler.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/hook/handler.go) |
| [`internal/integrations`](file:///Users/rohitshukla/Desktop/ContextOS/internal/integrations) | Automated installer for hooks, MCP configuration, and agent guidelines | [`integrations.go`](file:///Users/rohitshukla/Desktop/ContextOS/internal/integrations/integrations.go) |
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

## 3. The 6-Pass Context Optimization Pipeline (ASC-1)

When an agent requests a context plan (`ctx plan -task "..." -budget B`), ContextOS executes 6 mathematically-grounded passes in $O(n \log n)$ time:

```
Candidates
    │
    ▼
[ Pass 1: Individual Scoring ]
    │  - BM25 Lexical (TF Saturation + Length Normalization)
    │  - HashSemantic (64-bit Locality-Sensitive Cosine Proxy)
    │  - Authority & Staleness Filters (Hard Rejections → -Inf)
    ▼
[ Pass 2: Reciprocal Rank Fusion ]
    │  - Cross-candidate rank fusion across Semantic, Lexical, Affinity
    │  - Fused Score = RRF(k=60) × Authority × Freshness × Confidence
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

## 4. Operational CLI Workflows

### Setup a Repository
```bash
cd /path/to/my-project
ctx setup
```
Initializes the storage schema in `~/.contextos/context.db` (or file store), scans Git revision state, indexes AST symbols, and installs hooks and MCP configs for Claude Code (`~/.claude`), Cursor (`.cursor`), Codex (`.codex`), Gemini (`.gemini`), and Antigravity (`.agents`).

### Resume State Across Sessions or Restarts
```bash
ctx resume -repo .
```
Prints active branch, HEAD revision, open work item, recent decisions, past failures to avoid, and the recommended next step.

### Build a Token-Bounded Context Plan
```bash
ctx plan -task "Implement Redis failover with redlock" -budget 4000 -render
```
Runs the 6-pass allocator, prints candidate breakdown, density metrics, token allocations, and displays the assembled prompt divided into Stable Prefix and Variable Context.

### Hand Off Work Between Different AI Agents
```bash
ctx handoff -task "Finish distributed lock unit tests" -target codex -budget 4000
```
Exports compact, evidence-backed context directly formatted for the target agent's system prompt.

### Record Durable Decisions and Negative Knowledge
```bash
# Record an architectural decision
ctx remember -kind decision -authority user \
  -content "Use optimistic concurrency via version column in orders table"

# Record a failure (negative knowledge)
ctx remember -kind failure -authority test \
  -content "Pessimistic locking caused deadlocks under 50 concurrent workers"
```

### Switching Storage Engines (`ctx migrate`)
ContextOS provides seamless bidirectional data migration between SQLite and FileStore:
```bash
# Migrate existing SQLite database to zero-DB FileStore
ctx migrate -to file

# Migrate FileStore data back to SQLite
ctx migrate -to sqlite

# Explicit paths for custom directory structures
ctx migrate -from sqlite -to file -from-path /var/data/context.db -to-path /var/data/store
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
# Or set environment variable CONTEXTOS_AUTO_PRUNE=1
```

---

## 5. Verification & Test Suite

The test suite enforces rigorous mathematical and architectural invariants. Run all checks via:

```bash
make test          # Runs all unit and integration tests across packages
go vet ./...       # Static analysis and linting
make all           # Compiles cmd/ctx and cmd/contextd binaries
```

### Test Suite Breakdown:
- **`internal/store` (Contract & Migration Tests)**:
  - *Unified Contract*: Identical repository lifecycle, node graph indexing, memory storage/retrieval, work item tracking, session lifecycle, lifecycle events, and trace recording across both `SQLiteStore` and `FileStore`.
  - *Bidirectional Migration*: Validates zero-loss roundtrip migration (`SQLite -> FileStore -> SQLite`) preserving memory IDs, work items, agent sessions, and execution events.
  - *Pure Go Compatibility*: Full test pass under both `CGO_ENABLED=1` and pure-Go `CGO_ENABLED=0`.
- **`internal/server` (Service Lifecycle Tests)**:
  - *Cache Invalidation*: Verifies that file modifications change worktree hash and invalidate context cache.
  - *FileStore Integration*: Verifies end-to-end service lifecycle with pure Go FileStore.
  - *Opt-In Auto-Prune*: Verifies that `--auto-prune` / `CONTEXTOS_AUTO_PRUNE=1` triggers background GC during context planning while defaulting to off.
- **`internal/allocator` (30 Tests)**:
  - *Edge Cases*: Zero budget, negative budget, empty candidate list, all-rejected inputs, exact fit, single-item overflow.
  - *Scoring Bounds*: Low-authority hard rejection ($<0.6$), explicit invalidation rejection ($-\infty$ density), confidence dampening ($0.95$ vs $0.30$), revision freshness penalties.
  - *Mathematical Properties*: RRF rank monotonicity, commutativity, singleton rescue superiority, fill-pass budget saturation.
  - *Structural Invariants*: `SelectedTokens == sum(Selected.Tokens)`, `SelectedTokens <= Budget`, `Selected == StablePrefix ∪ VariableContext`.
- **`internal/textutil` (28 Tests)**:
  - *BM25*: Full match, disjoint terms, length normalization penalty for verbose docs, score bounds $\in [0, 1]$.
  - *HashSemantic*: Self-similarity $= 1.0$, symmetric similarity, cosine range $[0, 1]$.
  - *Token Estimation*: Sub-word approximations, whitespace handling, positive nonzero guarantees.
