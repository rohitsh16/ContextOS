package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"contextos/internal/integrations"
)

func init() {
	Register(&CodexAdapter{})
}

// CodexAdapter implements AgentAdapter for Codex CLI.
type CodexAdapter struct{}

func (a *CodexAdapter) ID() string   { return "codex" }
func (a *CodexAdapter) Name() string { return "Codex CLI" }

func (a *CodexAdapter) Capabilities() Capabilities {
	return Capabilities{
		MCP:                 true,
		Hooks:               true,
		PromptInjection:     true,
		SessionStart:        true,
		SessionEnd:          false,
		ToolEvents:          true,
		Rules:               true,
		Skills:              false,
		ProjectConfig:       true,
		UserConfig:          true,
		PortableProjectPath: true,
	}
}

func (a *CodexAdapter) Detect(ctx DetectContext) DetectionResult {
	codexDir := filepath.Join(ctx.RepoRoot, ".codex")
	_, err := os.Stat(codexDir)
	installed := err == nil
	return DetectionResult{
		Installed:   installed,
		ConfigFound: installed,
		Path:        codexDir,
		Notes:       "Codex CLI configuration directory detected",
	}
}

func (a *CodexAdapter) Plan(ctx InstallContext) (InstallPlan, error) {
	hookPath := filepath.Join(ctx.RepoRoot, ".codex", "hooks.json")
	mcpPath := filepath.Join(ctx.RepoRoot, ".codex", "config.toml")
	return InstallPlan{
		AdapterID: a.ID(),
		Actions: []PlanAction{
			{
				Type:        "merge_json",
				Path:        hookPath,
				Description: "Configure ContextOS hooks in .codex/hooks.json",
			},
			{
				Type:        "merge_toml",
				Path:        mcpPath,
				Description: "Configure MCP server contextd in .codex/config.toml",
			},
		},
		AffectedFiles: []string{hookPath, mcpPath},
	}, nil
}

func (a *CodexAdapter) Apply(ctx InstallContext, plan InstallPlan) error {
	_, err := integrations.Install(a.ID(), ctx.RepoRoot, ctx.HookBinary, ctx.MCPBinary)
	return err
}

func (a *CodexAdapter) Validate(ctx ValidateContext) ValidationResult {
	hookPath := filepath.Join(ctx.RepoRoot, ".codex", "hooks.json")
	mcpPath := filepath.Join(ctx.RepoRoot, ".codex", "config.toml")
	checked := []string{hookPath, mcpPath}

	var issues []string
	if b, err := os.ReadFile(hookPath); err != nil {
		issues = append(issues, ".codex/hooks.json not found or unreadable")
	} else {
		var data map[string]any
		if err := json.Unmarshal(b, &data); err != nil {
			issues = append(issues, ".codex/hooks.json contains invalid JSON: "+err.Error())
		}
	}

	if _, err := os.ReadFile(mcpPath); err != nil {
		issues = append(issues, ".codex/config.toml not found or unreadable")
	}

	return ValidationResult{
		Valid:        len(issues) == 0,
		Issues:       issues,
		CheckedFiles: checked,
	}
}

func (a *CodexAdapter) Remove(ctx RemoveContext) error {
	_ = os.Remove(filepath.Join(ctx.RepoRoot, ".codex", "hooks.json"))
	_ = os.Remove(filepath.Join(ctx.RepoRoot, ".codex", "config.toml"))
	return nil
}
