package models

import (
	"time"

	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

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
	ID               string `json:"id" db:"id"`
	AgentID          string `json:"agent_id" db:"agent_id"`
	Name             string `json:"name" db:"name"`
	AgentDisplayName string `json:"agent_display_name" db:"agent_display_name"`

	// BillingType is computed at read time from credential files — not stored in the DB.
	// Values: "api_key" | "subscription".
	BillingType string `json:"billing_type,omitempty" db:"-"`

	// Model is the ACP model ID applied via session/set_model at session start.
	// Validated against the host utility capability cache by the reconciler.
	Model string `json:"model" db:"model"`

	// Mode is the optional ACP session mode applied via session/set_mode at
	// session start. Empty when the agent does not advertise modes.
	// The DB column is nullable; the settings repo handles the empty-string ↔ NULL
	// conversion via sql.NullString in scan/insert paths, so callers see a
	// regular string here.
	Mode string `json:"mode,omitempty" db:"-"`

	// MigratedFrom records the agent_id this profile was migrated from, if any.
	// Same db:"-" treatment as Mode (nullable column, settings repo handles).
	MigratedFrom string `json:"migrated_from,omitempty" db:"-"`

	// CLIPassthrough enables TUI-passthrough execution style. Orthogonal to ACP.
	CLIPassthrough bool `json:"cli_passthrough" db:"cli_passthrough"`

	// AllowIndexing is retained for backward compatibility with existing
	// auggie profiles. The launch path no longer consults it — it is read
	// only by the legacy migration shim that seeds CLIFlags on the first
	// post-migration read. New code should use CLIFlags instead.
	AllowIndexing bool `json:"allow_indexing" db:"allow_indexing"`

	// CLIFlags is the user-configurable list of CLI flags passed to the agent
	// subprocess. At profile creation the list is seeded from the agent's
	// PermissionSettings(); users can toggle entries on/off, remove them, or
	// add custom entries via the settings UI. Only entries with Enabled=true
	// reach the subprocess argv. Stored as a JSON-encoded TEXT column;
	// settings repo handles the conversion via manual scan.
	CLIFlags []CLIFlag `json:"cli_flags" db:"-"`

	// Deprecated legacy permission fields: retained in the DB schema so rows
	// load cleanly, but no longer read by the launch path. ACP session modes
	// and interactive permission_request prompts replace them.
	AutoApprove                bool `json:"-" db:"auto_approve"`
	DangerouslySkipPermissions bool `json:"-" db:"dangerously_skip_permissions"`

	UserModified bool       `json:"user_modified" db:"user_modified"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`

	// Office enrichment fields (ADR 0005 Wave A; collapsed with
	// office.AgentInstance in Wave G). Empty / zero values mean the profile
	// is a "shallow" kanban-flavour profile; populated values mark a "rich"
	// office-flavour profile. Both flavours live in the same agent_profiles
	// row, and office code references this struct via the
	// office/models.AgentInstance type alias.
	//
	// WorkspaceID scopes the profile to a workspace. Empty = global / kanban-legacy.
	WorkspaceID string `json:"workspace_id,omitempty" db:"workspace_id"`
	// Role is the organisational role of an office agent. Empty for shallow
	// kanban profiles. The DB column is TEXT NOT NULL DEFAULT ''.
	Role AgentRole `json:"role,omitempty" db:"role"`
	// Icon is an emoji or short string surfaced in the office UI.
	Icon string `json:"icon,omitempty" db:"icon"`
	// ReportsTo references another agent_profile.id (org chart).
	ReportsTo string `json:"reports_to,omitempty" db:"reports_to"`
	// SkillIDs is the JSON-array string of skill IDs (office.skills) the
	// agent owns. Stored verbatim as a TEXT column to match the office
	// repo's existing string-based handling.
	SkillIDs string `json:"skill_ids,omitempty" db:"skill_ids"`
	// DesiredSkills is the legacy office field listing skill slugs the
	// agent wants when launched. JSON-array string (kept distinct from
	// SkillIDs per the ADR — separate columns, separate semantics).
	DesiredSkills string `json:"desired_skills,omitempty" db:"desired_skills"`
	// CustomPrompt is deprecated: per-agent prompt addendums were removed in
	// favour of multi-file instructions. The DB column remains for backward
	// compatibility with existing rows but is never read or written by the
	// UI/runtime. Slated for removal in a follow-up migration.
	CustomPrompt string `json:"custom_prompt,omitempty" db:"custom_prompt"`
	// Status is the runtime status of an office agent.
	Status AgentStatus `json:"status,omitempty" db:"status"`
	// PauseReason is a free-text explanation when Status == paused.
	PauseReason string `json:"pause_reason,omitempty" db:"pause_reason"`
	// LastRunFinishedAt is the wall-clock time of the most recent finished
	// run. Used by cooldown gating.
	LastRunFinishedAt *time.Time `json:"last_run_finished_at,omitempty" db:"last_run_finished_at"`
	// MaxConcurrentSessions caps the number of in-flight sessions for the agent.
	MaxConcurrentSessions int `json:"max_concurrent_sessions,omitempty" db:"max_concurrent_sessions"`
	// CooldownSec is the minimum gap between successive runs.
	CooldownSec int `json:"cooldown_sec,omitempty" db:"cooldown_sec"`
	// SkipIdleRuns short-circuits scheduler ticks when the agent has nothing
	// queued.
	SkipIdleRuns bool `json:"skip_idle_runs,omitempty" db:"skip_idle_runs"`
	// ConsecutiveFailures is the running count for auto-pause gating.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty" db:"consecutive_failures"`
	// FailureThreshold is the per-agent override for the workspace-level
	// auto-pause threshold. nil → use workspace default. The DB column is
	// INTEGER NOT NULL DEFAULT 3; the office repo round-trips nil ↔ 0 via
	// NULLIF / failureThresholdToColumn.
	FailureThreshold *int `json:"failure_threshold,omitempty" db:"failure_threshold"`
	// ExecutorPreference is a hint for which executor backend to prefer
	// (free-form JSON or a simple type name).
	ExecutorPreference string `json:"executor_preference,omitempty" db:"executor_preference"`
	// BudgetMonthlyCents is the per-agent monthly budget cap.
	BudgetMonthlyCents int `json:"budget_monthly_cents,omitempty" db:"budget_monthly_cents"`
	// Settings is a free-form JSON object holding office fields not promoted
	// to dedicated columns.
	Settings string `json:"settings,omitempty" db:"settings"`
	// Permissions is a free-form JSON object holding office permission
	// flags (e.g. spawn_subagents, hire_agents, approve_budget). Stored as
	// the raw JSON string so callers may unmarshal into their own shape.
	// Empty / "{}" means "no special permissions".
	Permissions string `json:"permissions,omitempty" db:"permissions"`

	// Utilization is populated for subscription-billed agents only. nil
	// for api_key billing. Computed at read time — not stored in the DB.
	Utilization *agentusage.ProviderUsage `json:"utilization,omitempty" db:"-"`
}

// CLIFlag is a single user-configurable CLI argument on an AgentProfile.
// The raw Flag string is shell-tokenised at launch time: a single entry
// like "--add-dir /shared" becomes two argv tokens.
type CLIFlag struct {
	Description string `json:"description"`
	Flag        string `json:"flag"`
	Enabled     bool   `json:"enabled"`
}
