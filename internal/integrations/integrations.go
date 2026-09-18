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
				if !containsJSON(da, x) {
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

func containsJSON(arr []any, needle any) bool {
	a, _ := json.Marshal(needle)
	for _, x := range arr {
		b, _ := json.Marshal(x)
		if string(a) == string(b) {
			return true
		}
	}
	return false
}

func mcpJSON(command, repoPlaceholder string) map[string]any {
	return map[string]any{"mcpServers": map[string]any{
		"contextos": map[string]any{
			"type":    "stdio",
			"command": command,
			"args":    []any{"-repo", repoPlaceholder, "-mcp"},
		},
	}}
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

func Install(agent, repoRoot, hookBinary, mcpBinary string) (InstallResult, error) {
	repoRoot, _ = filepath.Abs(repoRoot)
	hookBinary, _ = filepath.Abs(hookBinary)
	mcpBinary, _ = filepath.Abs(mcpBinary)
	switch agent {
	case "claude":
		hookPath := filepath.Join(repoRoot, ".claude", "settings.local.json")
		group := func(event string) map[string]any {
			return map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand(hookBinary, agent, event)}}}
		}
		hooks := map[string]any{"SessionStart": []any{group("SessionStart")}, "UserPromptSubmit": []any{group("UserPromptSubmit")}, "PostToolUse": []any{group("PostToolUse")}, "PostToolUseFailure": []any{group("PostToolUseFailure")}, "Stop": []any{group("Stop")}, "SessionEnd": []any{group("SessionEnd")}}
		if err := mergeMapFile(hookPath, map[string]any{"hooks": hooks}); err != nil {
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
		if err := mergeMapFile(hookPath, map[string]any{"version": 1, "hooks": hooks}); err != nil {
			return InstallResult{}, err
		}
		mcpPath := filepath.Join(repoRoot, ".cursor", "mcp.json")
		if err := mergeMapFile(mcpPath, mcpJSON(mcpBinary, ".")); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Agent: agent, Files: []string{hookPath, mcpPath}, Action: "installed Cursor hooks + MCP"}, nil
	case "codex":
		hookPath := filepath.Join(repoRoot, ".codex", "hooks.json")
		grp := func(event string) []any {
			return []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand(hookBinary, agent, event)}}}}
		}
		hooks := map[string]any{"SessionStart": grp("SessionStart"), "UserPromptSubmit": grp("UserPromptSubmit"), "PreToolUse": grp("PreToolUse"), "PostToolUse": grp("PostToolUse"), "PostToolUseFailure": grp("PostToolUseFailure"), "Stop": grp("Stop")}
		if err := mergeMapFile(hookPath, map[string]any{"hooks": hooks}); err != nil {
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
		base := map[string]any{"hooks": hooks, "mcpServers": mcpJSON(mcpBinary, ".")["mcpServers"]}
		if err := mergeMapFile(p, base); err != nil {
			return InstallResult{}, err
		}
		return InstallResult{Agent: agent, Files: []string{p}, Action: "installed Gemini CLI hooks + MCP"}, nil
	default:
		return InstallResult{}, fmt.Errorf("unsupported agent %q", agent)
	}
}
