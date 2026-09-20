package agent

// DetectContext provides environment information for detecting an agent.
type DetectContext struct {
	RepoRoot    string
	HomeDir     string
	Environment map[string]string
}

// DetectionResult indicates whether an agent was detected.
type DetectionResult struct {
	Installed   bool   `json:"installed"`
	ConfigFound bool   `json:"config_found"`
	Version     string `json:"version,omitempty"`
	Path        string `json:"path,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// InstallContext provides the parameters needed to plan and install an agent integration.
type InstallContext struct {
	RepoRoot   string
	RepoName   string
	HookBinary string
	MCPBinary  string
	HomeDir    string
	DryRun     bool
}

// PlanAction defines an atomic modification to the filesystem or configuration.
type PlanAction struct {
	Type        string `json:"type"` // "create_file", "merge_json", "append_rule", "delete_file"
	Path        string `json:"path"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
}

// InstallPlan represents the deterministic set of actions needed to install or update an integration.
type InstallPlan struct {
	AdapterID     string       `json:"adapter_id"`
	Actions       []PlanAction `json:"actions"`
	AffectedFiles []string     `json:"affected_files"`
}

// ValidateContext provides context for verifying an integration's health.
type ValidateContext struct {
	RepoRoot string
	HomeDir  string
}

// ValidationResult reports whether the integration is currently healthy.
type ValidationResult struct {
	Valid        bool     `json:"valid"`
	Issues       []string `json:"issues,omitempty"`
	CheckedFiles []string `json:"checked_files"`
}

// RemoveContext provides parameters for uninstalling an integration.
type RemoveContext struct {
	RepoRoot string
	HomeDir  string
	DryRun   bool
}

// AgentAdapter defines the canonical contract for AI coding assistants.
type AgentAdapter interface {
	ID() string
	Name() string
	Capabilities() Capabilities
	Detect(ctx DetectContext) DetectionResult
	Plan(ctx InstallContext) (InstallPlan, error)
	Apply(ctx InstallContext, plan InstallPlan) error
	Validate(ctx ValidateContext) ValidationResult
	Remove(ctx RemoveContext) error
}
