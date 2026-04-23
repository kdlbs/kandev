package dto

import (
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type AgentProfileDTO struct {
	ID               string       `json:"id"`
	AgentID          string       `json:"agent_id"`
	Name             string       `json:"name"`
	AgentDisplayName string       `json:"agent_display_name"`
	Model            string       `json:"model"`
	Mode             string       `json:"mode,omitempty"`
	AllowIndexing    bool         `json:"allow_indexing"` // Deprecated: use CLIFlags. Retained for legacy clients.
	CLIFlags         []CLIFlagDTO `json:"cli_flags"`
	CLIPassthrough   bool         `json:"cli_passthrough"`
	UserModified     bool         `json:"user_modified"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// CLIFlagDTO mirrors models.CLIFlag on the wire. Each entry is one user-facing
// CLI argument on a profile; at launch time the `flag` string is shell-split
// and only entries with `enabled:true` reach the agent subprocess argv.
type CLIFlagDTO struct {
	Description string `json:"description"`
	Flag        string `json:"flag"`
	Enabled     bool   `json:"enabled"`
}

type TUIConfigDTO struct {
	Command         string   `json:"command"`
	DisplayName     string   `json:"display_name"`
	Model           string   `json:"model,omitempty"`
	Description     string   `json:"description,omitempty"`
	CommandArgs     []string `json:"command_args,omitempty"`
	WaitForTerminal bool     `json:"wait_for_terminal"`
}

type AgentDTO struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	WorkspaceID   *string           `json:"workspace_id,omitempty"`
	SupportsMCP   bool              `json:"supports_mcp"`
	MCPConfigPath string            `json:"mcp_config_path,omitempty"`
	TUIConfig     *TUIConfigDTO     `json:"tui_config,omitempty"`
	Profiles      []AgentProfileDTO `json:"profiles"`
	// CapabilityStatus mirrors the host utility probe status so clients can
	// flag agents that need login or reinstallation without fetching the
	// full model config separately. "" for agents that aren't probed
	// (mock, tui-only).
	CapabilityStatus string    `json:"capability_status,omitempty"`
	CapabilityError  string    `json:"capability_error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ListAgentsResponse struct {
	Agents []AgentDTO `json:"agents"`
	Total  int        `json:"total"`
}

type AgentDiscoveryDTO struct {
	Name              string   `json:"name"`
	SupportsMCP       bool     `json:"supports_mcp"`
	MCPConfigPath     string   `json:"mcp_config_path,omitempty"`
	InstallationPaths []string `json:"installation_paths,omitempty"`
	Available         bool     `json:"available"`
	MatchedPath       string   `json:"matched_path,omitempty"`
}

type ListDiscoveryResponse struct {
	Agents []AgentDiscoveryDTO `json:"agents"`
	Total  int                 `json:"total"`
}

type AgentCapabilitiesDTO struct {
	SupportsSessionResume bool `json:"supports_session_resume"`
	SupportsShell         bool `json:"supports_shell"`
	SupportsWorkspaceOnly bool `json:"supports_workspace_only"`
}

type ModelEntryDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"`
	IsDefault     bool   `json:"is_default"`
	Source        string `json:"source,omitempty"`
	// Meta carries agent-specific extras from ACP's `_meta` field. For
	// GitHub Copilot this includes `copilotUsage` (e.g. "1x", "0.33x",
	// "0x" — the premium-request multiplier) and `copilotEnablement`.
	Meta map[string]any `json:"meta,omitempty"`
}

type ModelConfigDTO struct {
	DefaultModel          string            `json:"default_model"`
	AvailableModels       []ModelEntryDTO   `json:"available_models"`
	CurrentModelID        string            `json:"current_model_id,omitempty"`
	AvailableModes        []ModeEntryDTO    `json:"available_modes,omitempty"`
	CurrentModeID         string            `json:"current_mode_id,omitempty"`
	AvailableCommands     []CommandEntryDTO `json:"available_commands,omitempty"`
	SupportsDynamicModels bool              `json:"supports_dynamic_models"`
	// Status reflects the host utility probe state for this agent type:
	// "probing" | "ok" | "auth_required" | "not_installed" | "failed".
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type PermissionSettingDTO struct {
	Supported    bool   `json:"supported"`
	Default      bool   `json:"default"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	ApplyMethod  string `json:"apply_method,omitempty"`
	CLIFlag      string `json:"cli_flag,omitempty"`
	CLIFlagValue string `json:"cli_flag_value,omitempty"`
}

type PassthroughConfigDTO struct {
	Supported   bool   `json:"supported"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type ToolStatusDTO struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Available     bool   `json:"available"`
	MatchedPath   string `json:"matched_path,omitempty"`
	InstallScript string `json:"install_script,omitempty"`
	Description   string `json:"description,omitempty"`
	InfoURL       string `json:"info_url,omitempty"`
}

type AvailableAgentDTO struct {
	Name               string                          `json:"name"`
	DisplayName        string                          `json:"display_name"`
	Description        string                          `json:"description,omitempty"`
	InstallScript      string                          `json:"install_script,omitempty"`
	SupportsMCP        bool                            `json:"supports_mcp"`
	MCPConfigPath      string                          `json:"mcp_config_path,omitempty"`
	InstallationPaths  []string                        `json:"installation_paths,omitempty"`
	Available          bool                            `json:"available"`
	MatchedPath        string                          `json:"matched_path,omitempty"`
	Capabilities       AgentCapabilitiesDTO            `json:"capabilities"`
	ModelConfig        ModelConfigDTO                  `json:"model_config"`
	PermissionSettings map[string]PermissionSettingDTO `json:"permission_settings,omitempty"`
	PassthroughConfig  *PassthroughConfigDTO           `json:"passthrough_config,omitempty"`
	UpdatedAt          time.Time                       `json:"updated_at"`
}

type ListAvailableAgentsResponse struct {
	Agents []AvailableAgentDTO `json:"agents"`
	Tools  []ToolStatusDTO     `json:"tools,omitempty"`
	Total  int                 `json:"total"`
}

type AgentProfileMcpConfigDTO struct {
	ProfileID string                         `json:"profile_id"`
	Enabled   bool                           `json:"enabled"`
	Servers   map[string]mcpconfig.ServerDef `json:"servers"`
	Meta      map[string]any                 `json:"meta,omitempty"`
}

// CommandPreviewRequest is the request body for previewing the agent CLI command
type CommandPreviewRequest struct {
	Model              string          `json:"model"`
	PermissionSettings map[string]bool `json:"permission_settings"`
	CLIPassthrough     bool            `json:"cli_passthrough"`
}

// CommandPreviewResponse is the response for the command preview endpoint
type CommandPreviewResponse struct {
	Supported     bool     `json:"supported"`
	Command       []string `json:"command"`
	CommandString string   `json:"command_string"`
}

// DynamicModelsResponse is the response for the /agent-models/:agentName endpoint.
// Data now comes from the host utility capability cache populated by ACP probes.
type DynamicModelsResponse struct {
	AgentName      string            `json:"agent_name"`
	Status         string            `json:"status"` // "probing" | "ok" | "auth_required" | "not_installed" | "failed"
	Models         []ModelEntryDTO   `json:"models"`
	CurrentModelID string            `json:"current_model_id,omitempty"`
	Modes          []ModeEntryDTO    `json:"modes,omitempty"`
	CurrentModeID  string            `json:"current_mode_id,omitempty"`
	Commands       []CommandEntryDTO `json:"commands,omitempty"`
	Error          *string           `json:"error"`
}

// ModeEntryDTO is a single ACP session mode advertised by an agent.
type ModeEntryDTO struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

// CommandEntryDTO is a slash command advertised by the agent.
type CommandEntryDTO struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
