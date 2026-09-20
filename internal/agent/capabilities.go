package agent

// Capabilities represents the features supported by a specific AI agent or coding environment.
type Capabilities struct {
	MCP                 bool `json:"mcp"`
	Hooks               bool `json:"hooks"`
	PromptInjection     bool `json:"prompt_injection"`
	SessionStart        bool `json:"session_start"`
	SessionEnd          bool `json:"session_end"`
	ToolEvents          bool `json:"tool_events"`
	Rules               bool `json:"rules"`
	Skills              bool `json:"skills"`
	ProjectConfig       bool `json:"project_config"`
	UserConfig          bool `json:"user_config"`
	PortableProjectPath bool `json:"portable_project_path"`
}
