# ContextOS Rules for Antigravity

This repository uses ContextOS for persistent, token-bounded context management, adaptive compute routing, and deterministic retrieval.

## Core Directives

1. **Session Initialization**:
   - Call `context_resume` at the start of a task or session to recover uncommitted changes, active branch state, open work items, and high-authority engineering decisions.

2. **Context Planning & Deterministic Retrieval (R17)**:
   - Use `context_plan` to assemble minimum-sufficient context under strict token budgets.
   - For exact file or symbol lookups, prefix or pass explicit paths (e.g. `internal/retrieval/planner.go` or `planner.go`). ContextOS R17 provides quotient-space candidate canonicalization and dedicated exact indexes (`LookupExactPath`, `LookupBasename`), guaranteeing recall completeness without heuristic degradation.
   - Respect the planner soundness guarantee: satisfied evidence states ensure all target nodes are strictly bounded within the token budget.

3. **Adaptive Compute & Model Routing (R16)**:
   - Use `context_compute_plan` or `context_route` before complex multi-step reasoning to evaluate task complexity, recommended token budgets, reasoning effort, and verification tiers (T0 deterministic AST bypass, T1 cached context plan, T2 full graph traversal).

4. **Durable Knowledge Lifecycle**:
   - Persist critical architectural choices with `context_remember` (`kind: "decision"`, `authority: "user"` or `"verified"`).
   - Persist dead ends and anti-patterns with `context_remember` (`kind: "failure"`) to prevent subsequent agent loops from repeating discarded approaches.
   - When a previous decision or assumption is superseded by code changes, invalidate it using `context_invalidate` with its memory ID. ContextOS anchors invalidations to git commit revisions, preserving historical fidelity.

5. **Cross-Agent Handoff**:
   - Use `context_handoff` when transferring engineering state, active tasks, and context budgets to another agent, subagent, or model family.

