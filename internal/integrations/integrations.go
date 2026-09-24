package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InstallResult struct {
	Agent  string   `json:"agent"`
	Files  []string `json:"files"`
	Action string   `json:"action"`
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func hookCommand(hookBinary, agent, event string) string {
	return fmt.Sprintf("%s -agent %s -event %s", shellQuote(hookBinary), shellQuote(agent), shellQuote(event))
}

// mergeMapFile reads an existing JSON file, structurally merges the new entries,
// and writes the result back. It uses identity-based dedup for arrays
// (matching on "command" or "name" keys where available) so that repeated
// installations do not duplicate ContextOS entries while preserving
// unrelated user configuration.
func mergeMapFile(path string, base map[string]any) error {
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		var existing map[string]any
		if json.Unmarshal(b, &existing) == nil {
			base = deepMerge(existing, base)
		}
	}
	b, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func deepMerge(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				dst[k] = deepMerge(dv, sv)
			} else {
				dst[k] = sv
			}
		} else if sa, ok := v.([]any); ok {
			da, _ := dst[k].([]any)
			for _, x := range sa {
				if !containsJSONByIdentity(da, x) {
					da = append(da, x)
				}
			}
			dst[k] = da
		} else {
			dst[k] = v
		}
	}
	return dst
}

// containsJSONByIdentity uses stable identity fields (command, name)
// to detect existing installations rather than full JSON comparison.
// This is structurally aware per PR-01 Step 5.
func containsJSONByIdentity(arr []any, needle any) bool {
	nID := identityKey(needle)
	if nID != "" {
		for _, x := range arr {
			if identityKey(x) == nID {
				return true
			}
		}
		return false
	}
	// Fallback to full JSON comparison for non-map items
	a, _ := json.Marshal(needle)
	for _, x := range arr {
		b, _ := json.Marshal(x)
		if string(a) == string(b) {
			return true
		}
	}
	return false
}

// identityKey extracts a stable identity from a map for dedup purposes.
// Uses hook.command or server.name or mcp.server.name as the identity.
func identityKey(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	// Check for "command" key (hook identity)
	if cmd, ok := m["command"].(string); ok && strings.Contains(cmd, "ctx-hook") {
		return "hook:" + cmd
	}
	// Check for nested hooks with command
	if hooks, ok := m["hooks"].([]any); ok && len(hooks) > 0 {
		if hm, ok := hooks[0].(map[string]any); ok {
			if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "ctx-hook") {
				return "hook:" + cmd
			}
		}
	}
	// Check for "name" key (server/tool identity)
	if name, ok := m["name"].(string); ok {
		return "name:" + name
	}
	return ""
}

func mcpJSON(command, repoPlaceholder string) map[string]any {
	return map[string]any{
		"_managedBy": ManagedBy,
		"_version":   ConfigVersion,
		"mcpServers": map[string]any{
			"contextos": map[string]any{
				"type":    "stdio",
				"command": command,
				"args":    []any{"-repo", repoPlaceholder, "-mcp"},
			},
		},
	}
}

func mcpTOML(command, repoPlaceholder string) string {
	return fmt.Sprintf("\n[mcp_servers.contextos]\ncommand = %s\nargs = [\"-repo\", %s, \"-mcp\"]\n", tomlString(command), tomlString(repoPlaceholder))
}

func tomlString(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func ensureTomlSection(path, section string) error {
	b, _ := os.ReadFile(path)
	text := string(b)
	if strings.Contains(text, "[mcp_servers.contextos]") {
		return nil
	}
	text += section
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0600)
}

// Install configures agent integrations (hooks, MCP, rules) for the given agent.
// Binary paths can be:
//   - Absolute paths (backward compatible)
//   - Relative paths (resolved from repoRoot)
//   - Bare names (resolved via ResolveBinary)
//
// The installation is idempotent: Install(Install(R)) = Install(R).
// Existing unrelated configuration is preserved.
func Install(agent, repoRoot, hookBinary, mcpBinary string) (InstallResult, error) {
	repoRoot, _ = filepath.Abs(repoRoot)

	// Resolve binaries portably — don't require absolute paths
	var err error
	hookBinary, err = resolveBinaryPath(hookBinary, repoRoot)
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve hook binary: %w", err)
	}
	mcpBinary, err = resolveBinaryPath(mcpBinary, repoRoot)
	if err != nil {
		return InstallResult{}, fmt.Errorf("resolve mcp binary: %w", err)
	}

	switch agent {
	case "claude":
		hookPath := filepath.Join(repoRoot, ".claude", "settings.local.json")
		group := func(event string) map[string]any {
			return map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand(hookBinary, agent, event)}}}
		}
		hooks := map[string]any{"SessionStart": []any{group("SessionStart")}, "UserPromptSubmit": []any{group("UserPromptSubmit")}, "PostToolUse": []any{group("PostToolUse")}, "PostToolUseFailure": []any{group("PostToolUseFailure")}, "Stop": []any{group("Stop")}, "SessionEnd": []any{group("SessionEnd")}}
		base := map[string]any{
			"_managedBy": ManagedBy,
			"_version":   ConfigVersion,
			"hooks":      hooks,
		}
		if err := mergeMapFile(hookPath, base); err != nil {
			return InstallResult{}, err
		}
		mcpPath := filepath.Join(repoRoot, ".mcp.json")
		if err := mergeMapFile(mcpPath, mcpJSON(mcpBinary, ".")); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Agent: agent, Files: []string{hookPath, mcpPath}, Action: "installed Claude Code hooks + MCP"}, nil
	case "cursor":
		hookPath := filepath.Join(repoRoot, ".cursor", "hooks.json")
		arr := func(event string) []any {
			return []any{map[string]any{"command": hookCommand(hookBinary, agent, event)}}
		}
		hooks := map[string]any{"sessionStart": arr("sessionStart"), "beforeSubmitPrompt": arr("beforeSubmitPrompt"), "postToolUse": arr("postToolUse"), "postToolUseFailure": arr("postToolUseFailure"), "stop": arr("stop"), "workspaceOpen": arr("workspaceOpen")}
		base := map[string]any{
			"_managedBy": ManagedBy,
			"_version":   ConfigVersion,
			"version":    1,
			"hooks":      hooks,
		}
		if err := mergeMapFile(hookPath, base); err != nil {
			return InstallResult{}, err
		}
		mcpPath := filepath.Join(repoRoot, ".cursor", "mcp.json")
		if err := mergeMapFile(mcpPath, mcpJSON(mcpBinary, ".")); err != nil {
			return InstallResult{}, err
		}
		rulePath := filepath.Join(repoRoot, ".cursor", "rules", "contextos.mdc")
		cursorRule := `---
description: ContextOS persistent context runtime
globs: *
alwaysApply: true
---

# ContextOS Integration for Cursor

This project uses ContextOS for persistent, budget-aware context management.

- Call ` + "`context_resume`" + ` when resuming work to recover active work items and past context.
- Use ` + "`context_plan`" + ` to build token-budgeted context packages before executing complex tasks.
- Persist architectural decisions and constraints with ` + "`context_remember`" + ` (kind: "decision" or "constraint").
- Document negative knowledge / failed approaches with ` + "`context_remember`" + ` (kind: "failure").
- Use ` + "`context_handoff`" + ` for cross-model or cross-session context transfers.
`
		_ = os.MkdirAll(filepath.Dir(rulePath), 0700)
		_ = os.WriteFile(rulePath, []byte(cursorRule), 0644)
		return InstallResult{Agent: agent, Files: []string{hookPath, mcpPath, rulePath}, Action: "installed Cursor hooks + MCP + rules"}, nil
	case "antigravity", "agy":
		mcpPath := filepath.Join(repoRoot, ".agents", "mcp_config.json")
		if err := mergeMapFile(mcpPath, map[string]any{
			"_managedBy": ManagedBy,
			"_version":   ConfigVersion,
			"mcpServers": map[string]any{
				"contextos": map[string]any{
					"command": mcpBinary,
					"args":    []any{"-repo", repoRoot, "-mcp"},
				},
			},
		}); err != nil {
			return InstallResult{}, err
		}
		hookPath := filepath.Join(repoRoot, ".agents", "hooks.json")
		hookGroup := func(event string) []any {
			return []any{map[string]any{
				"matcher": "*",
				"hooks": []any{
					map[string]any{
						"type":    "command",
						"command": hookCommand(hookBinary, "antigravity", event),
						"timeout": 15,
					},
				},
			}}
		}
		flatHook := func(event string) []any {
			return []any{map[string]any{
				"type":    "command",
				"command": hookCommand(hookBinary, "antigravity", event),
				"timeout": 15,
			}}
		}
		hooksCfg := map[string]any{
			"_managedBy": ManagedBy,
			"_version":   ConfigVersion,
			"contextos": map[string]any{
				"PreInvocation": flatHook("PreInvocation"),
				"PostToolUse":   hookGroup("PostToolUse"),
				"Stop":          flatHook("Stop"),
			},
		}
		if err := mergeMapFile(hookPath, hooksCfg); err != nil {
			return InstallResult{}, err
		}
		rulePath := filepath.Join(repoRoot, ".agents", "rules", "contextos.md")
		ruleContent := `# ContextOS Rules for Antigravity

This repository uses ContextOS for persistent, token-bounded context management, adaptive compute routing, and deterministic retrieval.

## Core Directives

1. **Session Initialization**:
   - Call ` + "`context_resume`" + ` at the start of a task or session to recover uncommitted changes, active branch state, open work items, and high-authority engineering decisions.

2. **Context Planning & Deterministic Retrieval (R17)**:
   - Use ` + "`context_plan`" + ` to assemble minimum-sufficient context under strict token budgets.
   - For exact file or symbol lookups, prefix or pass explicit paths (e.g. ` + "`internal/retrieval/planner.go`" + ` or ` + "`planner.go`" + `). ContextOS R17 provides quotient-space candidate canonicalization and dedicated exact indexes (` + "`LookupExactPath`" + `, ` + "`LookupBasename`" + `), guaranteeing recall completeness without heuristic degradation.
   - Respect the planner soundness guarantee: satisfied evidence states ensure all target nodes are strictly bounded within the token budget.

3. **Adaptive Compute & Model Routing (R16)**:
   - Use ` + "`context_compute_plan`" + ` or ` + "`context_route`" + ` before complex multi-step reasoning to evaluate task complexity, recommended token budgets, reasoning effort, and verification tiers (T0 deterministic AST bypass, T1 cached context plan, T2 full graph traversal).

4. **Durable Knowledge Lifecycle**:
   - Persist critical architectural choices with ` + "`context_remember`" + ` (kind: "decision", authority: "user" or "verified").
   - Persist dead ends and anti-patterns with ` + "`context_remember`" + ` (kind: "failure") so future sessions avoid repeating mistakes.
   - When a previous decision or assumption is superseded by code changes, invalidate it using ` + "`context_invalidate`" + ` with its memory ID. ContextOS anchors invalidations to git commit revisions, preserving historical fidelity.

5. **Cross-Agent Handoff**:
   - Use ` + "`context_handoff`" + ` when transferring engineering state to another agent or model.
`
		_ = os.MkdirAll(filepath.Dir(rulePath), 0700)
		_ = os.WriteFile(rulePath, []byte(ruleContent), 0644)

		skillPath := filepath.Join(repoRoot, ".agents", "skills", "contextos", "SKILL.md")
		skillContent := `---
name: contextos
description: Use ContextOS to manage persistent agent context, execute 6-pass token allocations, recall durable engineering decisions, generate adaptive compute plans, and record session traces.
---

# ContextOS Agent Workflow & Reference

ContextOS provides deterministic, budget-bounded context management, adaptive compute routing, and persistent engineering memory for autonomous AI agents.

## Core Capabilities (R16 & R17)

- **Quotient-Space Canonical Invariance (Theorem 1)**: Candidates are normalized into canonical equivalence classes [x] in C/~ across path representations and symbol signatures, preventing score dilution and duplicate token waste.
- **Exact Path & Basename Retrieval (Theorem 2)**: Querying explicit paths (e.g. ` + "`internal/retrieval/planner.go`" + `) or basenames (` + "`planner.go`" + `) triggers dedicated exact indexes (` + "`LookupExactPath`" + `, ` + "`LookupBasename`" + `), ensuring complete 100% recall without graph heuristic distortion.
- **Planner Soundness State Machine (Theorem 3)**: Monotonic state transitions (EvidenceUnevaluated -> EvidenceSatisfied | EvidenceBudgetExhausted | EvidenceContradiction) mathematically guarantee that satisfied plans strictly bound target nodes within the token budget.
- **Adaptive Compute Planning (R16)**: Computes provider-neutral reasoning effort, token budgets, and verification tiers (T0 AST bypass, T1 cached plan, T2 full graph search) for optimal cost-accuracy tradeoffs across Gemini, Claude, and local models.
- **Two-Tier Cache Architecture**: L1 in-memory LRU cache + L2 persistent SQLite cache with prefix hash matching, delivering 70%+ token savings on repetitive tool invocations.

## CLI Quick Access

` + "```bash" + `
# Resume latest branch state, uncommitted changes, and active decisions
./bin/ctx resume -repo .

# Plan minimum-sufficient context under a strict token budget (e.g., 2000 tokens)
./bin/ctx plan -repo . -task "Fix issue" -budget 2000

# Remember key decisions or bug fixes
./bin/ctx remember -repo . -kind decision -authority user -content "Decision..."

# View allocation traces and cache token savings
./bin/ctx stats -repo .
` + "```" + `
`
		_ = os.MkdirAll(filepath.Dir(skillPath), 0700)
		_ = os.WriteFile(skillPath, []byte(skillContent), 0644)

		return InstallResult{
			Agent:  agent,
			Files:  []string{mcpPath, hookPath, rulePath, skillPath},
			Action: "installed Antigravity hooks + MCP + rules + skill",
		}, nil
	case "codex":
		hookPath := filepath.Join(repoRoot, ".codex", "hooks.json")
		grp := func(event string) []any {
			return []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand(hookBinary, agent, event)}}}}
		}
		hooks := map[string]any{"SessionStart": grp("SessionStart"), "UserPromptSubmit": grp("UserPromptSubmit"), "PreToolUse": grp("PreToolUse"), "PostToolUse": grp("PostToolUse"), "PostToolUseFailure": grp("PostToolUseFailure"), "Stop": grp("Stop")}
		base := map[string]any{
			"_managedBy": ManagedBy,
			"_version":   ConfigVersion,
			"hooks":      hooks,
		}
		if err := mergeMapFile(hookPath, base); err != nil {
			return InstallResult{}, err
		}
		mcpPath := filepath.Join(repoRoot, ".codex", "config.toml")
		if err := ensureTomlSection(mcpPath, mcpTOML(mcpBinary, repoRoot)); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Agent: agent, Files: []string{hookPath, mcpPath}, Action: "installed Codex hooks + project MCP"}, nil
	case "gemini":
		p := filepath.Join(repoRoot, ".gemini", "settings.json")
		grp := func(event string) []any {
			return []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "name": "contextos-" + event, "command": hookCommand(hookBinary, agent, event), "timeout": 5000}}}}
		}
		hooks := map[string]any{"SessionStart": grp("SessionStart"), "BeforeAgent": grp("BeforeAgent"), "BeforeTool": grp("BeforeTool"), "AfterTool": grp("AfterTool"), "AfterAgent": grp("AfterAgent"), "SessionEnd": grp("SessionEnd")}
		base := map[string]any{
			"_managedBy": ManagedBy,
			"_version":   ConfigVersion,
			"hooks":      hooks,
			"mcpServers": mcpJSON(mcpBinary, ".")["mcpServers"],
		}
		if err := mergeMapFile(p, base); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Agent: agent, Files: []string{p}, Action: "installed Gemini CLI hooks + MCP"}, nil
	default:
		return InstallResult{}, fmt.Errorf("unsupported agent %q", agent)
	}
}

// resolveBinaryPath resolves a binary path portably.
// If the path is already absolute, it is returned as-is.
// If it's a bare name, ResolveBinary is used for lookup.
// If it's a relative path, it's resolved from repoRoot.
func resolveBinaryPath(binary, repoRoot string) (string, error) {
	if filepath.IsAbs(binary) {
		return binary, nil
	}
	// Check if it looks like a relative path (contains separator)
	if strings.Contains(binary, string(filepath.Separator)) || strings.Contains(binary, "/") {
		abs, err := filepath.Abs(filepath.Join(repoRoot, binary))
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	// Bare name — use portable resolution
	resolved, err := ResolveBinary(binary, repoRoot)
	if err != nil {
		// Fall back to using the name as-is (for testing / PATH availability later)
		return binary, nil
	}
	return resolved, nil
}
