# ContextOS Rules for Antigravity

This repository uses ContextOS for persistent, token-bounded context management.

## Guidelines
- Call `context_resume` at the beginning of a task to restore previous decisions and active work state.
- Use `context_plan` to assemble minimum-sufficient context for complex coding tasks under budget constraints.
- Record durable architectural choices with `context_remember` (kind: "decision", authority: "user").
- Record failed approaches or dead ends with `context_remember` (kind: "failure") so future sessions avoid repeating mistakes.
- Use `context_handoff` when transferring engineering state to another agent or model.
