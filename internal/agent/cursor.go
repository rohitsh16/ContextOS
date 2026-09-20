package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"contextos/internal/integrations"
)

func init() {
	Register(&CursorAdapter{})
}

// CursorAdapter implements AgentAdapter for Cursor IDE.
type CursorAdapter struct{}

func (a *CursorAdapter) ID() string   { return "cursor" }
func (a *CursorAdapter) Name() string { return "Cursor" }

func (a *CursorAdapter) Capabilities() Capabilities {
	return Capabilities{
		MCP:                 true,
		Hooks:               true,
		PromptInjection:     false,
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

func (a *CursorAdapter) Detect(ctx DetectContext) DetectionResult {
	cursorDir := filepath.Join(ctx.RepoRoot, ".cursor")
	rulesFile := filepath.Join(ctx.RepoRoot, ".cursorrules")
	_, errDir := os.Stat(cursorDir)
	_, errRules := os.Stat(rulesFile)

	installed := errDir == nil || errRules == nil
	return DetectionResult{
		Installed:   installed,
		ConfigFound: installed,
		Path:        cursorDir,
		Notes:       "Cursor workspace configuration detected",
	}
}

func (a *CursorAdapter) Plan(ctx InstallContext) (InstallPlan, error) {
	mcpPath := filepath.Join(ctx.RepoRoot, ".cursor", "mcp.json")
	rulesPath := filepath.Join(ctx.RepoRoot, ".cursorrules")
	return InstallPlan{
		AdapterID: a.ID(),
		Actions: []PlanAction{
			{
				Type:        "merge_json",
				Path:        mcpPath,
				Description: "Configure MCP server contextd in .cursor/mcp.json",
			},
			{
				Type:        "append_rule",
				Path:        rulesPath,
				Description: "Configure ContextOS rules in .cursorrules",
			},
		},
		AffectedFiles: []string{mcpPath, rulesPath},
	}, nil
}

func (a *CursorAdapter) Apply(ctx InstallContext, plan InstallPlan) error {
	_, err := integrations.Install(a.ID(), ctx.RepoRoot, ctx.HookBinary, ctx.MCPBinary)
	return err
}

func (a *CursorAdapter) Validate(ctx ValidateContext) ValidationResult {
	mcpPath := filepath.Join(ctx.RepoRoot, ".cursor", "mcp.json")
	checked := []string{mcpPath}
	b, err := os.ReadFile(mcpPath)
	if err != nil {
		return ValidationResult{
			Valid:        false,
			Issues:       []string{".cursor/mcp.json not found or unreadable"},
			CheckedFiles: checked,
		}
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		return ValidationResult{
			Valid:        false,
			Issues:       []string{".cursor/mcp.json contains invalid JSON: " + err.Error()},
			CheckedFiles: checked,
		}
	}
	return ValidationResult{
		Valid:        true,
		CheckedFiles: checked,
	}
}

func (a *CursorAdapter) Remove(ctx RemoveContext) error {
	mcpPath := filepath.Join(ctx.RepoRoot, ".cursor", "mcp.json")
	_ = os.Remove(mcpPath)
	return nil
}
