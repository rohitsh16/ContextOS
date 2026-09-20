package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"contextos/internal/integrations"
)

func init() {
	Register(&ClaudeAdapter{})
}

// ClaudeAdapter implements AgentAdapter for Claude Code.
type ClaudeAdapter struct{}

func (a *ClaudeAdapter) ID() string   { return "claude" }
func (a *ClaudeAdapter) Name() string { return "Claude Code" }

func (a *ClaudeAdapter) Capabilities() Capabilities {
	return Capabilities{
		MCP:                 true,
		Hooks:               true,
		PromptInjection:     true,
		SessionStart:        true,
		SessionEnd:          true,
		ToolEvents:          true,
		Rules:               true,
		Skills:              false,
		ProjectConfig:       true,
		UserConfig:          true,
		PortableProjectPath: true,
	}
}

func (a *ClaudeAdapter) Detect(ctx DetectContext) DetectionResult {
	claudeDir := filepath.Join(ctx.RepoRoot, ".claude")
	claudeMD := filepath.Join(ctx.RepoRoot, "CLAUDE.md")
	_, errDir := os.Stat(claudeDir)
	_, errMD := os.Stat(claudeMD)

	installed := errDir == nil || errMD == nil
	return DetectionResult{
		Installed:   installed,
		ConfigFound: installed,
		Path:        claudeDir,
		Notes:       "Claude Code project configuration detected",
	}
}

func (a *ClaudeAdapter) Plan(ctx InstallContext) (InstallPlan, error) {
	settingsPath := filepath.Join(ctx.RepoRoot, ".claude", "settings.local.json")
	mcpPath := filepath.Join(ctx.RepoRoot, ".mcp.json")
	return InstallPlan{
		AdapterID: a.ID(),
		Actions: []PlanAction{
			{
				Type:        "merge_json",
				Path:        settingsPath,
				Description: "Configure ContextOS hooks in .claude/settings.local.json",
			},
			{
				Type:        "merge_json",
				Path:        mcpPath,
				Description: "Configure MCP server contextd in .mcp.json",
			},
		},
		AffectedFiles: []string{settingsPath, mcpPath},
	}, nil
}

func (a *ClaudeAdapter) Apply(ctx InstallContext, plan InstallPlan) error {
	_, err := integrations.Install(a.ID(), ctx.RepoRoot, ctx.HookBinary, ctx.MCPBinary)
	return err
}

func (a *ClaudeAdapter) Validate(ctx ValidateContext) ValidationResult {
	settingsPath := filepath.Join(ctx.RepoRoot, ".claude", "settings.local.json")
	mcpPath := filepath.Join(ctx.RepoRoot, ".mcp.json")
	checked := []string{settingsPath, mcpPath}

	var issues []string
	if b, err := os.ReadFile(settingsPath); err != nil {
		issues = append(issues, ".claude/settings.local.json not found or unreadable")
	} else {
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			issues = append(issues, ".claude/settings.local.json contains invalid JSON: "+err.Error())
		}
	}

	if b, err := os.ReadFile(mcpPath); err != nil {
		issues = append(issues, ".mcp.json not found or unreadable")
	} else {
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			issues = append(issues, ".mcp.json contains invalid JSON: "+err.Error())
		}
	}

	return ValidationResult{
		Valid:        len(issues) == 0,
		Issues:       issues,
		CheckedFiles: checked,
	}
}

func (a *ClaudeAdapter) Remove(ctx RemoveContext) error {
	_ = os.Remove(filepath.Join(ctx.RepoRoot, ".claude", "settings.local.json"))
	_ = os.Remove(filepath.Join(ctx.RepoRoot, ".mcp.json"))
	return nil
}
