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
	Plan                       string    `json:"plan"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type AgentDTO struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	WorkspaceID   *string           `json:"workspace_id,omitempty"`
	SupportsMCP   bool              `json:"supports_mcp"`
	MCPConfigPath string            `json:"mcp_config_path,omitempty"`
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
}

type ModelConfigDTO struct {
	DefaultModel    string          `json:"default_model"`
	AvailableModels []ModelEntryDTO `json:"available_models"`
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
