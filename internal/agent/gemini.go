package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"contextos/internal/integrations"
)

func init() {
	Register(&GeminiAdapter{})
}

// GeminiAdapter implements AgentAdapter for Gemini CLI / Code Assist.
type GeminiAdapter struct{}

func (a *GeminiAdapter) ID() string   { return "gemini" }
func (a *GeminiAdapter) Name() string { return "Gemini CLI" }

func (a *GeminiAdapter) Capabilities() Capabilities {
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

func (a *GeminiAdapter) Detect(ctx DetectContext) DetectionResult {
	geminiDir := filepath.Join(ctx.RepoRoot, ".gemini")
	_, err := os.Stat(geminiDir)
	installed := err == nil
	return DetectionResult{
		Installed:   installed,
		ConfigFound: installed,
		Path:        geminiDir,
		Notes:       "Gemini configuration directory detected",
	}
}

func (a *GeminiAdapter) Plan(ctx InstallContext) (InstallPlan, error) {
	settingsPath := filepath.Join(ctx.RepoRoot, ".gemini", "settings.json")
	return InstallPlan{
		AdapterID: a.ID(),
		Actions: []PlanAction{
			{
				Type:        "merge_json",
				Path:        settingsPath,
				Description: "Configure MCP server contextd in .gemini/settings.json",
			},
		},
		AffectedFiles: []string{settingsPath},
	}, nil
}

func (a *GeminiAdapter) Apply(ctx InstallContext, plan InstallPlan) error {
	_, err := integrations.Install(a.ID(), ctx.RepoRoot, ctx.HookBinary, ctx.MCPBinary)
	return err
}

func (a *GeminiAdapter) Validate(ctx ValidateContext) ValidationResult {
	settingsPath := filepath.Join(ctx.RepoRoot, ".gemini", "settings.json")
	checked := []string{settingsPath}
	b, err := os.ReadFile(settingsPath)
	if err != nil {
		return ValidationResult{
			Valid:        false,
			Issues:       []string{".gemini/settings.json not found or unreadable"},
			CheckedFiles: checked,
		}
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		return ValidationResult{
			Valid:        false,
			Issues:       []string{".gemini/settings.json contains invalid JSON: " + err.Error()},
			CheckedFiles: checked,
		}
	}
	return ValidationResult{
		Valid:        true,
		CheckedFiles: checked,
	}
}

func (a *GeminiAdapter) Remove(ctx RemoveContext) error {
	settingsPath := filepath.Join(ctx.RepoRoot, ".gemini", "settings.json")
	_ = os.Remove(settingsPath)
	return nil
}
