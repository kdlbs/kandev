// Package config provides config sync, import, and export for the office domain.
package config

// ConfigBundle represents a portable workspace configuration.
type ConfigBundle struct {
	Settings SettingsConfig  `json:"settings" yaml:"settings"`
	Agents   []AgentConfig   `json:"agents" yaml:"agents"`
	Skills   []SkillConfig   `json:"skills" yaml:"skills"`
	Routines []RoutineConfig `json:"routines" yaml:"routines"`
	Projects []ProjectConfig `json:"projects" yaml:"projects"`
}

// SettingsConfig represents workspace-level settings.
type SettingsConfig struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// AgentConfig represents a portable agent instance configuration.
type AgentConfig struct {
	Name                  string `json:"name" yaml:"name"`
	Role                  string `json:"role" yaml:"role"`
	Icon                  string `json:"icon,omitempty" yaml:"icon,omitempty"`
	ReportsTo             string `json:"reports_to,omitempty" yaml:"reports_to,omitempty"`
	BudgetMonthlyCents    int    `json:"budget_monthly_cents" yaml:"budget_monthly_cents"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions" yaml:"max_concurrent_sessions"`
	DesiredSkills         string `json:"desired_skills,omitempty" yaml:"desired_skills,omitempty"`
	ExecutorPreference    string `json:"executor_preference,omitempty" yaml:"executor_preference,omitempty"`
}

// SkillConfig represents a portable skill configuration.
type SkillConfig struct {
	Name        string `json:"name" yaml:"name"`
	Slug        string `json:"slug" yaml:"slug"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	SourceType  string `json:"source_type" yaml:"source_type"`
	Content     string `json:"content,omitempty" yaml:"content,omitempty"`
}

// RoutineConfig represents a portable routine configuration.
type RoutineConfig struct {
	Name              string `json:"name" yaml:"name"`
	Description       string `json:"description,omitempty" yaml:"description,omitempty"`
	TaskTemplate      string `json:"task_template,omitempty" yaml:"task_template,omitempty"`
	AssigneeName      string `json:"assignee_name,omitempty" yaml:"assignee_name,omitempty"`
	ConcurrencyPolicy string `json:"concurrency_policy,omitempty" yaml:"concurrency_policy,omitempty"`
}

// ProjectConfig represents a portable project configuration.
type ProjectConfig struct {
	Name           string `json:"name" yaml:"name"`
	Description    string `json:"description,omitempty" yaml:"description,omitempty"`
	Status         string `json:"status,omitempty" yaml:"status,omitempty"`
	Color          string `json:"color,omitempty" yaml:"color,omitempty"`
	BudgetCents    int    `json:"budget_cents,omitempty" yaml:"budget_cents,omitempty"`
	Repositories   string `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	ExecutorConfig string `json:"executor_config,omitempty" yaml:"executor_config,omitempty"`
	LeadAgentName  string `json:"lead_agent_name,omitempty" yaml:"lead_agent_name,omitempty"`
}

// ImportDiff represents the changes that would be applied.
type ImportDiff struct {
	Created []string `json:"created"`
	Updated []string `json:"updated"`
	Deleted []string `json:"deleted"`
}

// ImportPreview shows what an import would change.
type ImportPreview struct {
	Agents   ImportDiff `json:"agents"`
	Skills   ImportDiff `json:"skills"`
	Routines ImportDiff `json:"routines"`
	Projects ImportDiff `json:"projects"`
}

// ImportResult reports what was actually changed.
type ImportResult struct {
	CreatedCount int `json:"created_count"`
	UpdatedCount int `json:"updated_count"`
}

// ParseError describes a single file that failed to parse during a FS scan.
type ParseError struct {
	WorkspaceID string `json:"workspace_id"`
	FilePath    string `json:"file_path"`
	Error       string `json:"error"`
}

// SyncDiff is a bidirectional preview returned by sync endpoints.
type SyncDiff struct {
	Direction string         `json:"direction"`
	Preview   *ImportPreview `json:"preview"`
	Errors    []ParseError   `json:"errors,omitempty"`
}
