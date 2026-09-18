# ContextOS provider integrations

ContextOS uses a provider-neutral local MCP server plus provider-specific hook installation.

## Claude Code

`ctx setup` installs project-local `.mcp.json` and `.claude/settings.local.json`.
Claude Code hooks can inject additional context through hook-specific output on supported lifecycle events.

## Cursor

`ctx setup` installs `.cursor/mcp.json`, `.cursor/hooks.json`, and `.cursor/rules/contextos.mdc`.
- **MCP**: Registered under `.cursor/mcp.json` connecting directly to `contextd` stdio.
- **Hooks**: Supports `sessionStart`, `beforeSubmitPrompt`, `postToolUse`, `postToolUseFailure`, `stop`, and `workspaceOpen`. On `beforeSubmitPrompt`, ContextOS injects active budget-bounded context into `additional_context` with `{"continue": true}` so prompts execute with full background state.
- **Rules**: `.cursor/rules/contextos.mdc` instructs Cursor models to leverage `context_resume` at the start of tasks, plan context budgets with `context_plan`, and persist decisions/failures via `context_remember`.

## Codex

`ctx setup` installs `.codex/config.toml` and `.codex/hooks.json`.
The project MCP server is local and the durable event capture path is enabled. Hook output is kept conservative because supported hook output fields can vary between Codex releases.

## Gemini CLI

`ctx setup` installs `.gemini/settings.json` containing both the MCP server and hook configuration.
Gemini `SessionStart`/`BeforeAgent` hooks are used for context injection where supported.

## Antigravity

`ctx setup` installs `.agents/mcp_config.json`, `.agents/hooks.json`, and `.agents/rules/contextos.md`.
- **MCP**: Configured in `.agents/mcp_config.json` running `contextd -repo . -mcp` via stdio.
- **Hooks**: Defined in `.agents/hooks.json`:
  - `PreInvocation`: Computes a token-budgeted context plan for the incoming prompt/active work item and injects ephemeral context steps via `{"injectSteps": [{"ephemeralMessage": "..."}]}`.
  - `PostToolUse`: Captures tool output, successes, and failures using wildcard matcher (`"matcher": "*"`).
  - `Stop`: Inspects run termination status and permits graceful completion with `{"decision": "allow"}`.
- **Rules**: `.agents/rules/contextos.md` provides explicit guidance to the agent runtime on invoking `context_resume` to restore past decisions, `context_plan` to construct minimum-sufficient context, and `context_remember` to record decisions and dead ends.

## Portable fallback

All five providers (Claude Code, Cursor, Codex, Gemini CLI, Antigravity) can use the same underlying server manually:

```bash
contextd -repo /path/to/repository -mcp
```

The core memory/state format does not depend on a specific model provider.
