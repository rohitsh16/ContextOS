# ContextOS 1.0.0 — Production Context Runtime & Compute Optimizer

## Included Capabilities

- **7-Stage Authoritative Hybrid Retrieval Pipeline**:
  - Deterministic exact path matching (Theorem 2: Recall@1 = 1.0).
  - AST symbol definition and call-graph resolution with $0 LLM compute bypass.
  - Entity primary-key & hash lookup.
  - Robertson-Sparck Jones BM25 probabilistic lexical retrieval with length normalization.
  - Block-Max WAND (BMW) dynamic score upper-bound pruning (0.91ms P50 latency, 97% postings skipped).
  - 64-bit Locality-Sensitive HashSemantic cosine proxy.
  - Bounded candidate-local graph neighborhood expansion ($O(T(|V_q| + |E_q|) + k)$).
  - Scale-invariant Reciprocal Rank Fusion ($k=60$) with authority and confidence multiplicative gating.
  - Quotient-space worktree canonicalization (Worktree Contamination Rate $\text{WCR} = 0$).

- **Test-Time Compute Optimization ($R^*$)**:
  - Minimum-Sufficient Compute optimizer: $\min E[C_{\text{E2E}}]$ s.t. $Q_{\text{LCB}} \ge Q_{\text{min}}$.
  - Value of Information (VOI) uncertainty routing (Information- vs. Reasoning-limited).
  - Calibrated effort tiering (`minimal`, `low`, `medium`, `high`, `maximum`) with knee termination.
  - Deterministic AST graph bypass eliminating LLM reasoning tokens on code structure queries.

- **Enterprise Multi-Tenant SaaS Gateway**:
  - Reverse proxy (`contextos-proxy`) with API key authentication and tenant isolation.
  - Token-bucket rate limiter and dual-tier billing counters (standard vs saved/pruned tokens).
  - Stripe-compatible invoice generator and verified cost savings ledger.

- **Local Real-Time Telemetry Dashboard (`ctx ui`)**:
  - Embedded zero-dependency Web UI (`index.html`, `style.css`, `app.js`).
  - Real-time Memory Hub, system status, interactive allocation playground, and trace graphs.

- **Model Context Protocol (MCP) Server**:
  - 13 specialized tools: `context_compute_plan`, `context_plan`, `context_search`, `context_remember`, `context_invalidate`, `context_resume`, `context_handoff`, `context_route`, `context_work_start`, `context_event`, `context_session_start`, `context_trace`, `context_stats`.
  - 5 canonical resources: `context://repo/current`, `context://work-item/current`, `context://memory/relevant`, `context://session/latest`, `context://trace/latest`.

- **Pluggable Persistence & Incremental Indexing**:
  - Bidirectional zero-loss data migration (`ctx migrate`) between SQLite (WAL+FTS5) and pure-Go FileStore.
  - AST symbol parsing and incremental Git change detection.
  - Opt-in ephemeral trace garbage collection (`ctx gc`).

## Validation

The release is validated across the complete test matrix:

```text
go test ./...               PASS
go vet ./...                PASS
make all                    PASS
ctx audit                   PASS
Longitudinal Benchmark      PASS (100% success, 0% rediscovery, 77.6% token reduction)
Independent Benchmark       PASS (92.7% - 95.7% spend reduction on real codebases)
Pure Go CGO_ENABLED=0       PASS
```
