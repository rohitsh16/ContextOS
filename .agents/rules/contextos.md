# ContextOS Agent Guidelines

This project uses ContextOS to manage agent memory, context budgets, and git synchronization.

## How to Work with ContextOS

1. **Start of Session**:
   Call `context_resume` at the beginning of a task to see active branch state, uncommitted changes, open work items, and past architectural decisions.

2. **Planning Context**:
   Use `context_plan` to assemble only the code symbols and memories needed for your task under a strict token budget.
   If you know the specific file or symbol you need, include the path (e.g., `internal/allocator/allocator.go`) in the task query for instant exact matching.

3. **Routing & Budgets**:
   Use `context_route` or `context_compute_plan` before complex multi-step reasoning to get a recommended token budget and reasoning effort level.

4. **Saving Knowledge**:
   - Save key architectural choices with `context_remember` (`kind: "decision"`).
   - Save bugs, failed attempts, and gotchas with `context_remember` (`kind: "failure"`) so future sessions don't repeat them.
   - If an older decision is superseded by code changes, invalidate it with `context_invalidate`.

5. **Passing Context to Another Agent**:
   Use `context_handoff` when handing off work to another agent or subagent so it receives a clean, focused summary of the task state.
