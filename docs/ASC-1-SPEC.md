# ASC-1: Adaptive Sufficient Context

## Goal
Minimize the cost/latency of context supplied to a software-engineering agent while maintaining a target probability of task success.

## Core optimization

For task $q$, state $S$, model $M$, budget $B$:

$$
\begin{aligned}
\max_{C, M} \quad & \mathcal{U}(C, M \mid q, S) \\
\text{subject to} \quad & \text{Tokens}(C) \le B \quad \text{and} \quad P(\text{success} \mid C, M, q, S) \ge \tau
\end{aligned}
$$

where:

$$
\mathcal{U} = \mathcal{Q}(C, M) + \eta \cdot \text{CacheReuse}(C, M) - \lambda \cdot \text{Cost}(C, M) - \mu \cdot \text{Latency}(C) - \rho \cdot \text{StaleRisk}(C)
$$

## State
S = (repo, revision, work_item, trajectory, memory_graph, cache_state, provider_state)

## Memory object
Memory = {
  id,
  kind: fact | decision | constraint | failure | state | preference,
  content,
  scope: repo/work-item/global,
  valid_from_revision,
  invalidated_at_revision,
  authority: user | source | test | commit | doc | inference,
  confidence,
  provenance[],
  token_cost,
  embedding,
  graph_edges[],
  access_stats
}

## Candidate sources
- current code/symbol slices
- AST/dependency graph
- Git history and diffs
- work-item state
- prior session trajectory
- durable memories
- decisions/failures
- tests and evidence

## Pipeline
1. Detect task + repository + revision.
2. Decide whether retrieval has positive expected value.
3. Scope to repo/branch/work-item.
4. Filter temporally invalid and low-authority candidates.
5. Localize via repository/engineering graph.
6. Hybrid retrieve candidates.
7. Estimate marginal utility per token.
8. Select a coherent subset under token budget.
9. Compress only low-density material.
10. Arrange stable prefix before volatile suffix.
11. Apply cache-aware routing/model choice.
12. Execute agent task.
13. Record outcome, memory use, cache state, and task result.
14. Update utility/cache/prefetch statistics.

## Deterministic vs learned
Deterministic: git revisions, file/symbol existence, dependency edges, hashes, provenance, obvious invalidation.
Learned: semantic relevance, marginal utility, compression choice, prefetch probability, model choice.

## v0 scoring baseline

$$
\text{Score}(m) = 1.0 \cdot \text{Semantic} + 1.2 \cdot \text{Graph} + 0.8 \cdot \text{Freshness} + 1.0 \cdot \text{Authority} + 0.8 \cdot \text{Reuse} + 0.8 \cdot \text{TaskAffinity} - 1.0 \cdot \text{StaleRisk} - 0.3 \cdot \text{Tokens}_{\text{norm}}
$$

Use cost-scaled greedy selection as a baseline. Later replace score with learned marginal-utility estimates.

## Cache levels
L0 artifact cache: AST, graph, embeddings, parsed docs.
L1 memory cache: high-reuse claims/decisions/failures.
L2 context bundle cache: task+state -> selected context.
L3 provider prefix cache: stable prompt prefix.

## v0 MCP surface
Resources:
- context://repo/current
- context://work-item/current
- context://memory/relevant
- context://decision/{id}
- context://failure/{id}
- context://session/latest

Tools:
- context_search
- context_plan
- context_resume
- context_handoff
- context_remember
- context_invalidate
- context_trace
- context_stats

## Acceptance criteria
A v0 is implementation-ready when it can:
- persist across restart
- survive agent/model changes
- detect repo/revision context
- preserve evidence and provenance
- avoid stale claims
- select within a token budget
- expose a trace showing why context was selected/rejected
- report token/cost/cache metrics
