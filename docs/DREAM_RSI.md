# Dream-RSI: Recursive Self-Improvement through Evolving Worlds in ContextOS

**Architecture Specification & System Design**  
**Specification:** DREAM-RSI-1  
**Status:** Architectural Design & Integration  
**Reference Paper:** arXiv:2609.14858 [cs.AI]  

---

## 1. Academic Citation & Attribution

This architecture implements and embeds the foundational principles introduced in:

> **Dream-RSI: Recursive Self-Improvement through Evolving Worlds**  
> *Tong Zheng, Xidong Wu, Zheng Zhang, Zhankui He, Chaoyi Zhang, Benjamin Coleman, Ruoqiao Wei, Di Bai, Haolin Liu, Rui Liu, Xue Wang, Yue Zhuan, Wang-Cheng Kang, Renkai Xiang, Heng Huang, Xinwu Cheng, Yunsong Guo.*  
> **arXiv preprint arXiv:2609.14858**, September 2026.  
> Primary Subjects: Artificial Intelligence (`cs.AI`), Machine Learning (`cs.LG`), Software Engineering (`cs.SE`).  
> Links: [arXiv:2609.14858](https://arxiv.org/abs/2609.14858)

---

## 2. Motivation & Executive Summary

### 2.1 The Bottleneck in Autonomous Coding Agents
Modern software engineering agents (e.g. Claude Code, Cursor, Antigravity, Codex) spend an overwhelming majority of their token and latency budgets on **exploration**: navigating unfamiliar codebases, generating speculative hypotheses, following unproductive dependency chains, and recovering from dead ends.

Traditional paradigms for Recursive Self-Improvement (RSI) assume continuous fine-tuning or reinforcement learning over the neural network weights. However, for real-world software engineering agents, this approach breaks down due to three severe bottlenecks:
1. **Model Inaccessibility**: State-of-the-art reasoning models (e.g., Claude 3.7 Sonnet, Gemini 2.5 Flash, GPT-4o) are closed-source API endpoints where continuous parameter weight updating is impossible.
2. **Exploration Inefficiency**: Online trial-and-error exploration over live developer workspaces is exorbitantly expensive, slow (high TTFT and wall-clock execution), and risks workspace corruption.
3. **Catastrophic Forgetting & Sample Inefficiency**: Weight updates on localized coding tasks often cause unpredictable regressions on unrelated architectural patterns.

### 2.2 The Dream-RSI Paradigm
The **Dream-RSI** framework (Zheng et al., September 2026) solves this dilemma through three structural insights:
1. **Keep Base Model Weights Frozen**: Treat the underlying foundation model as a fixed semantic engine.
2. **The Exploration Policy as the Object of Improvement**: Explicitly parameterize and optimize the *orchestration and exploration layer*—how the agent traverses search spaces, allocates context, prunes candidate branches, and routes between retrieval strategies.
3. **The "Dreaming" Replay Simulator**: Instead of costly real-world exploration, convert past agent sessions, search trees, and execution traces into deterministic **replay simulators ("evolving worlds")**. The system simulates thousands of alternative exploration policies offline at native execution speeds ($<10\text{ms}$ per branch) without invoking LLM API calls.
4. **Recursive Flywheel**: Validated, superior exploration policies are deployed back into the live agent. The resulting higher-quality traces expand the simulator pool, enabling perpetual recursive self-improvement.

### 2.3 Why ContextOS is the Ideal Dream-RSI Substrate
ContextOS already possesses the four structural prerequisites required by Dream-RSI:
- **Telemetry & Traces**: Ingests fine-grained `HookEvent` streams, tool invocations, and session traces across all IDEs (`internal/hook`, `internal/model`).
- **Temporal Event Sourcing**: Git revision tracking, historical invalidation, and state rollback (`internal/temporal`).
- **Parameterized Allocation**: 6-pass multi-signal context knapsack scoring (`internal/allocator`).
- **Longitudinal Evaluation Engine**: Multi-generation simulation, counterfactual ablation testing, and bootstrap statistical validation (`internal/bench`).

---

## 3. System Architecture

```
                                  ONLINE REAL WORLD
                         ┌─────────────────────────────────┐
                         │   Developer & Coding Agent      │
                         │ (Claude / Cursor / Antigravity) │
                         └───────────────┬─────────────────┘
                                         │
                   1. Live Actions, Prompts, Tools & Errors
                                         ▼
                         ┌─────────────────────────────────┐
                         │   ContextOS Hook Interceptor    │
                         │       (internal/hook)           │
                         └───────────────┬─────────────────┘
                                         │
                         2. Ingests Trajectory Traces & Outcomes
                                         ▼
 ═══════════════════════════════════════════════════════════════════════════════════
                             OFFLINE "DREAM" SUBSYSTEM
                         ┌─────────────────────────────────┐
                         │     Trajectory Tree Builder     │
                         │     (internal/dreamrsi)         │
                         └───────────────┬─────────────────┘
                                         │
                         3. Synthesizes Historical Search Forests
                                         ▼
                         ┌─────────────────────────────────┐
                         │     Replay World Simulator      │
                         │  (Evolving World Environment)   │
                         └───────────────┬─────────────────┘
                                         │
                         4. Simulates Counterfactual Policies
                                         ▼
                     ┌─────────────────────────────────────────┐
                     │          Offline Dreaming Loop          │
                     │  • Mutation & Policy Search ($\pi_\theta$)    │
                     │  • Evaluates Pruning & Retrieval Paths  │
                     │  • Fast Deterministic State Playback    │
                     └───────────────────┬─────────────────────┘
                                         │
                         5. Paired Comparative Statistical Tests
                                         ▼
                     ┌─────────────────────────────────────────┐
                     │         Scientific Gatekeeper           │
                     │ • Bootstrap 95% Confidence Intervals    │
                     │ • Permutation Test ($p < 0.05$)         │
                     │ • Non-parametric Cliff's Delta ($\delta>0.3$) │
                     └───────────────────┬─────────────────────┘
                                         │
 ════════════════════════════════════════╪═══════════════════════════════════════════
                                         │ 6. Hot-Reload Winning Policy Profile
                                         ▼
                         ┌─────────────────────────────────┐
                         │   Runtime Context Allocator     │
                         │    (internal/allocator)         │
                         └─────────────────────────────────┘
```

---

## 4. Mathematical Formulation of Dream-RSI in ContextOS

### 4.1 Trajectory Decision Graph
Let a real agent session $\mathcal{T}$ be modeled as a directed decision tree $\mathcal{T} = (\mathcal{V}, \mathcal{E}, v_0)$, where:
- Each vertex $v \in \mathcal{V}$ represents an agent state $v = (S_t, C_t, q, \mathcal{H}_t)$:
  - $S_t$: repository working state at commit/revision.
  - $C_t \subseteq \mathcal{M}$: context package injected by ContextOS.
  - $q$: original task intent.
  - $\mathcal{H}_t$: history of previous tool calls and outputs.
- Each directed edge $e = (u, v) \in \mathcal{E}$ represents an action $a \in \mathcal{A}$ (e.g. read file, execute unit test, write code diff, backtrack) with observed transition outcome $o \in \mathcal{O}$ and token cost $\kappa(a)$.
- Terminal nodes $v_{\text{term}} \in \mathcal{V}_{\text{leaf}}$ carry a verification outcome $r(v_{\text{term}}) \in \{0, 1\}$ (test pass/fail, build status, oracle rubric score).

### 4.2 The Parameterized Exploration Policy $\pi_\theta$
The exploration policy $\pi_\theta$ governs the context planning, branch prioritization, and candidate pruning decisions:

$$\theta = \Big( \mathbf{w}_{\text{alloc}}, \, \tau_{\text{prune}}, \, \gamma_{\text{backtrack}}, \, \mathbf{w}_{\text{stages}}, \, \beta_{\text{budget}} \Big)$$

Where:
1. $\mathbf{w}_{\text{alloc}} = (w_{\text{sem}}, w_{\text{lex}}, w_{\text{graph}}, w_{\text{fresh}}, w_{\text{auth}}, w_{\text{conf}}, w_{\text{reuse}}, w_{\text{density}})$: weight vector for the 6-pass ASC-1 knapsack scorer.
2. $\tau_{\text{prune}} \in [0, 1]$: minimum marginal utility threshold below which context candidates and search subtrees are immediately pruned.
3. $\gamma_{\text{backtrack}} \in [0, 1]$: threshold for triggering early backtracking upon consecutive test/verification failures.
4. $\mathbf{w}_{\text{stages}}$: relative priority and early-termination cutoff across the 7 retrieval stages (`exact_path`, `symbol_ast`, `entity`, `bm25`, `semantic_hash`, `graph_expansion`, `rrf_rerank`).
5. $\beta_{\text{budget}}$: ratio of exploration vs. exploitation budget in multi-turn tasks.

### 4.3 The "Dream" Optimization Objective
In the offline replay simulator ("Evolving World") $\mathcal{W}_{\text{sim}}$, we replay the historical task corpus $\mathcal{D} = \{\mathcal{T}_1, \mathcal{T}_2, \dots, \mathcal{T}_N\}$ under counterfactual policy parameters $\theta$:

$$\theta^* = \arg\max_{\theta \in \Theta} \sum_{i=1}^N \left[ \mathbb{E}_{\mathcal{W}_{\text{sim}}}\left[ \mathcal{Q}(\mathcal{T}_i \mid \pi_\theta) \right] - \lambda \cdot \text{Cost}(\mathcal{T}_i \mid \pi_\theta) - \mu \cdot \text{Steps}(\mathcal{T}_i \mid \pi_\theta) \right]$$

Subject to the non-regression constraint:

$$\forall \mathcal{T}_k \in \mathcal{D}_{\text{benchmark}}, \quad \text{Success}(\mathcal{T}_k \mid \pi_\theta) \ge \text{Success}(\mathcal{T}_k \mid \pi_{\text{baseline}})$$

Because counterfactual policy evaluation runs over pre-recorded tool outcomes and AST dependency graphs in the replay simulator, thousands of permutations $\theta$ are evaluated in seconds with zero LLM API costs.

---

## 5. Core Subsystem Components

### 5.1 Trajectory Tree Ingestor (`internal/dreamrsi/trajectory.go`)
Converts raw telemetry from `internal/hook` into structured search trees:
- Groups events by `SessionID` and `WorkItem`.
- Infers branch divergence: identifies when an agent encountered an error, reverted an edit, or initiated a secondary search query.
- Annotates every decision point with the candidates considered, the context allocated, and the subsequent tool outcome.

### 5.2 The Replay World Simulator (`internal/dreamrsi/simulator.go`)
Implements the "Evolving World" abstraction from the paper:
- Uses Git commit snapshots and `internal/temporal` scoped staleness to reproduce repository state at step $t$.
- Caches tool invocation responses (e.g. compiler outputs, test execution results, LSP responses).
- Simulates counterfactual allocations: computes what context would have been selected under candidate weights $\theta'$ and whether critical evidence tokens were retained.

### 5.3 Offline Policy Mutator & Searcher (`internal/dreamrsi/optimizer.go`)
Explores the policy space $\Theta$ using a combination of:
- **CMA-ES / Evolutionary Mutation**: Perturbs continuous weight parameters $\mathbf{w}_{\text{alloc}}$.
- **Simulated Annealing on Stage Orderings**: Optimizes the execution sequence and early-exit thresholds of hybrid retrieval stages.
- **Pareto-Frontier Tracking**: Identifies policies that maximize task success while minimizing injected tokens.

### 5.4 The Gatekeeper (`internal/dreamrsi/gatekeeper.go`)
Enforces scientific rigor before deploying any newly discovered policy into production:
- Employs `internal/bench/stats.go`:
  - **Paired Bootstrap Confidence Intervals** (95% CI on mean token delta).
  - **Permutation Tests** ($p < 0.05$ significance threshold against baseline).
  - **Cliff’s Delta** ($\delta \ge 0.30$ to ensure non-trivial effect size).
- Guarantees zero regressions on authoritative benchmark suites.

### 5.5 Runtime Policy Profile Hot-Reload (`internal/allocator/profile.go`)
- Enables ContextOS to decouple hardcoded weights from `internal/allocator/allocator.go`.
- Dynamic profiles can be scoped globally, per-repository, or per-agent-type.

---

## 6. Data Structures (Go Specification)

```go
package dreamrsi

import (
	"time"
	"contextos/internal/model"
)

// TrajectoryNode represents a single decision state in an agent's exploration tree.
type TrajectoryNode struct {
	ID             string              `json:"id"`
	ParentID       string              `json:"parent_id,omitempty"`
	Timestamp      time.Time           `json:"timestamp"`
	Task           string              `json:"task"`
	RepoRevision   string              `json:"repo_revision"`
	ContextPlan    model.ContextPlan   `json:"context_plan"`
	ActionTaken    string              `json:"action_taken"`
	ToolName       string              `json:"tool_name,omitempty"`
	ToolOutput     string              `json:"tool_output,omitempty"`
	IsFailure      bool                `json:"is_failure"`
	IsBacktrack    bool                `json:"is_backtrack"`
	BranchDepth    int                 `json:"branch_depth"`
	TerminalResult *model.Result       `json:"terminal_result,omitempty"`
}

// ExplorationPolicy defines the tunable orchestration parameters optimized by Dream-RSI.
type ExplorationPolicy struct {
	ID                  string             `json:"id"`
	Version             int                `json:"version"`
	
	// ASC-1 Scorer Weight Vector
	WeightSemantic      float64            `json:"weight_semantic"`
	WeightLexical       float64            `json:"weight_lexical"`
	WeightGraph         float64            `json:"weight_graph"`
	WeightFreshness     float64            `json:"weight_freshness"`
	WeightAuthority     float64            `json:"weight_authority"`
	WeightConfidence    float64            `json:"weight_confidence"`
	WeightReuse         float64            `json:"weight_reuse"`
	WeightStaleRisk     float64            `json:"weight_stale_risk"`
	
	// Dynamic Pruning & Exploration Parameters
	PruneThreshold      float64            `json:"prune_threshold"`
	BacktrackPenalty    float64            `json:"backtrack_penalty"`
	StageEarlyExitK     int                `json:"stage_early_exit_k"`
	RetrievalStages     []string           `json:"retrieval_stages"`
}

// DreamOptimizationReport captures the outcome of an offline policy refinement cycle.
type DreamOptimizationReport struct {
	CycleID             string             `json:"cycle_id"`
	SimulatedTasks      int                `json:"simulated_tasks"`
	SimulatedPolicies   int                `json:"simulated_policies"`
	BaselinePolicyID    string             `json:"baseline_policy_id"`
	WinningPolicyID     string             `json:"winning_policy_id"`
	TokenSavingsPct     float64            `json:"token_savings_pct"`
	SuccessRateDelta    float64            `json:"success_rate_delta"`
	PermutationPValue   float64            `json:"permutation_p_value"`
	CliffsDelta         float64            `json:"cliffs_delta"`
	PromotedToActive    bool               `json:"promoted_to_active"`
	CompletedAt         time.Time          `json:"completed_at"`
}
```

---

## 7. Execution CLI & MCP Integration

To operate Dream-RSI within ContextOS:

### CLI Command Surface
```bash
# 1. Ingest session logs into the offline trajectory tree
ctx dream ingest --repo . --since 7d

# 2. Run the offline dreaming optimization loop over past trajectories
ctx dream optimize --repo . --cycles 500 --workers 8 --output dream_report.json

# 3. Inspect active self-improved policy parameters
ctx dream status --repo .

# 4. Rollback to default deterministic ASC-1 baseline if desired
ctx dream reset --repo .
```

### MCP Tool Surface
ContextOS exposes Dream-RSI capabilities to IDE agents via:
- `context_dream_status`: Inspects active exploration policy and generation count.
- `context_dream_checkpoint`: Commits current session trajectory as an authoritative exploration tree node.
- `context_dream_evaluate`: Evaluates candidate context configurations against historical replay benchmarks.

---

## 8. Summary of Benefits

1. **Self-Optimizing Without Model Retraining**: ContextOS adapts to any project, architecture, or tech stack completely on client machines or local clusters.
2. **Deterministic & Safe**: Mutations are tested against historical replay worlds, preventing hallucinated or broken context policies from hitting live agents.
3. **Compound Cost Reductions**: Synergizes with ASC-1 6-pass knapsack allocation and KV-cache stable prefixing to continually drive Cost-Per-Success (CPS) lower with each completed task.
