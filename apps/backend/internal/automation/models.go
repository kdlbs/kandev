// Package automation provides a generic automation system for Kandev.
// Automations are named rules with triggers (cron, GitHub events, webhooks)
// that automatically create and optionally start tasks when fired.
package automation

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/github"
)

// TriggerType identifies the kind of trigger.
type TriggerType string

const (
	TriggerTypeScheduled  TriggerType = "scheduled"
	TriggerTypeGitHubPR   TriggerType = "github_pr"
	TriggerTypeGitHubPush TriggerType = "github_push"
	TriggerTypeGitHubCI   TriggerType = "github_ci"
	TriggerTypeWebhook    TriggerType = "webhook"
)

// RunStatus tracks the outcome of a trigger firing.
type RunStatus string

const (
	RunStatusTriggered   RunStatus = "triggered"
	RunStatusTaskCreated RunStatus = "task_created"
	RunStatusFailed      RunStatus = "failed"
	RunStatusSkipped     RunStatus = "skipped"
)

// Automation is a named rule with triggers, a prompt template, and agent/executor config.
type Automation struct {
	ID                string     `json:"id" db:"id"`
	WorkspaceID       string     `json:"workspace_id" db:"workspace_id"`
	Name              string     `json:"name" db:"name"`
	Description       string     `json:"description" db:"description"`
	WorkflowID        string     `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID    string     `json:"workflow_step_id" db:"workflow_step_id"`
	AgentProfileID    string     `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutorProfileID string     `json:"executor_profile_id" db:"executor_profile_id"`
	Prompt            string     `json:"prompt" db:"prompt"`
	TaskTitleTemplate string     `json:"task_title_template" db:"task_title_template"`
	Enabled           bool       `json:"enabled" db:"enabled"`
	MaxConcurrentRuns int        `json:"max_concurrent_runs" db:"max_concurrent_runs"`
	WebhookSecret     string     `json:"-" db:"webhook_secret"`
	LastTriggeredAt   *time.Time `json:"last_triggered_at,omitempty" db:"last_triggered_at"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`

	// Hydrated separately, not stored in the automations table.
	Triggers []AutomationTrigger `json:"triggers" db:"-"`
}

// AutomationTrigger is a single trigger attached to an automation.
type AutomationTrigger struct {
	ID              string          `json:"id" db:"id"`
	AutomationID    string          `json:"automation_id" db:"automation_id"`
	Type            TriggerType     `json:"type" db:"type"`
	Config          json.RawMessage `json:"config" db:"-"`
	ConfigJSON      string          `json:"-" db:"config"`
	Enabled         bool            `json:"enabled" db:"enabled"`
	LastEvaluatedAt *time.Time      `json:"last_evaluated_at,omitempty" db:"last_evaluated_at"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
}

// AutomationRun records a single trigger firing for audit/observability.
type AutomationRun struct {
	ID              string          `json:"id" db:"id"`
	AutomationID    string          `json:"automation_id" db:"automation_id"`
	TriggerID       string          `json:"trigger_id" db:"trigger_id"`
	TriggerType     TriggerType     `json:"trigger_type" db:"trigger_type"`
	TaskID          string          `json:"task_id,omitempty" db:"task_id"`
	Status          RunStatus       `json:"status" db:"status"`
	DedupKey        string          `json:"dedup_key" db:"dedup_key"`
	TriggerData     json.RawMessage `json:"trigger_data" db:"-"`
	TriggerDataJSON string          `json:"-" db:"trigger_data"`
	ErrorMessage    string          `json:"error_message,omitempty" db:"error_message"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
}

// --- Trigger config types ---

// ScheduledTriggerConfig holds configuration for cron-based triggers.
type ScheduledTriggerConfig struct {
	CronExpression string `json:"cron_expression"`
	Timezone       string `json:"timezone,omitempty"`
}

// GitHubPRTriggerConfig filters PR events.
type GitHubPRTriggerConfig struct {
	Events       []string            `json:"events"`             // opened, commented, merged, review_requested, closed
	Repos        []github.RepoFilter `json:"repos"`              // empty = all repos
	Branches     []string            `json:"branches,omitempty"` // base branch filter
	Authors      []string            `json:"authors,omitempty"`  // PR author filter
	Labels       []string            `json:"labels,omitempty"`   // label filter
	ExcludeDraft bool                `json:"exclude_draft,omitempty"`
}

// GitHubPushTriggerConfig filters push-to-branch events.
type GitHubPushTriggerConfig struct {
	Repos    []github.RepoFilter `json:"repos"`
	Branches []string            `json:"branches"` // glob patterns: ["main", "release/*"]
}

// GitHubCITriggerConfig filters CI completion events.
type GitHubCITriggerConfig struct {
	Repos       []github.RepoFilter `json:"repos"`
	Conclusions []string            `json:"conclusions"` // success, failure, etc.
	CheckNames  []string            `json:"check_names,omitempty"`
}

// WebhookTriggerConfig holds configuration for webhook triggers.
type WebhookTriggerConfig struct {
	FilterExpression string `json:"filter_expression,omitempty"`
}

// --- Request/response DTOs ---

// CreateAutomationRequest is the payload for creating an automation.
type CreateAutomationRequest struct {
	WorkspaceID       string              `json:"workspace_id"`
	Name              string              `json:"name"`
	Description       string              `json:"description"`
	WorkflowID        string              `json:"workflow_id"`
	WorkflowStepID    string              `json:"workflow_step_id"`
	AgentProfileID    string              `json:"agent_profile_id"`
	ExecutorProfileID string              `json:"executor_profile_id"`
	Prompt            string              `json:"prompt"`
	TaskTitleTemplate string              `json:"task_title_template"`
	MaxConcurrentRuns int                 `json:"max_concurrent_runs"`
	Triggers          []CreateTriggerSpec `json:"triggers"`
}

// CreateTriggerSpec defines a trigger to add during automation creation.
type CreateTriggerSpec struct {
	Type    TriggerType     `json:"type"`
	Config  json.RawMessage `json:"config"`
	Enabled bool            `json:"enabled"`
}

// UpdateAutomationRequest is the payload for updating an automation.
type UpdateAutomationRequest struct {
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	WorkflowID        *string `json:"workflow_id,omitempty"`
	WorkflowStepID    *string `json:"workflow_step_id,omitempty"`
	AgentProfileID    *string `json:"agent_profile_id,omitempty"`
	ExecutorProfileID *string `json:"executor_profile_id,omitempty"`
	Prompt            *string `json:"prompt,omitempty"`
	TaskTitleTemplate *string `json:"task_title_template,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	MaxConcurrentRuns *int    `json:"max_concurrent_runs,omitempty"`
}

// AddTriggerRequest adds a trigger to an existing automation.
type AddTriggerRequest struct {
	AutomationID string          `json:"automation_id"`
	Type         TriggerType     `json:"type"`
	Config       json.RawMessage `json:"config"`
	Enabled      bool            `json:"enabled"`
}

// UpdateTriggerRequest updates a trigger's config or enabled state.
type UpdateTriggerRequest struct {
	Config  *json.RawMessage `json:"config,omitempty"`
	Enabled *bool            `json:"enabled,omitempty"`
}

// AutomationTriggeredEvent is published when a trigger fires.
type AutomationTriggeredEvent struct {
	AutomationID string          `json:"automation_id"`
	TriggerID    string          `json:"trigger_id"`
	TriggerType  TriggerType     `json:"trigger_type"`
	TriggerData  json.RawMessage `json:"trigger_data"`
	DedupKey     string          `json:"dedup_key"`
}
