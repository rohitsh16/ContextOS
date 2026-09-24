# ContextOS — Evaluation Framework & Empirical Benchmark Report

This document defines the formal experimental framework, statistical metrics, and empirical findings measuring software engineering coding agent service efficiency **with and without ContextOS**.

---

## 1. Primary Research Question

> **Can ContextOS achieve higher software-engineering task success with strictly bounded context, lower total dollar cost, and higher cache hit rates than unmanaged agent baselines?**

---

## 2. Evaluation Protocols & Baselines

### 2.1 The Three-Arm Longitudinal Protocol
Evaluated across 10 sequential agent generations with a 20% repository churn rate:

1. **Arm 1: Stateless Cold (Without ContextOS)**
   - The agent starts fresh on each task with zero memory.
   - Must rediscover file layouts, symbols, and architectural decisions from scratch.
2. **Arm 2: Naive Accumulator (Unconstrained Context Dump)**
   - Unconditionally appends all prior turns and documents without pruning, ranking, or temporal invalidation.
   - Context expands unbounded until hitting budget limits, mixing obsolete and current decisions.
3. **Arm 3: ContextOS Adaptive Runtime (With ContextOS)**
   - Persistent, content-addressable memory ($id(m) = H(\text{content}, \text{rev}, \text{loc})$).
   - Prompt-cache-aware ordering maximizing identical stable prefixes ($P^*$).
   - Hazard-rate adaptive TTL ($TTL_m \propto \frac{1}{h_m + \epsilon}$) and symbol-level invalidation trees.
   - Submodular information-gain allocation and contradiction graph gating.

### 2.2 Independent Evaluation Protocol
Evaluates real-world tasks on actual codebases without controller contamination or internal evaluator leakage:
- Task dataset and ground truth are decoupled from the system under test.
- The evaluator measures pass rates, token reduction, and cloud spend using external provider pricing APIs.

---

## 3. Formal Metrics

### 3.1 Quality & Accuracy
- **Pass@1 / Task Success Rate**: Percentage of tasks satisfying test suites and ground truth without regressions.
- **Handoff Fidelity**: Percentage of architectural constraints successfully preserved when switching models or agents.
- **Rediscovery Rate**: Fraction of turns spent querying previously discovered facts or symbol locations.
- **Contradiction Rate**: Frequency with which mutually exclusive engineering decisions appear in the same prompt.

### 3.2 Context Efficiency
$$
CE = \frac{\text{Success}}{\text{InputTokens} / 1000}
$$

$$
B_\tau = \min \{ B : P(\text{success} \mid B) \ge \tau \}
$$

Where $B_\tau$ is the Minimum Sufficient Context (MSC) budget required to reach threshold $\tau$.

---

## 4. Empirical Benchmark Findings

### 4.1 Longitudinal Multi-Generation Results (10 Generations)

```bash
$ ctxbench -longitudinal -generations 10 -budget 2048
```

| Metric | Stateless Cold | Naive Accumulator | **ContextOS Adaptive** | Delta vs. Stateless |
| :--- | :---: | :---: | :---: | :---: |
| **Final Task Success** | 70.0% | 100.0% (stale risk) | **100.0%** | **+30.0%** |
| **Rediscovery Rate** | 100.0% | 0.0% | **0.0%** | **-100% (eliminated)** |
| **Handoff Success** | 20.0% | 100.0% | **100.0%** | **+80.0%** |
| **Cumulative Spend (USD)** | $0.15750 | $0.19040 | **$0.15577** | **Lowest cost** |
| **Active Memories** | 0 | 18 (unbounded) | **5 (compact, active)** | **Optimal footprint** |
| **Stale Memories Injected**| 0 | 8 stale | **0 stale** | **100% clean** |

### 4.2 Independent Real-Codebase Benchmark (ContextOS Repository)

Evaluates real files read by standard coding agents vs ContextOS packed Stable Prefixes:

| Dimension | Baseline | ContextOS | Improvement |
| :--- | :---: | :---: | :---: |
| **Total Tokens (6 Tasks)** | 27,447 | 6,162 | **77.55% reduction** |
| **Gemini 2.5 Flash 10-Turn Spend** | $0.020585 | $0.001502 | **92.70% cheaper** |
| **Claude 3.7 Sonnet 10-Turn Spend** | $0.8234 | $0.0351 | **95.73% cheaper** |
| **Exact Symbol Lookup (T0 Bypass)**| 1,428 tokens | 13 tokens | **99.1% reduction** |

---

## 5. Verification Commands

Run the empirical evaluation harness locally:
```bash
# Longitudinal 10-generation simulation
./bin/ctxbench -longitudinal -generations 10 -budget 2048

# Multi-budget sweep across [512, 1024, 2048, 4096, 8192]
./bin/ctxbench -sweep -n 50

# Independent real-task token reduction benchmark
go run benchmarks/independent/eval_independent.go

# Independent SWE thinking cost audit
go run benchmarks/independent/swe_thinking_benchmark.go
```
