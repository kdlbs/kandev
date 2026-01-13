package dto

import "time"

type AgentProfileDTO struct {
	ID                         string    `json:"id"`
	AgentID                    string    `json:"agent_id"`
	Name                       string    `json:"name"`
	Model                      string    `json:"model"`
	AutoApprove                bool      `json:"auto_approve"`
	DangerouslySkipPermissions bool      `json:"dangerously_skip_permissions"`
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
