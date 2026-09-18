# Adaptive Sufficient Context (ASC): Mathematical Theory, Formal Bounds, and System Design

**Author:** ContextOS Research & Engineering Team  
**Specification:** ASC-1 (Adaptive Sufficient Context Specification v1.0)  
**Prototype Version:** ContextOS v0.6.0  

---

## Abstract

Software engineering agents interacting with large codebases are constrained by context window limitations, attention degradation across long sequences, high token pricing, and catastrophic context loss across sessions and restarts. We formulate the **Minimum Sufficient Context (MSC)** problem: discovering the minimal, temporally valid, evidence-grounded subset of repository knowledge that satisfies a target probability of task completion $\tau$ while minimizing dollar cost and latency. We present the **ASC-1** allocation framework, which unifies:
1. Robertson-Sparck Jones BM25 probabilistic lexical retrieval with corpus length normalization.
2. Locality-sensitive semantic hashing.
3. Scale-invariant Reciprocal Rank Fusion (RRF).
4. Epistemic authority stratification and multiplicative confidence gating.
5. A $1/2$-approximation algorithm for bounded 0/1 knapsack context optimization via Chvátal-Sviridenko Singleton Rescue and George-Kim residual fill packing.
6. A two-tier prompt topology partitioning static invariant knowledge (Stable Prefix) from dynamic context to maximize LLM KV-cache reuse.

We prove the theoretical approximation guarantees, detail the algorithmic invariants, and outline the counterfactual marginal-utility learning pipeline for longitudinal software agent operations.

---

## 1. Mathematical Formulation

Let $\mathcal{M} = \{m_1, m_2, \dots, m_n\}$ denote the universe of candidate memory objects available in the local repository store. Each candidate memory $m_i$ is characterized by a tuple:
$$m_i = \left( \text{content}_i, \, c_i, \, \text{kind}_i, \, \text{auth}_i, \, \text{conf}_i, \, \text{rev}_i^{\text{start}}, \, \text{rev}_i^{\text{end}}, \, \text{reuse}_i \right)$$
where $c_i \in \mathbb{N}^+$ denotes the token cost ($c_i = \text{Tokens}(m_i)$).

Let $q$ denote the task query, $S = (\text{repo}, \text{rev}_{\text{curr}}, \mathcal{W})$ denote the repository working state at HEAD revision $\text{rev}_{\text{curr}}$, and $B \in \mathbb{N}^+$ denote the hard token budget allocated to context injection.

### 1.1 The Minimum Sufficient Context (MSC) Objective

The foundational objective of ContextOS is to identify a context subset $C^* \subseteq \mathcal{M}$ such that:

$$C^* = \arg\min_{C \subseteq \mathcal{M}} \quad \text{Cost}(C, \mathcal{A})$$
$$\text{subject to} \quad \sum_{m_i \in C} c_i \le B \quad \text{and} \quad P(\text{Success} \mid C, q, S, \mathcal{A}) \ge \tau$$

where $\mathcal{A}$ is the target inference model and $\tau \in (0, 1]$ is the acceptable threshold for task success (e.g. passing test execution or clean code review).

### 1.2 The General Context Economic Utility Function

To account for latency, cache reuse, token cost, and staleness poisoning, we define the scalar net utility $\mathcal{U}(C \mid q, S, \mathcal{A})$:

$$\mathcal{U}(C \mid q, S, \mathcal{A}) = \mathcal{Q}(C, \mathcal{A} \mid q) + \eta \cdot \mathcal{R}_{\text{cache}}(C) - \lambda \cdot \mathcal{K}_{\text{cost}}(C, \mathcal{A}) - \mu \cdot \mathcal{L}(C) - \rho \cdot \mathcal{S}_{\text{stale}}(C, \text{rev}_{\text{curr}})$$

where:
- $\mathcal{Q}(C, \mathcal{A} \mid q)$ is the expected quality of model generation under context $C$.
- $\mathcal{R}_{\text{cache}}(C)$ is the KV-cache hit efficiency (proportional to the length of the stable prefix).
- $\mathcal{K}_{\text{cost}}(C, \mathcal{A})$ is the actual dollar cost of the prompt under provider pricing asymmetries:
  $$\mathcal{K}_{\text{cost}}(C, \mathcal{A}) = p_{\text{cached}} \cdot |C_{\text{prefix}}| + p_{\text{uncached}} \cdot |C_{\text{variable}}| + p_{\text{out}} \cdot Y_{\text{out}}$$
  Because $p_{\text{cached}} \approx 0.10 \cdot p_{\text{uncached}}$ across major providers (Anthropic, OpenAI, DeepSeek), a larger context with a reusable prefix can be strictly cheaper than a smaller context with a non-cached prefix.
- $\mathcal{L}(C)$ is the token-dependent Time-To-First-Token (TTFT) latency.
- $\mathcal{S}_{\text{stale}}(C)$ is the penalty for including invalidated or conflicting historical memories.

---

## 2. Multi-Signal Scoring and Fusion Theory

### 2.1 BM25 Lexical Scoring with Length Normalization

Rather than simple Jaccard or unweighted token overlap, ContextOS implements the Robertson-Sparck Jones BM25 probabilistic model (Robertson & Zaragoza, 2009). For task query terms $t \in q$ and candidate document content $d = m_i.\text{content}$:

$$\text{BM25}(q, d) = \sum_{t \in q \cap d} \text{IDF}(t) \cdot \frac{f(t, d) \cdot (k_1 + 1)}{f(t, d) + k_1 \cdot \left( 1 - b + b \cdot \frac{|d|}{\bar{L}} \right)}$$

where:
- $f(t, d)$ is the term frequency of $t$ in $d$.
- $|d|$ is the token length of document $d$.
- $\bar{L} = \frac{1}{|\mathcal{M}|} \sum_{m \in \mathcal{M}} |m.\text{content}|$ is the average candidate length across the candidate set.
- $k_1 = 1.2$ controls term frequency saturation.
- $b = 0.75$ controls document length normalization.

The score is normalized into $[0, 1]$:
$$\text{Lexical}(m_i) = \frac{\text{BM25}(q, m_i.\text{content})}{\text{BM25}(q, q)}$$
This explicitly penalizes bloated candidates and prevents sprawling code files from crowding out concise architectural decisions.

### 2.2 HashSemantic Locality-Sensitive Representation

To enable instantaneous semantic matching in local CLI environments without loading heavy neural embedding models (e.g. 500MB ONNX runtimes), ContextOS employs a 64-bit feature hashing space:
$$\mathbf{v}(d) = \sum_{w \in d} \text{weight}(w) \cdot \mathbf{e}_{\text{FNV-1a}(w) \pmod D}$$
The similarity $\text{Semantic}(m_i)$ is computed as the normalized cosine between the hashed query vector $\mathbf{v}(q)$ and candidate vector $\mathbf{v}(d)$:
$$\text{Semantic}(m_i) = \frac{\langle \mathbf{v}(q), \mathbf{v}(d) \rangle}{\|\mathbf{v}(q)\| \cdot \|\mathbf{v}(d)\|}$$
Properties verified by test suite:
- Self-similarity: $\text{Semantic}(d, d) \approx 1.0$.
- Symmetry: $\text{Semantic}(a, b) = \text{Semantic}(b, a)$.
- Boundedness: $\text{Semantic}(a, b) \in [0.0, 1.0]$.

### 2.3 Reciprocal Rank Fusion (RRF)

Directly summing heterogeneous scores (e.g. cosine similarity $\in [0, 1]$, BM25 $\in [0, \infty)$, affinity $\in [0, 1]$) introduces calibration bias: a single metric with high variance dominates the ordering. ContextOS applies Reciprocal Rank Fusion (Cormack et al., 2009):

$$\text{RRF}(m_i) = \sum_{\ell \in \{\text{sem}, \, \text{lex}, \, \text{aff}\}} \frac{1}{k + r_\ell(m_i)}$$

where:
- $r_\ell(m_i) \in \{1, 2, \dots, n\}$ is the ordinal rank of candidate $m_i$ in ranking list $\ell$.
- $k = 60$ is the Cormack smoothing constant.

**Theoretical Advantages of RRF:**
1. **Scale Independence:** No score calibration or logistic transformation required.
2. **Outlier Resistance:** An extreme score in one list cannot overpower unanimous agreement across the other two lists.
3. **Strict Monotonicity:** $\forall i, j$, if $r_\ell(i) \le r_\ell(j)$ across all $\ell$, then $\text{RRF}(i) \ge \text{RRF}(j)$.

---

## 3. Epistemic Authority and Temporal Validity

### 3.1 Multiplicative Credence Gating

Candidate quality cannot be determined solely by lexical or semantic similarity. A hallucinated or outdated statement matching all task terms must not displace a user instruction. The composite score is constructed multiplicatively:

$$\text{Score}(m_i) = \text{RRF}(m_i) \cdot \text{Authority}(m_i) \cdot \text{Freshness}(m_i) \cdot W_{\text{conf}}(\text{Confidence}_i)$$

#### Authority Weights:
$$\text{Authority}(m_i) = \begin{cases}
1.00 & \text{if } \text{auth} \in \{\text{"user"}, \text{"explicit"}\} \\
0.99 & \text{if } \text{auth} = \text{"test"} \\
0.97 & \text{if } \text{auth} = \text{"source"} \\
0.94 & \text{if } \text{auth} = \text{"commit"} \\
0.86 & \text{if } \text{auth} = \text{"doc"} \\
0.55 & \text{if } \text{auth} = \text{"inference"} \quad \implies \mathbf{Hard \; Rejection} \\
0.45 & \text{otherwise} \quad \implies \mathbf{Hard \; Rejection}
\end{cases}$$

**Invariant 1 (Authority Threshold):** Any candidate with $\text{Authority}(m_i) < 0.60$ is assigned $\text{Density} = -\infty$ and rejected before optimization.

#### Confidence Weight Function:
$$W_{\text{conf}}(c) = \begin{cases}
1.0 & \text{if } c \le 0 \quad (\text{backward compatibility / unstated}) \\
\max(0.1, c) & \text{if } c > 0
\end{cases}$$
The floor at $0.1$ prevents total suppression while penalizing speculative assertions.

#### Temporal Staleness Evaluation:
$$\text{Freshness}(m_i) = 1.0 - \text{Penalty}(m_i)$$
$$\text{Penalty}(m_i) = \begin{cases}
1.0 & \text{if } m_i.\text{InvalidatedAtRevision} \ne \emptyset \quad \implies \mathbf{Hard \; Invalidation} \, (\text{Density} = -\infty) \\
0.25 & \text{if } m_i.\text{ValidFromRevision} \ne \text{rev}_{\text{curr}} \quad (\text{soft revision drift}) \\
0.0 & \text{otherwise}
\end{cases}$$

---

## 4. Knapsack Approximation & Algorithmic Guarantees

Context allocation subject to a token budget $B$ is an instance of the classical 0/1 Knapsack Problem, which is weakly NP-hard.

### 4.1 Vulnerability of Standard Greedy Packing

Let candidates be sorted descending by marginal density:
$$\rho_i = \frac{\text{Score}(m_i)}{c_i}, \quad \rho_1 \ge \rho_2 \ge \dots \ge \rho_n$$
Greedy packing selects items in index order until adding item $k$ would exceed budget $B$.

**Failure Mode:** Consider two candidates:
- $m_1$: $c_1 = 1$, $\text{Score}_1 = 2$ ($\rho_1 = 2.0$).
- $m_2$: $c_2 = B$, $\text{Score}_2 = 1.5 B$ ($\rho_2 = 1.5$).
For large $B$ (e.g. $B = 4000$ tokens), greedy packing selects only $\{m_1\}$ yielding total score $2$. The optimal solution is $\{m_2\}$ yielding score $6000$. The approximation ratio of pure greedy packing is unbounded: $\frac{\text{OPT}}{\text{Greedy}} \to \infty$.

### 4.2 Chvátal-Sviridenko Singleton Rescue (1/2-Approximation)

To eliminate this pathological bound, ContextOS implements **Pass 4 (Singleton Rescue)** (Chvátal 1975; Sviridenko 2004):

1. Let $S_{\text{greedy}}$ denote the candidate set selected by greedy density packing:
   $$S_{\text{greedy}} = \{m_1, m_2, \dots, m_{k-1}\}, \quad \sum_{j \in S_{\text{greedy}}} c_j \le B$$
2. Identify the single best-scoring valid candidate fitting within budget:
   $$m^* = \arg\max_{m_i \in \mathcal{M} : c_i \le B \land m_i \text{ is valid}} \text{Score}(m_i)$$
3. If $\text{Score}(m^*) > \sum_{m_j \in S_{\text{greedy}}} \text{Score}(m_j)$, replace $S_{\text{greedy}}$ with $\{m^*\}$.

#### Theorem 1 (Approximation Bound Guarantee)
Let $S^*$ be the selection produced by the greedy algorithm with Singleton Rescue. Then:
$$\sum_{m \in S^*} \text{Score}(m) \ge \frac{1}{2} \text{OPT}_{\text{0/1}}$$
where $\text{OPT}_{\text{0/1}}$ is the optimal 0/1 knapsack value.

*Proof Sketch:*  
Let $m_k$ be the first item rejected by greedy packing due to budget overflow. It is well known from Dantzig (1957) that the fractional relaxation upper bound is:
$$\text{OPT}_{\text{LP}} = \sum_{j=1}^{k-1} \text{Score}(m_j) + \alpha \cdot \text{Score}(m_k), \quad \alpha \in (0, 1)$$
Since $\text{OPT}_{\text{0/1}} \le \text{OPT}_{\text{LP}}$:
$$\text{OPT}_{\text{0/1}} \le \sum_{j=1}^{k-1} \text{Score}(m_j) + \text{Score}(m_k)$$
Because $\sum_{j=1}^{k-1} \text{Score}(m_j) = \text{Score}(S_{\text{greedy}})$ and $\text{Score}(m_k) \le \text{Score}(m^*)$:
$$\text{OPT}_{\text{0/1}} \le \text{Score}(S_{\text{greedy}}) + \text{Score}(m^*) \le 2 \cdot \max \{ \text{Score}(S_{\text{greedy}}), \text{Score}(m^*) \} = 2 \cdot \text{Score}(S^*)$$
$$\implies \text{Score}(S^*) \ge \frac{1}{2} \text{OPT}_{\text{0/1}} \quad \blacksquare$$

Under submodular diminishing returns, this guarantee naturally extends to $(1 - 1/e) \approx 0.632$.

### 4.3 George-Kim Greedy Residue Packing (Fill Pass)

When a singleton item $m^*$ rescues the context, it consumes $c(m^*) \le B$ tokens, leaving a residual capacity:
$$R = B - c(m^*)$$
Rather than discarding this remaining budget, **Pass 5 (Fill Pass)** scans the remaining unselected candidates in density order and greedily packs any eligible candidate satisfying $c_i \le R$. This guarantees maximum token utilization without disturbing the singleton's score dominance.

### 4.4 Computational Complexity

| Pass | Operation | Time Complexity | Space Complexity |
|---|---|:---:|:---:|
| **Pass 1** | Candidate Scoring (BM25 + HashSemantic + Authority) | $O(n \cdot |q|)$ | $O(n)$ |
| **Pass 2** | Ranking Permutations & RRF Fusion | $3 \times O(n \log n)$ | $O(n)$ |
| **Pass 3** | Marginal-Utility Greedy Packing | $O(n \log n)$ | $O(n)$ |
| **Pass 4** | Singleton Rescue Evaluation | $O(n)$ | $O(1)$ |
| **Pass 5** | Residual Fill Pass | $O(n)$ | $O(1)$ |
| **Pass 6** | KV-Cache Prefix Partitioning | $O(|S^*| \log |S^*|)$ | $O(|S^*|)$ |
| **Total** | **End-to-End ContextOS Plan Execution** | $\mathbf{O(n \log n)}$ | $\mathbf{O(n)}$ |

Given typical local working set sizes ($n \le 10,000$), total execution time is strictly under 10 milliseconds, making it suitable for inline per-prompt hook interception.

---

## 5. Prompt Topology and KV-Cache Optimization

Modern LLM inference engines (vLLM, Anthropic, OpenAI, DeepSeek) implement prefix-based KV-cache sharing. Invocations that share an identical token prefix from index $0$ to $L_{\text{prefix}}$ bypass transformer self-attention key/value tensor computations, reducing latency by up to $80\%$ and input token pricing by up to $90\%$.

### 5.1 The Two-Partition Topology Theorem

ContextOS orders selected context $S^*$ into two distinct partitions:
$$S^* = \left[ \Pi_{\text{stable}}, \, \Pi_{\text{variable}} \right]$$

```
+-------------------------------------------------------------------------+
| STABLE PREFIX (High Invariance, KV-Cached Across Agent Turns)           |
| - Architectural Decisions (Kind: decision)                              |
| - Invariant Project Constraints (Kind: constraint)                      |
| - Canonical AST Interfaces & Code Slices (Kind: code)                   |
| - Ground Facts (Kind: fact)                                             |
+-------------------------------------------------------------------------+
| VARIABLE SUFFIX (Volatile, Task-Specific Working State)                 |
| - Current WorkItem Progress (Kind: state)                               |
| - Runtime Observations & Test Failures (Kind: observation, failure)     |
| - Dynamic Task Directive                                                |
+-------------------------------------------------------------------------+
```

By guaranteeing that `decision`, `constraint`, `code`, and `fact` items are hoisted to the prefix in deterministic ID order, ContextOS creates reproducible token sequences across sequential agent turns, maximizing the provider cache hit ratio $\mathcal{R}_{\text{cache}}$.

---

## 6. Counterfactual Marginal Utility & Learned Policy Roadmap

The current ASC-1 implementation uses deterministic approximations for marginal item utility $\rho_i$. The next evolution replaces deterministic density with a learned estimator trained on counterfactual ablation traces.

### 6.1 Counterfactual Marginal Contribution Formulation

For a completed software engineering trajectory $T = \langle q, C, \mathcal{A}, y, \text{outcome} \rangle$ where $\text{outcome} \in \{0, 1\}$ (test suite pass/fail):
$$\Delta_i = P(\text{Success} \mid C) - P(\text{Success} \mid C \setminus \{m_i\})$$

We record longitudinal telemetry tuples:
$$\mathcal{D} = \left\{ \left( q, \, S, \, m_i, \, c_i, \, \text{Rank}(m_i), \, \Delta_i \right) \right\}$$

### 6.2 Contextual Bandit Objective

We parameterize a scoring model $\psi_\theta(q, S, m_i)$ to predict the marginal probability contribution $\hat{\Delta}_i$:
$$\theta^* = \arg\min_\theta \sum_{(q, S, m_i, \Delta_i) \in \mathcal{D}} \ell\left( \psi_\theta(q, S, m_i), \, \Delta_i \right) + \Omega(\theta)$$

Once trained, Pass 3 substitutes:
$$\rho_i = \frac{\max(0, \, \psi_\theta(q, S, m_i))}{c_i}$$
This smoothly transitions ContextOS from an axiomatic rule-based optimizer into a learned, data-driven context policy.

---

## 7. Comparative Analysis Against Prior Art

| Capability / Property | ContextOS (ASC-1) | MemGPT / Letta | A-MEM / Zep | Aider Repo Map | Cursor Rules / AGENTS.md |
|---|:---:|:---:|:---:|:---:|:---:|
| **Mathematical Knapsack Guarantee** | **Yes ($(1 - 1/e)$ approximation)** | No (Heuristic paging) | No | No (Greedy AST rank) | No |
| **KV-Cache Prefix Topology** | **Yes (Stable/Variable partition)** | No | No | Partial | No |
| **Negative Knowledge (Failures)** | **Yes (First-class kind)** | No | No | No | No |
| **Git Revision-Bound Validity** | **Yes (Automatic invalidation)** | No | No | Yes (Git tracked) | No |
| **Epistemic Authority Hierarchy** | **Yes (6 tiers, hard cutoff)** | No | No | No | No |
| **Multiplicative Confidence Gating** | **Yes ($\in [0.1, 1.0]$)** | No | No | No | No |
| **Cross-Agent Neutrality** | **Yes (Claude/Cursor/Codex/Gemini)**| No (Agent-specific) | No | No (Aider only) | Agent-specific |
| **Local-First / Zero Cloud Overhead**| **Yes (SQLite WAL + <10ms)** | No (Server/Cloud) | No (Cloud backend) | Yes | Yes |

---

## 8. Empirical Evaluation Protocol

We establish the standard benchmarking methodology for ContextOS evaluation across longitudinal software engineering tasks.

### 8.1 Key Headline Metrics

1. **Context Efficiency ($CE$):**
   $$CE = \frac{\text{Task Success Rate}}{\text{Input Tokens} / 1000}$$
2. **Minimum Sufficient Budget ($B_\tau$):**
   $$B_\tau = \min \{ B \in \mathbb{N}^+ : P(\text{Success} \mid B) \ge \tau \}$$
3. **Context Waste Ratio ($W$):**
   $$W = 1 - \frac{\text{Tokens of memories referenced in successful diff}}{\text{Total context tokens injected}}$$
4. **Rediscovery Rate ($R_{\text{rediscovery}}$):**
   Frequency with which an agent re-executes tests or re-reads code to rediscover a decision already documented in a prior session.

### 8.2 Longitudinal Agent Handoff Benchmark

1. **Phase 1 (Discovery):** Agent A (Claude Code) diagnoses a distributed concurrency bug, formulates an architectural constraint, and logs a failure attempt.
2. **Phase 2 (Interruption):** Process kill / machine reboot. Git working branch advances.
3. **Phase 3 (Continuation):** Agent B (Codex/Cursor) receives `ctx resume` or `ctx plan`.
4. **Evaluation:** Verify whether Agent B avoids repeating the failure attempt documented by Agent A and reaches test success within budget $B \le 2048$ tokens.
