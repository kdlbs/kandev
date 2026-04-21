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
	ID               string `json:"id"`
	AgentID          string `json:"agent_id"`
	Name             string `json:"name"`
	AgentDisplayName string `json:"agent_display_name"`

	// Model is the ACP model ID applied via session/set_model at session start.
	// Validated against the host utility capability cache by the reconciler.
	Model string `json:"model"`

	// Mode is the optional ACP session mode applied via session/set_mode at
	// session start. Empty when the agent does not advertise modes.
	Mode string `json:"mode,omitempty"`

	// MigratedFrom records the agent_id this profile was migrated from, if any.
	// Used for audit of the one-shot non-ACP → ACP migration.
	MigratedFrom string `json:"migrated_from,omitempty"`

	// CLIPassthrough enables TUI-passthrough execution style. Orthogonal to ACP.
	CLIPassthrough bool `json:"cli_passthrough"`

	// AllowIndexing is retained for backward compatibility with existing
	// auggie profiles. The launch path no longer consults it — it is read
	// only by the legacy migration shim that seeds CLIFlags on the first
	// post-migration read. New code should use CLIFlags instead.
	AllowIndexing bool `json:"allow_indexing"`

	// CLIFlags is the user-configurable list of CLI flags passed to the agent
	// subprocess. At profile creation the list is seeded from the agent's
	// PermissionSettings(); users can toggle entries on/off, remove them, or
	// add custom entries via the settings UI. Only entries with Enabled=true
	// reach the subprocess argv.
	CLIFlags []CLIFlag `json:"cli_flags"`

	// Deprecated legacy permission fields: retained in the DB schema so rows
	// load cleanly, but no longer read by the launch path. ACP session modes
	// and interactive permission_request prompts replace them.
	AutoApprove                bool `json:"-"`
	DangerouslySkipPermissions bool `json:"-"`

	UserModified bool       `json:"user_modified"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// CLIFlag is a single user-configurable CLI argument on an AgentProfile.
// The raw Flag string is shell-tokenised at launch time: a single entry
// like "--add-dir /shared" becomes two argv tokens.
type CLIFlag struct {
	Description string `json:"description"`
	Flag        string `json:"flag"`
	Enabled     bool   `json:"enabled"`
}
