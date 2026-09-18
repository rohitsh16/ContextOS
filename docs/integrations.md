# ContextOS provider integrations

ContextOS uses a provider-neutral local MCP server plus provider-specific hook installation.

## Claude Code

`ctx setup` installs project-local `.mcp.json` and `.claude/settings.local.json`.
Claude Code hooks can inject additional context through hook-specific output on supported lifecycle events.

## Cursor

`ctx setup` installs `.cursor/mcp.json` and `.cursor/hooks.json`.
Cursor session-start/failure hooks can return additional context; `beforeSubmitPrompt` is used for capture/continuation because its response contract does not provide the same arbitrary context field.

## Codex

`ctx setup` installs `.codex/config.toml` and `.codex/hooks.json`.
The project MCP server is local and the durable event capture path is enabled. Hook output is kept conservative because supported hook output fields can vary between Codex releases.

## Gemini CLI

`ctx setup` installs `.gemini/settings.json` containing both the MCP server and hook configuration.
Gemini `SessionStart`/`BeforeAgent` hooks are used for context injection where supported.

## Portable fallback

All four providers can use the same underlying server manually:

```bash
contextd -repo /path/to/repository -mcp
```

The core memory/state format does not depend on a specific model provider.
