// Package automation provides a generic automation system for Kandev.
// Automations are named rules with triggers (cron, GitHub events, webhooks)
// that automatically create and optionally start tasks when fired.
package automation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/github"
)

// TriggerType identifies the kind of trigger.
type TriggerType string

const (
	TriggerTypeScheduled      TriggerType = "scheduled"
	TriggerTypeGitHubPR       TriggerType = "github_pr"
	TriggerTypeGitHubPRMerged TriggerType = "github_pr_merged"
	TriggerTypeGitHubPush     TriggerType = "github_push"
	TriggerTypeGitHubCI       TriggerType = "github_ci"
	TriggerTypeWebhook        TriggerType = "webhook"
)

const (
	automationAuthorLoginKey   = "author_login"
	automationBaseBranchKey    = "base_branch"
	automationBodyKey          = "body"
	automationHeadBranchKey    = "head_branch"
	automationHTMLURLKey       = "html_url"
	automationRepoKey          = "repo"
	defaultBranchMain          = "main"
	exampleGitHubPRURL         = "https://github.com/acme/api/pull/7"
	exampleRepositoryOwner     = "org/repo"
	placeholderRepositoryOwner = "Repository (owner/name)"
	triggerCategoryGitHub      = "github"
	triggerDataSourceKey       = "source"
	triggerDataSourceManual    = "manual"
)

// RunStatus tracks the outcome of a trigger firing.
type RunStatus string

const (
	RunStatusTriggered   RunStatus = "triggered"
	RunStatusTaskCreated RunStatus = "task_created"
	RunStatusSucceeded   RunStatus = "succeeded"
	RunStatusFailed      RunStatus = "failed"
	RunStatusSkipped     RunStatus = "skipped"
	// RunStatusArchived is a read-time-derived terminal status: it is never
	// written by a trigger-firing code path, only computed by ListRuns when
	// a task_created run's generated task has been archived (archived_at
	// set) — whether archived through the UI or by the agent itself (e.g.
	// via an "archive this task" instruction). The task's outcome may still
	// be known (succeeded/failed already recorded); this only overrides the
	// still-open task_created status, never a real terminal one.
	RunStatusArchived RunStatus = "archived"
	// RunStatusCancelled is a read-time-derived terminal status: it is never
	// written by a trigger-firing code path, only computed by ListRuns (and
	// implied by CountActiveRuns' filter) for a task_created run whose
	// generated task either no longer exists (deleted — outcome
	// unrecoverable) or whose task's *current* session (task_sessions row
	// with is_primary = 1) was explicitly put into the CANCELLED state —
	// set only when an agent run was manually stopped (a deliberate
	// cancellation, distinct from archiving; the task's own state is not
	// touched by a stop and so isn't consulted here). Archived_at takes
	// precedence over the cancelled-session check when both apply — see
	// listRunsWithTaskState.
	RunStatusCancelled RunStatus = "cancelled"
)

// ContinuationPolicy controls whether a firing receives an isolated task or
// continues the automation's durable thread.
type ContinuationPolicy string

const (
	ContinuationPolicyNewTask     ContinuationPolicy = "new_task"
	ContinuationPolicyReuseThread ContinuationPolicy = "reuse_thread"
)

// TaskMode controls whether an automation firing owns a coordinator-only
// task or a normal user-visible task.
type TaskMode string

const (
	TaskModeAutomationRun TaskMode = "automation_run"
	TaskModeNormalTask    TaskMode = "normal_task"
)

// RepositoryMode controls how a firing chooses its repository environment.
type RepositoryMode string

const (
	RepositoryModeWorkspaceDefault RepositoryMode = "workspace_default"
	RepositoryModeSelected         RepositoryMode = "selected"
	RepositoryModeNone             RepositoryMode = "none"
)

// AutomationRepository is one exact repository environment selected for an
// automation. BaseBranch is persisted with the repository so dispatch never
// depends on workspace repository ordering or a later default-branch change.
type AutomationRepository struct {
	RepositoryID string `json:"repository_id" db:"repository_id"`
	BaseBranch   string `json:"base_branch" db:"base_branch"`
}

// ThreadAction describes how a dispatched run reached its task/session.
type ThreadAction string

const (
	ThreadActionCreated  ThreadAction = "created"
	ThreadActionResumed  ThreadAction = "resumed"
	ThreadActionReplaced ThreadAction = "replaced"
)

// Automation is a named rule with triggers, a prompt template, and agent/executor config.
type Automation struct {
	ID          string `json:"id" db:"id"`
	WorkspaceID string `json:"workspace_id" db:"workspace_id"`
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	// TaskModeAutomationRun is coordinator-only and may omit a workflow. A
	// TaskModeNormalTask must name a workflow so the generated task enters the
	// normal task lifecycle and appears in the Kanban/sidebar.
	TaskMode           TaskMode           `json:"task_mode" db:"task_mode"`
	RepositoryMode     RepositoryMode     `json:"repository_mode" db:"repository_mode"`
	WorkflowID         string             `json:"workflow_id" db:"workflow_id"`
	WorkflowStepID     string             `json:"workflow_step_id" db:"workflow_step_id"`
	AgentProfileID     string             `json:"agent_profile_id" db:"agent_profile_id"`
	ExecutorProfileID  string             `json:"executor_profile_id" db:"executor_profile_id"`
	Prompt             string             `json:"prompt" db:"prompt"`
	TaskTitleTemplate  string             `json:"task_title_template" db:"task_title_template"`
	Enabled            bool               `json:"enabled" db:"enabled"`
	MaxConcurrentRuns  int                `json:"max_concurrent_runs" db:"max_concurrent_runs"`
	ContinuationPolicy ContinuationPolicy `json:"continuation_policy" db:"continuation_policy"`
	// ContinuationTaskID is runtime state. It is intentionally omitted from
	// the public automation JSON because the saved task is not portable
	// configuration and may be deleted or replaced by the server.
	ContinuationTaskID string     `json:"-" db:"continuation_task_id"`
	WebhookSecret      string     `json:"-" db:"webhook_secret"`
	LastTriggeredAt    *time.Time `json:"last_triggered_at,omitempty" db:"last_triggered_at"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`

	// LegacyBoardCard reports that this automation was created while the
	// withdrawn execution_mode still decided where a firing landed, and that
	// its stored value is `task` — the DEFAULT, so this covers every
	// automation nobody explicitly set to `run`. Those firings used to put a
	// card on the kanban and no longer do; the UI uses this to say so once,
	// per workspace, instead of leaving the cards to just stop appearing.
	//
	// Derived in SQL (`execution_mode = 'task'`) rather than by reading the
	// column into a field, so the raw mode never enters the Go model and
	// cannot grow a firing-path branch: this is a migration-window notice,
	// not a second destination. See docs/specs/office/requirements/automations-settings.md
	// § Migration.
	LegacyBoardCard bool `json:"legacy_board_card" db:"legacy_board_card"`

	// Hydrated separately, not stored as columns on this table.
	Triggers []AutomationTrigger `json:"triggers" db:"-"`
	// Repositories is the canonical ordered repository/base-branch selection.
	// An empty list means task-owned scratch execution. RepositoryIDs remains a
	// response compatibility projection for older clients.
	Repositories  []AutomationRepository `json:"repositories" db:"-"`
	RepositoryIDs []string               `json:"repository_ids" db:"-"`
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

	// Summary is the tail of the agent's last message on the generated task,
	// read at list time and truncated for display. Hidden automation-run tasks
	// stay out of the board, while normal-task automation tasks follow the
	// ordinary task lists. The summary keeps the run row useful in either mode
	// when the task is not open. Empty when the run never produced a task or the
	// agent never spoke.
	Summary string `json:"summary,omitempty" db:"summary"`
	// SessionID is the run's primary conversation, empty when the task is gone
	// or never started one. The detail view mounts the transcript from it.
	SessionID    string       `json:"session_id,omitempty" db:"session_id"`
	TurnID       string       `json:"turn_id,omitempty" db:"turn_id"`
	ThreadAction ThreadAction `json:"thread_action,omitempty" db:"thread_action"`
	ThreadReason string       `json:"thread_reason,omitempty" db:"thread_reason"`
	DisplayTitle string       `json:"display_title,omitempty" db:"display_title"`
}

// WorkspaceAutomationRun is a run carrying just enough of its owning
// automation to be readable outside that automation's own settings page.
// The workspace-wide feed interleaves runs from every automation, so a row
// that knows only its automation_id is unattributable — the reader sees
// that something fired but not what fired it.
type WorkspaceAutomationRun struct {
	AutomationRun
	AutomationName string `json:"automation_name" db:"automation_name"`
}

// AutomationSummary is one automation's health, answered per automation rather
// than inferred from a capped feed: what it last said, and whether anything of
// its own is still running. The runs list reads exactly these two facts.
type AutomationSummary struct {
	AutomationID string `json:"automation_id"`
	// OpenRuns counts the runs still outstanding under the same definition the
	// concurrency cap uses, so "won't fire — still running" and the cap that
	// causes it can never disagree.
	OpenRuns int `json:"open_runs"`
	// LastRun is nil when the automation has never run, or when its runs have
	// all been deleted — both of which read as "no runs yet".
	LastRun *AutomationRun `json:"last_run,omitempty"`
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
	Branches    []string            `json:"branches,omitempty"` // head-branch glob filter; empty = all
}

// GitHubPRMergedTriggerConfig filters PR-merged events.
// all_repos: true = every repository matches; false + non-empty repos = those entries only;
// false + empty repos = matches nothing (the fail-closed default).
type GitHubPRMergedTriggerConfig struct {
	AllRepos     bool                `json:"all_repos"`
	Repos        []github.RepoFilter `json:"repos"`
	BaseBranches []string            `json:"base_branches"`
}

// WebhookTriggerConfig holds configuration for webhook triggers.
type WebhookTriggerConfig struct {
	FilterExpression string `json:"filter_expression,omitempty"`
}

// TaskOriginLookup answers the task workspace and whether it is hidden
// automation-run work. The merged-PR subscriber uses the same facts to avoid
// loops, while run cleanup uses them to leave visible automation-created tasks
// in the normal task lifecycle. ok=false means the task could not be resolved
// (absent or query error; the adapter collapses both into ok=false and logs at
// the appropriate level).
type TaskOriginLookup interface {
	TaskWorkspaceAndAutomationOrigin(ctx context.Context, taskID string) (
		workspaceID string, isAutomationRun bool, ok bool,
	)
}

// --- Request/response DTOs ---

// CreateAutomationRequest is the payload for creating an automation.
type CreateAutomationRequest struct {
	WorkspaceID        string                 `json:"workspace_id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	WorkflowID         string                 `json:"workflow_id"`
	WorkflowStepID     string                 `json:"workflow_step_id"`
	AgentProfileID     string                 `json:"agent_profile_id"`
	ExecutorProfileID  string                 `json:"executor_profile_id"`
	Repositories       []AutomationRepository `json:"repositories,omitempty"`
	RepositoryIDs      []string               `json:"repository_ids"`
	Prompt             string                 `json:"prompt"`
	TaskTitleTemplate  string                 `json:"task_title_template"`
	MaxConcurrentRuns  int                    `json:"max_concurrent_runs"`
	ContinuationPolicy ContinuationPolicy     `json:"continuation_policy,omitempty"`
	TaskMode           TaskMode               `json:"task_mode,omitempty"`
	RepositoryMode     RepositoryMode         `json:"repository_mode,omitempty"`
	Triggers           []CreateTriggerSpec    `json:"triggers"`
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
	// Repositories replaces the exact repository/base-branch list when non-nil.
	Repositories []AutomationRepository `json:"repositories,omitempty"`
	// RepositoryIDs replaces the automation's repository list when non-nil.
	// nil means "leave unchanged"; an explicit empty slice clears it.
	RepositoryIDs      []string            `json:"repository_ids,omitempty"`
	Prompt             *string             `json:"prompt,omitempty"`
	TaskTitleTemplate  *string             `json:"task_title_template,omitempty"`
	Enabled            *bool               `json:"enabled,omitempty"`
	MaxConcurrentRuns  *int                `json:"max_concurrent_runs,omitempty"`
	ContinuationPolicy *ContinuationPolicy `json:"continuation_policy,omitempty"`
	TaskMode           *TaskMode           `json:"task_mode,omitempty"`
	RepositoryMode     *RepositoryMode     `json:"repository_mode,omitempty"`
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

// CreateAutomationResponse wraps a newly-created automation and returns the
// generated webhook secret in plaintext exactly once, so the UI can show the
// user a value they can paste into an external system. Subsequent GET / list
// responses keep the secret hidden (WebhookSecret is `json:"-"`) — the
// dedicated reveal endpoint is the way back to it.
type CreateAutomationResponse struct {
	*Automation
	WebhookSecret string `json:"webhook_secret"`
}

// RevealWebhookSecretResponse returns the webhook secret for an automation.
type RevealWebhookSecretResponse struct {
	WebhookSecret string `json:"webhook_secret"`
}

// AutomationTriggeredEvent is published when a trigger fires.
type AutomationTriggeredEvent struct {
	RunID        string          `json:"run_id"`
	AutomationID string          `json:"automation_id"`
	TriggerID    string          `json:"trigger_id"`
	TriggerType  TriggerType     `json:"trigger_type"`
	TriggerData  json.RawMessage `json:"trigger_data"`
	DedupKey     string          `json:"dedup_key"`
}

// RepositoryLookup resolves a repository's workspace ownership for
// validating an automation's repository_ids. Structurally identical to the
// jira/linear/gitlab RepositoryLookup interfaces — the same
// repositoryLookupAdapter over the task service satisfies all of them.
type RepositoryLookup interface {
	GetRepository(ctx context.Context, id string) (workspaceID, defaultBranch string, ok bool)
}
