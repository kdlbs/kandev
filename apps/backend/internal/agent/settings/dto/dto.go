package dto

import (
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type AgentProfileDTO struct {
	ID                         string    `json:"id"`
	AgentID                    string    `json:"agent_id"`
	Name                       string    `json:"name"`
	AgentDisplayName           string    `json:"agent_display_name"`
	Model                      string    `json:"model"`
	AutoApprove                bool      `json:"auto_approve"`
	DangerouslySkipPermissions bool      `json:"dangerously_skip_permissions"`
	AllowIndexing              bool      `json:"allow_indexing"`
	CLIPassthrough             bool      `json:"cli_passthrough"`
	UserModified               bool      `json:"user_modified"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
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
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
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
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"`
	IsDefault     bool   `json:"is_default"`
	Source        string `json:"source,omitempty"`
}

type ModelConfigDTO struct {
	DefaultModel          string          `json:"default_model"`
	AvailableModels       []ModelEntryDTO `json:"available_models"`
	SupportsDynamicModels bool            `json:"supports_dynamic_models"`
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

type AvailableAgentDTO struct {
	Name               string                          `json:"name"`
	DisplayName        string                          `json:"display_name"`
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

// DynamicModelsResponse is the response for the dynamic models endpoint
type DynamicModelsResponse struct {
	AgentName string          `json:"agent_name"`
	Models    []ModelEntryDTO `json:"models"`
	Cached    bool            `json:"cached"`
	CachedAt  *time.Time      `json:"cached_at,omitempty"`
	Error     *string         `json:"error"`
}
