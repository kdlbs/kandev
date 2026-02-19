package models

import "time"

type Agent struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	WorkspaceID   *string        `json:"workspace_id,omitempty"`
	SupportsMCP   bool           `json:"supports_mcp"`
	MCPConfigPath string         `json:"mcp_config_path,omitempty"`
	TUIConfig     *TUIConfigJSON `json:"tui_config,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// TUIConfigJSON is the JSON schema stored in the tui_config column for custom TUI agents.
type TUIConfigJSON struct {
	Command         string   `json:"command"`
	DisplayName     string   `json:"display_name"`
	Model           string   `json:"model,omitempty"`
	Description     string   `json:"description,omitempty"`
	CommandArgs     []string `json:"command_args,omitempty"`
	WaitForTerminal bool     `json:"wait_for_terminal"`
}

type AgentProfile struct {
	ID                         string     `json:"id"`
	AgentID                    string     `json:"agent_id"`
	Name                       string     `json:"name"`
	AgentDisplayName           string     `json:"agent_display_name"`
	Model                      string     `json:"model"`
	AutoApprove                bool       `json:"auto_approve"`
	DangerouslySkipPermissions bool       `json:"dangerously_skip_permissions"`
	AllowIndexing              bool       `json:"allow_indexing"`
	CLIPassthrough             bool       `json:"cli_passthrough"`
	UserModified               bool       `json:"user_modified"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	DeletedAt                  *time.Time `json:"deleted_at,omitempty"`
}
