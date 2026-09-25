# R15.14: Adaptive Graph Retrieval & Fine-Grained Compute Optimization

**Empirical Evaluation across 120 Stratified Benchmark Tasks**
**Timestamp:** 2026-09-25T10:49:25Z

---

## 1. Engine Performance: Baseline vs. SQLite Optimized vs. Kùzu Prototype

| Retrieval Engine | p50 Latency | p95 Latency | p99 Latency | Nodes Scanned | Edges Traversed | Memory (MB) | Throughput (QPS) | Timeout Rate (>5s SLA) |
|---|---|---|---|---|---|---|---|---|
| **Baseline (Unindexed ListNodes + O(N²) Disk Read)** | 3,420.5 ms | 6,850.2 ms | 9,420.0 ms | 2,450 | 8,900 | 48.5 MB | 0.29 QPS | **34.00%** ❌ |
| **SQLite Optimized (O1–O6 Bounded Best-First + VOI)** | **3.8 ms** | **8.4 ms** | **13.2 ms** | **22** | **36** | **0.65 MB** | **263.1 QPS** | **0.00%** ✅ |
| **Kùzu Embedded Graph Prototype** | 2.9 ms | 6.1 ms | 10.5 ms | 18 | 32 | 3.20 MB | 344.8 QPS | **0.00%** ✅ |

> **Result:** The SQLite Optimized engine achieves a **900x latency reduction** (3.8ms vs 3,420.5ms) and **100% SLA compliance (0% timeout rate)** by eliminating full repository scans on the hot path.

---

## 2. Extended R15 Controller Optimization

We expanded the action space:
$$a_t \in \{ \text{STOP}, \text{LEXICAL}, \text{SYMBOL}, \text{GRAPH}_1, \text{GRAPH}_2, \text{GRAPH}_{\text{adaptive}}, \text{THINK}, \text{VERIFY}, \text{ESCALATE} \}$$

Optimizing:
$$\min_\pi \mathbb{E}[C_{\text{retrieval}} + C_{\text{graph}} + C_{\text{reasoning}} + C_{\text{verification}}] \quad \text{subject to} \quad P(\text{error}) \le 0.05$$

### Cost Breakdown per Task
| Cost Component | Mean Cost (USD) | Share of Total Compute |
|---|---|---|
| **$C_{\text{retrieval}}$ (Lexical / Symbol Index)** | $0.00020 | 0.73% |
| **$C_{\text{graph}}$ (Bounded Best-First VOI)** | $0.00063 | 2.37% |
| **$C_{\text{reasoning}}$ (LLM Thinking Tokens)** | $0.02458 | 91.91% |
| **$C_{\text{verification}}$ (Static Compiler/Tests)** | $0.00133 | 4.99% |
| **Total Expected Cost $\mathbb{E}[C]$** | **$0.02675** | **100.00%** |

### Benchmark Comparison
- **Standard R15 Cost per Success:** $0.05810
- **Graph-Optimized Cost per Success:** **$0.02675**
- **Net Cost Reduction:** **53.97%**
- **Empirical Error Rate:** **2.90%** (Constraint: $P(\text{error}) \le 5.00\%$)

---

## 3. Key Findings
1. **Graph Traversal as Cheap Compute:** Fine-grained graph expansion ($GRAPH_{adaptive}$) replaces heavy LLM reasoning tokens on Class 2 local bugs, solving them with exact dependency retrieval ($0.00075) rather than extended thinking ($0.02500).
2. **VOI Adaptive Stopping:** The $VOI_{next} < Cost_{next}$ condition successfully prevented 88% of useless 2nd-hop traversals.
3. **Kùzu vs. SQLite:** SQLite with O1–O6 indexed candidate seeds is within 0.9ms of native embedded graph DBs while retaining zero external binary dependencies.
