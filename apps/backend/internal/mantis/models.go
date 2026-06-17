// Package mantis implements the Mantis Bug Tracker integration: a per-workspace
// configuration, a REST client for issues and transitions, and the HTTP and
// WebSocket handlers that expose these capabilities to the frontend.
//
// Unlike the Jira and Linear integrations — which are install-wide singletons —
// Mantis is configured per workspace because deployments tend to be team- or
// project-specific (self-hosted instances, on-prem behind a VPN, etc.). Each
// workspace can point at a different Mantis endpoint with its own API token.
package mantis

import (
	"time"

	"github.com/kandev/kandev/internal/integrations/optional"
)

// AuthMethodAPIToken is the only auth method Mantis supports today: a Personal
// API Token created from the user's Mantis account page and sent in the
// `Authorization` header. The constant exists so the wire format mirrors the
// Jira/Linear `authMethod` field and leaves room for HTTP Basic in the future.
const AuthMethodAPIToken = "api_token"

// MantisConfig is the per-workspace configuration for the Mantis integration.
// The API token is stored separately in the encrypted secret store under
// SecretKeyForWorkspace(WorkspaceID).
type MantisConfig struct {
	WorkspaceID      string `json:"workspaceId" db:"workspace_id"`
	BaseURL          string `json:"baseUrl" db:"base_url"`
	Username         string `json:"username" db:"username"`
	AuthMethod       string `json:"authMethod" db:"auth_method"`
	DefaultProjectID int    `json:"defaultProjectId" db:"default_project_id"`
	HasSecret        bool   `json:"hasSecret" db:"-"`
	// LastCheckedAt / LastOk / LastError are written by the background auth
	// poller. They let the UI render a "connected/disconnected + checked Xs ago"
	// indicator without doing its own probing.
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty" db:"last_checked_at"`
	LastOk        bool       `json:"lastOk" db:"last_ok"`
	LastError     string     `json:"lastError,omitempty" db:"last_error"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
}

// SecretKeyForWorkspace returns the secret-store key used for a workspace's
// Mantis API token. Unlike Jira/Linear (which are install-wide singletons),
// Mantis is configured per workspace so each row gets its own secret key.
func SecretKeyForWorkspace(workspaceID string) string {
	return "mantis:" + workspaceID + ":token"
}

// MantisFilter is a structured search filter used by issue watches and the
// search UI. Mantis exposes a `filter_search` REST endpoint that takes the
// same dimensions; the poller serializes the filter to JSON for storage and
// translates it to query parameters at request time.
//
// All fields are optional; an empty filter matches every issue the API token
// can see. Watch creation rejects fully-empty filters via filterIsEmpty.
type MantisFilter struct {
	// ProjectID restricts to a single Mantis project. 0 means "all projects
	// the API token can read".
	ProjectID int `json:"projectId,omitempty"`
	// CategoryName filters by category label (Mantis categories are
	// per-project strings, not stable IDs across projects).
	CategoryName string `json:"categoryName,omitempty"`
	// StatusIDs is the set of Mantis status numerics to include. Empty
	// matches every status. Standard Mantis values: 10=new, 20=feedback,
	// 30=acknowledged, 40=confirmed, 50=assigned, 80=resolved, 90=closed.
	StatusIDs []int `json:"statusIds,omitempty"`
	// HandlerUsername restricts to issues assigned to this Mantis user.
	// Empty means any handler (including unassigned).
	HandlerUsername string `json:"handlerUsername,omitempty"`
	// ReporterUsername restricts to issues reported by this Mantis user.
	// Empty means any reporter.
	ReporterUsername string `json:"reporterUsername,omitempty"`
	// SearchText is a free-text match against summary and description.
	SearchText string `json:"searchText,omitempty"`
	// PriorityIDs is the set of Mantis priority numerics to include. Empty
	// matches every priority. Standard values: 10=none, 20=low, 30=normal,
	// 40=high, 50=urgent, 60=immediate.
	PriorityIDs []int `json:"priorityIds,omitempty"`
}

// MantisIssue is the subset of Mantis's bug payload that Kandev consumes.
// Kept small intentionally: the UI needs enough to prefill a task, show the
// current status, and surface a few familiar fields (handler, reporter,
// priority) in the popover so users don't have to switch tabs to Mantis.
type MantisIssue struct {
	// ID is the Mantis bug number — exposed as a string for wire-format
	// parity with Jira (key) and Linear (identifier). The numeric form is
	// canonical inside Mantis URLs (view.php?id=NNNN).
	ID          string `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	// Status mirrors Jira's status tuple (id, name, category) so frontend
	// code styling status pills can branch on Category without per-integration
	// switches. Mantis status numerics map onto: 10/20/30 → "new",
	// 40/50/60/70 → "indeterminate", 80/90 → "done".
	StatusID       int    `json:"statusId"`
	StatusName     string `json:"statusName"`
	StatusCategory string `json:"statusCategory"`
	ProjectID      int    `json:"projectId"`
	ProjectName    string `json:"projectName"`
	CategoryName   string `json:"categoryName,omitempty"`
	Priority       string `json:"priority,omitempty"`
	Severity       string `json:"severity,omitempty"`
	HandlerName    string `json:"handlerName,omitempty"`
	HandlerEmail   string `json:"handlerEmail,omitempty"`
	ReporterName   string `json:"reporterName,omitempty"`
	ReporterEmail  string `json:"reporterEmail,omitempty"`
	Updated        string `json:"updated,omitempty"`
	URL            string `json:"url"`
}

// SecretKey is the secret-store key prefix used for Mantis API tokens. The
// per-workspace key is built via SecretKeyForWorkspace; this constant exists
// for symmetry with Jira/Linear and as a sentinel for migration tooling.
const SecretKey = "mantis"

// DefaultIssueWatchPollInterval is the polling cadence assigned to a watcher
// when the caller does not specify one. Five minutes balances freshness against
// the load self-hosted Mantis instances can absorb when many workspaces have
// watches configured.
const DefaultIssueWatchPollInterval = 300

// MantisIssueWatch configures periodic Mantis polling: a workspace-scoped
// watcher runs the filter on a schedule and emits a NewMantisIssueEvent for
// each matching bug the orchestrator hasn't already turned into a Kandev task.
//
// As with Jira and Linear, Mantis issues have no repository affinity — the
// target workflow step's defaults determine where the resulting task runs.
type MantisIssueWatch struct {
	ID                  string       `json:"id" db:"id"`
	WorkspaceID         string       `json:"workspaceId" db:"workspace_id"`
	WorkflowID          string       `json:"workflowId" db:"workflow_id"`
	WorkflowStepID      string       `json:"workflowStepId" db:"workflow_step_id"`
	Filter              MantisFilter `json:"filter"`
	AgentProfileID      string       `json:"agentProfileId" db:"agent_profile_id"`
	ExecutorProfileID   string       `json:"executorProfileId" db:"executor_profile_id"`
	Prompt              string       `json:"prompt" db:"prompt"`
	Enabled             bool         `json:"enabled" db:"enabled"`
	PollIntervalSeconds int          `json:"pollIntervalSeconds" db:"poll_interval_seconds"`
	// MaxInflightTasks caps how many open watcher-created tasks this watch can
	// hold at once. nil = uncapped. Values <= 0 are rejected at the API layer.
	// See docs/specs/throttle-watcher-fanout/spec.md for the open-task definition.
	MaxInflightTasks *int       `json:"maxInflightTasks,omitempty" db:"max_inflight_tasks"`
	LastPolledAt     *time.Time `json:"lastPolledAt,omitempty" db:"last_polled_at"`
	// LastError / LastErrorAt are stamped when the dispatch pipeline self-
	// heals the watcher (e.g. the bound agent profile was soft-deleted).
	// Empty for a healthy watcher.
	LastError   string     `json:"lastError,omitempty" db:"last_error"`
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty" db:"last_error_at"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time  `json:"updatedAt" db:"updated_at"`
}

// MantisIssueWatchTask deduplicates task creation per (watch, issue) tuple. The
// UNIQUE constraint on (issue_watch_id, issue_id) prevents two concurrent
// pollers from racing to create duplicate tasks for the same bug.
type MantisIssueWatchTask struct {
	ID           string    `json:"id" db:"id"`
	IssueWatchID string    `json:"issueWatchId" db:"issue_watch_id"`
	IssueID      string    `json:"issueId" db:"issue_id"`
	IssueURL     string    `json:"issueUrl" db:"issue_url"`
	TaskID       string    `json:"taskId" db:"task_id"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
}

// NewMantisIssueEvent is published on the bus whenever the poller observes a
// bug matching a watch that has no existing dedup row. The orchestrator
// consumes this to create (and optionally auto-start) a Kandev task.
type NewMantisIssueEvent struct {
	IssueWatchID      string `json:"issueWatchId"`
	WorkspaceID       string `json:"workspaceId"`
	WorkflowID        string `json:"workflowId"`
	WorkflowStepID    string `json:"workflowStepId"`
	AgentProfileID    string `json:"agentProfileId"`
	ExecutorProfileID string `json:"executorProfileId"`
	Prompt            string `json:"prompt"`
	// MaxInflightTasks mirrors the watch row's per-watcher throttle cap so the
	// orchestrator's gate can read it without loading the row again. nil =
	// uncapped.
	MaxInflightTasks *int         `json:"maxInflightTasks,omitempty"`
	Issue            *MantisIssue `json:"issue"`
}

// SetConfigRequest is the payload sent by the UI to create or update the
// Mantis configuration for a workspace. When Secret is empty on update, the
// existing secret is retained; when non-empty it replaces the stored value.
type SetConfigRequest struct {
	WorkspaceID      string `json:"workspaceId"`
	BaseURL          string `json:"baseUrl"`
	Username         string `json:"username"`
	AuthMethod       string `json:"authMethod"`
	DefaultProjectID int    `json:"defaultProjectId"`
	Secret           string `json:"secret"`
}

// TestConnectionResult reports what the backend learned when pinging Mantis
// with the supplied credentials. It is used both to verify newly-entered
// credentials and to surface a meaningful error to the UI when they fail.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	UserID      int    `json:"userId,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Error       string `json:"error,omitempty"`
}

// CreateIssueWatchRequest is the payload for POST /api/v1/mantis/watches/issue.
type CreateIssueWatchRequest struct {
	WorkspaceID         string       `json:"workspaceId"`
	WorkflowID          string       `json:"workflowId"`
	WorkflowStepID      string       `json:"workflowStepId"`
	Filter              MantisFilter `json:"filter"`
	AgentProfileID      string       `json:"agentProfileId"`
	ExecutorProfileID   string       `json:"executorProfileId"`
	Prompt              string       `json:"prompt"`
	PollIntervalSeconds int          `json:"pollIntervalSeconds"`
	MaxInflightTasks    *int         `json:"maxInflightTasks,omitempty"`
	Enabled             *bool        `json:"enabled,omitempty"`
}

// UpdateIssueWatchRequest is the payload for PATCH
// /api/v1/mantis/watches/issue/:id. Most fields are pointers so callers can
// omit the ones they don't want to change. MaxInflightTasks uses optional.Int
// for tri-state PATCH semantics (absent = unchanged, null = uncapped, positive
// int = cap).
type UpdateIssueWatchRequest struct {
	WorkflowID          *string       `json:"workflowId,omitempty"`
	WorkflowStepID      *string       `json:"workflowStepId,omitempty"`
	Filter              *MantisFilter `json:"filter,omitempty"`
	AgentProfileID      *string       `json:"agentProfileId,omitempty"`
	ExecutorProfileID   *string       `json:"executorProfileId,omitempty"`
	Prompt              *string       `json:"prompt,omitempty"`
	Enabled             *bool         `json:"enabled,omitempty"`
	PollIntervalSeconds *int          `json:"pollIntervalSeconds,omitempty"`
	// MaxInflightTasks is tri-state so a partial PATCH that omits the field
	// leaves the cap unchanged (a plain *int can't tell "omitted" from
	// "null"). Absent = unchanged, null = uncapped, positive int = cap.
	MaxInflightTasks optional.Int `json:"maxInflightTasks"`
}
