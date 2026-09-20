package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"contextos/internal/integrations"
)

func init() {
	Register(&AntigravityAdapter{})
}

// AntigravityAdapter implements AgentAdapter for Google Antigravity IDE.
type AntigravityAdapter struct{}

func (a *AntigravityAdapter) ID() string   { return "antigravity" }
func (a *AntigravityAdapter) Name() string { return "Google Antigravity" }

func (a *AntigravityAdapter) Capabilities() Capabilities {
	return Capabilities{
		MCP:                 true,
		Hooks:               true,
		PromptInjection:     true,
		SessionStart:        true,
		SessionEnd:          true,
		ToolEvents:          true,
		Rules:               true,
		Skills:              true,
		ProjectConfig:       true,
		UserConfig:          true,
		PortableProjectPath: true,
	}
}

func (a *AntigravityAdapter) Detect(ctx DetectContext) DetectionResult {
	agentsDir := filepath.Join(ctx.RepoRoot, ".agents")
	mcpFile := filepath.Join(agentsDir, "mcp_config.json")
	_, errDir := os.Stat(agentsDir)
	_, errMcp := os.Stat(mcpFile)

	installed := errDir == nil || errMcp == nil
	return DetectionResult{
		Installed:   installed,
		ConfigFound: installed,
		Path:        agentsDir,
		Notes:       "Antigravity IDE workspace configuration detected",
	}
}

func (a *AntigravityAdapter) Plan(ctx InstallContext) (InstallPlan, error) {
	agentsDir := filepath.Join(ctx.RepoRoot, ".agents")
	mcpPath := filepath.Join(agentsDir, "mcp_config.json")
	hooksPath := filepath.Join(agentsDir, "hooks.json")
	rulesPath := filepath.Join(agentsDir, "rules", "contextos.md")

	return InstallPlan{
		AdapterID: a.ID(),
		Actions: []PlanAction{
			{
				Type:        "merge_json",
				Path:        mcpPath,
				Description: "Configure MCP server contextd in .agents/mcp_config.json",
			},
			{
				Type:        "merge_json",
				Path:        hooksPath,
				Description: "Configure session and command hooks in .agents/hooks.json",
			},
			{
				Type:        "create_file",
				Path:        rulesPath,
				Description: "Create ContextOS behavioral guidelines in .agents/rules/contextos.md",
			},
		},
		AffectedFiles: []string{mcpPath, hooksPath, rulesPath},
	}, nil
}

func (a *AntigravityAdapter) Apply(ctx InstallContext, plan InstallPlan) error {
	_, err := integrations.Install(a.ID(), ctx.RepoRoot, ctx.HookBinary, ctx.MCPBinary)
	return err
}

func (a *AntigravityAdapter) Validate(ctx ValidateContext) ValidationResult {
	agentsDir := filepath.Join(ctx.RepoRoot, ".agents")
	mcpPath := filepath.Join(agentsDir, "mcp_config.json")
	hooksPath := filepath.Join(agentsDir, "hooks.json")
	checked := []string{mcpPath, hooksPath}

	var issues []string
	if b, err := os.ReadFile(mcpPath); err != nil {
		issues = append(issues, ".agents/mcp_config.json not found or unreadable")
	} else {
		var d map[string]any
		if err := json.Unmarshal(b, &d); err != nil {
			issues = append(issues, ".agents/mcp_config.json contains invalid JSON: "+err.Error())
		}
	}

	if b, err := os.ReadFile(hooksPath); err != nil {
		issues = append(issues, ".agents/hooks.json not found or unreadable")
	} else {
		var d map[string]any
		if err := json.Unmarshal(b, &d); err != nil {
			issues = append(issues, ".agents/hooks.json contains invalid JSON: "+err.Error())
		}
	}

	return ValidationResult{
		Valid:        len(issues) == 0,
		Issues:       issues,
		CheckedFiles: checked,
	}
}

func (a *AntigravityAdapter) Remove(ctx RemoveContext) error {
	agentsDir := filepath.Join(ctx.RepoRoot, ".agents")
	_ = os.Remove(filepath.Join(agentsDir, "mcp_config.json"))
	_ = os.Remove(filepath.Join(agentsDir, "hooks.json"))
	_ = os.Remove(filepath.Join(agentsDir, "rules", "contextos.md"))
	return nil
}
