// Package jira implements the Jira/Atlassian Cloud integration: a single
// install-wide configuration plus per-workspace JQL issue watchers, a REST
// client for tickets and transitions, and the HTTP and WebSocket handlers that
// expose these capabilities to the frontend.
package jira

import "time"

// Auth method identifiers persisted in JiraConfig.AuthMethod.
const (
	AuthMethodAPIToken      = "api_token"
	AuthMethodSessionCookie = "session_cookie"
)

// JiraConfig is the install-wide configuration for the Jira integration. The
// secret value (API token or session cookie) is stored separately in the
// encrypted secret store under SecretKey.
type JiraConfig struct {
	SiteURL           string `json:"siteUrl" db:"site_url"`
	Email             string `json:"email" db:"email"`
	AuthMethod        string `json:"authMethod" db:"auth_method"`
	DefaultProjectKey string `json:"defaultProjectKey" db:"default_project_key"`
	HasSecret         bool   `json:"hasSecret" db:"-"`
	// SecretExpiresAt is populated for session_cookie auth when the cookie is
	// a JWT (cloud.session.token / tenant.session.token). Nil for api_token or
	// opaque session cookies.
	SecretExpiresAt *time.Time `json:"secretExpiresAt,omitempty" db:"-"`
	// LastCheckedAt / LastOk / LastError are written by the background auth
	// poller. They let the UI render a "connected/disconnected + checked Xs ago"
	// indicator without doing its own probing.
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty" db:"last_checked_at"`
	LastOk        bool       `json:"lastOk" db:"last_ok"`
	LastError     string     `json:"lastError,omitempty" db:"last_error"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
}

// SetConfigRequest is the payload sent by the UI to create or update the Jira
// configuration. When Secret is empty on update, the existing secret is
// retained; when non-empty it replaces the stored value.
type SetConfigRequest struct {
	SiteURL           string `json:"siteUrl"`
	Email             string `json:"email"`
	AuthMethod        string `json:"authMethod"`
	DefaultProjectKey string `json:"defaultProjectKey"`
	Secret            string `json:"secret"`
}

// TestConnectionResult reports what the backend learned when pinging Jira with
// the supplied credentials. It is used both to verify newly-entered credentials
// and to surface a meaningful error to the UI when they fail.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	AccountID   string `json:"accountId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Error       string `json:"error,omitempty"`
}

// JiraTicket is the subset of Atlassian's issue payload that Kandev consumes.
// Kept small intentionally: the UI needs enough to prefill a task, show status,
// and surface a few familiar fields (assignee, reporter, priority) in the
// popover so users don't have to switch tabs to Jira.
type JiraTicket struct {
	Key            string            `json:"key"`
	Summary        string            `json:"summary"`
	Description    string            `json:"description"`
	StatusID       string            `json:"statusId"`
	StatusName     string            `json:"statusName"`
	StatusCategory string            `json:"statusCategory"` // "new" | "indeterminate" | "done"
	ProjectKey     string            `json:"projectKey"`
	IssueType      string            `json:"issueType"`
	IssueTypeIcon  string            `json:"issueTypeIcon,omitempty"`
	Priority       string            `json:"priority,omitempty"`
	PriorityIcon   string            `json:"priorityIcon,omitempty"`
	AssigneeName   string            `json:"assigneeName,omitempty"`
	AssigneeAvatar string            `json:"assigneeAvatar,omitempty"`
	ReporterName   string            `json:"reporterName,omitempty"`
	ReporterAvatar string            `json:"reporterAvatar,omitempty"`
	Updated        string            `json:"updated,omitempty"`
	URL            string            `json:"url"`
	Transitions    []JiraTransition  `json:"transitions"`
	Fields         map[string]string `json:"fields,omitempty"`
}

// JiraTransition is one of the workflow transitions available for a ticket at
// the time of fetch. Transition IDs are stable within a project but the set of
// available transitions changes with ticket status, so the UI must re-fetch
// after a transition.
type JiraTransition struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ToStatusID   string `json:"toStatusId"`
	ToStatusName string `json:"toStatusName"`
}

// JiraProject is the minimal shape used by the project selector on the settings
// page.
type JiraProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	ID   string `json:"id"`
}

// SearchResult is a page of tickets from a JQL search. Atlassian's
// /rest/api/3/search/jql endpoint is token-paginated and returns no total
// count, so the UI relies on IsLast and NextPageToken to walk pages.
type SearchResult struct {
	Tickets       []JiraTicket `json:"tickets"`
	MaxResults    int          `json:"maxResults"`
	IsLast        bool         `json:"isLast"`
	NextPageToken string       `json:"nextPageToken,omitempty"`
}

// SecretKey is the secret-store key used for the install-wide Jira token.
// Centralised so that the service, store and provider migration agree.
const SecretKey = "jira:singleton:token"

// LegacySecretKeyForWorkspace returns the pre-singleton per-workspace secret
// key. Only used by the one-shot startup migration in provider.go to copy an
// existing token over to SecretKey.
func LegacySecretKeyForWorkspace(workspaceID string) string {
	return "jira:" + workspaceID + ":token"
}

// DefaultIssueWatchPollInterval is the polling cadence assigned to a watcher
// when the caller does not specify one. Five minutes balances freshness against
// Atlassian rate limits when many workspaces have watches configured.
const DefaultIssueWatchPollInterval = 300

// IssueWatch configures periodic JQL polling: a workspace-scoped watcher runs
// the JQL on a schedule and emits a NewJiraIssueEvent for each matching ticket
// the orchestrator hasn't already turned into a Kandev task.
//
// Unlike the GitHub equivalent, JIRA issues have no repository affinity — the
// target workflow step's defaults determine where the resulting task runs, so
// there's no `repos` column.
type IssueWatch struct {
	ID                  string     `json:"id" db:"id"`
	WorkspaceID         string     `json:"workspaceId" db:"workspace_id"`
	WorkflowID          string     `json:"workflowId" db:"workflow_id"`
	WorkflowStepID      string     `json:"workflowStepId" db:"workflow_step_id"`
	JQL                 string     `json:"jql" db:"jql"`
	AgentProfileID      string     `json:"agentProfileId" db:"agent_profile_id"`
	ExecutorProfileID   string     `json:"executorProfileId" db:"executor_profile_id"`
	Prompt              string     `json:"prompt" db:"prompt"`
	Enabled             bool       `json:"enabled" db:"enabled"`
	PollIntervalSeconds int        `json:"pollIntervalSeconds" db:"poll_interval_seconds"`
	LastPolledAt        *time.Time `json:"lastPolledAt,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time  `json:"updatedAt" db:"updated_at"`
}

// IssueWatchTask deduplicates task creation per (watch, ticket) tuple. The
// UNIQUE constraint on (issue_watch_id, issue_key) prevents two concurrent
// pollers from racing to create duplicate tasks for the same ticket.
type IssueWatchTask struct {
	ID           string    `json:"id" db:"id"`
	IssueWatchID string    `json:"issueWatchId" db:"issue_watch_id"`
	IssueKey     string    `json:"issueKey" db:"issue_key"`
	IssueURL     string    `json:"issueUrl" db:"issue_url"`
	TaskID       string    `json:"taskId" db:"task_id"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
}

// NewJiraIssueEvent is published on the bus whenever the poller observes a
// ticket matching a watch that has no existing dedup row. The orchestrator
// consumes this to create (and optionally auto-start) a Kandev task.
type NewJiraIssueEvent struct {
	IssueWatchID      string      `json:"issueWatchId"`
	WorkspaceID       string      `json:"workspaceId"`
	WorkflowID        string      `json:"workflowId"`
	WorkflowStepID    string      `json:"workflowStepId"`
	AgentProfileID    string      `json:"agentProfileId"`
	ExecutorProfileID string      `json:"executorProfileId"`
	Prompt            string      `json:"prompt"`
	Issue             *JiraTicket `json:"issue"`
}

// CreateIssueWatchRequest is the payload for POST /api/v1/jira/watches/issue.
type CreateIssueWatchRequest struct {
	WorkspaceID         string `json:"workspaceId"`
	WorkflowID          string `json:"workflowId"`
	WorkflowStepID      string `json:"workflowStepId"`
	JQL                 string `json:"jql"`
	AgentProfileID      string `json:"agentProfileId"`
	ExecutorProfileID   string `json:"executorProfileId"`
	Prompt              string `json:"prompt"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	Enabled             *bool  `json:"enabled,omitempty"`
}

// UpdateIssueWatchRequest is the payload for PATCH /api/v1/jira/watches/issue/:id.
// All fields are pointers so the caller can omit ones it doesn't want to change.
type UpdateIssueWatchRequest struct {
	WorkflowID          *string `json:"workflowId,omitempty"`
	WorkflowStepID      *string `json:"workflowStepId,omitempty"`
	JQL                 *string `json:"jql,omitempty"`
	AgentProfileID      *string `json:"agentProfileId,omitempty"`
	ExecutorProfileID   *string `json:"executorProfileId,omitempty"`
	Prompt              *string `json:"prompt,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
	PollIntervalSeconds *int    `json:"pollIntervalSeconds,omitempty"`
}
