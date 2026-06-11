// Package sentry implements the Sentry integration (Phase 1: configure + browse).
// A single install-wide configuration, a REST client for projects and issues,
// and the HTTP handlers that expose these capabilities to the frontend.
package sentry

import (
	"time"

	"github.com/kandev/kandev/internal/integrations/optional"
)

// AuthMethodAuthToken is the only auth method Sentry supports in Phase 1: a
// user or organization auth token sent as `Authorization: Bearer <token>`.
const AuthMethodAuthToken = "auth_token"

// SecretKey is the secret-store key used for the install-wide Sentry token.
const SecretKey = "sentry:singleton:token"

// SentryConfig is the install-wide configuration for the Sentry integration.
// The token is stored separately in the encrypted secret store under SecretKey.
type SentryConfig struct {
	AuthMethod string `json:"authMethod" db:"auth_method"`
	// URL is the base URL of the Sentry instance (e.g. https://sentry.io for
	// SaaS, or a self-hosted host). The REST client appends /api/0. Defaults
	// to the sentry.io SaaS endpoint when left blank.
	URL       string `json:"url" db:"url"`
	HasSecret bool   `json:"hasSecret" db:"-"`
	// LastCheckedAt / LastOk / LastError are written by the background auth
	// poller so the UI can render a "connected/disconnected + checked Xs ago"
	// indicator without doing its own probing.
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty" db:"last_checked_at"`
	LastOk        bool       `json:"lastOk" db:"last_ok"`
	LastError     string     `json:"lastError,omitempty" db:"last_error"`
	CreatedAt     time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time  `json:"updatedAt" db:"updated_at"`
}

// SetConfigRequest is the payload sent by the UI to create or update the
// Sentry configuration. When Secret is empty on update, the existing secret
// is retained; when non-empty it replaces the stored value.
type SetConfigRequest struct {
	AuthMethod string `json:"authMethod"`
	// URL is the Sentry instance base URL. Optional: blank defaults to the
	// sentry.io SaaS endpoint, preserving the prior single-tenant behavior.
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

// TestConnectionResult reports what the backend learned when pinging Sentry
// with the supplied credentials.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	UserID      string `json:"userId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Error       string `json:"error,omitempty"`
}

// SentryOrganization is the minimal shape used by the organization selector on
// the settings page.
type SentryOrganization struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// SentryProject is the minimal shape used by the project selector on the
// settings page and by the issue browser to scope searches.
type SentryProject struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	OrgSlug string `json:"orgSlug"`
}

// SentryIssue is the subset of Sentry's issue payload Kandev consumes.
type SentryIssue struct {
	ID          string `json:"id"`
	ShortID     string `json:"shortId"`
	Title       string `json:"title"`
	Culprit     string `json:"culprit,omitempty"`
	Permalink   string `json:"permalink"`
	ProjectSlug string `json:"projectSlug"`
	ProjectName string `json:"projectName,omitempty"`
	Level       string `json:"level"`
	Status      string `json:"status"`
	// Count is returned by Sentry as a string ("1234"), preserved as-is.
	Count        string `json:"count,omitempty"`
	UserCount    int    `json:"userCount,omitempty"`
	FirstSeen    string `json:"firstSeen,omitempty"`
	LastSeen     string `json:"lastSeen,omitempty"`
	AssigneeName string `json:"assigneeName,omitempty"`
}

// SearchFilter is a structured search filter used by SearchIssues. Sentry
// expresses level/status filters by appending tokens to the free-text `query`
// param; we expose them as structured fields so the UI can render multi-select
// chips and the backend builds the right query string.
type SearchFilter struct {
	OrgSlug     string   `json:"orgSlug"`
	ProjectSlug string   `json:"projectSlug,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Levels      []string `json:"levels,omitempty"`
	Statuses    []string `json:"statuses,omitempty"`
	Query       string   `json:"query,omitempty"`
	StatsPeriod string   `json:"statsPeriod,omitempty"`
}

// SearchResult is a page of issues from a search. Sentry uses opaque cursors
// embedded in the Link header for pagination; NextPageToken carries that
// cursor verbatim so the next call can pass it back as `?cursor=...`.
type SearchResult struct {
	Issues        []SentryIssue `json:"issues"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
	IsLast        bool          `json:"isLast"`
}

// DefaultIssueWatchPollInterval is the polling cadence assigned to a watcher
// when the caller does not specify one. Five minutes balances freshness
// against Sentry rate limits when many watches are configured.
const DefaultIssueWatchPollInterval = 300

// MinIssueWatchPollInterval / MaxIssueWatchPollInterval bound the per-watch
// search re-run cadence.
const (
	MinIssueWatchPollInterval = 60
	MaxIssueWatchPollInterval = 3600
)

// IssueWatch persists a Sentry issue-search filter that becomes a kandev task
// whenever a new matching issue appears. Mirrors the Linear/Jira shape so the
// orchestrator's WatcherSource pipeline applies uniformly.
type IssueWatch struct {
	ID                  string       `json:"id" db:"id"`
	WorkspaceID         string       `json:"workspaceId" db:"workspace_id"`
	WorkflowID          string       `json:"workflowId" db:"workflow_id"`
	WorkflowStepID      string       `json:"workflowStepId" db:"workflow_step_id"`
	Filter              SearchFilter `json:"filter"`
	AgentProfileID      string       `json:"agentProfileId" db:"agent_profile_id"`
	ExecutorProfileID   string       `json:"executorProfileId" db:"executor_profile_id"`
	Prompt              string       `json:"prompt" db:"prompt"`
	Enabled             bool         `json:"enabled" db:"enabled"`
	PollIntervalSeconds int          `json:"pollIntervalSeconds" db:"poll_interval_seconds"`
	// MaxInflightTasks caps how many open watcher-created tasks this watch can
	// hold at once. nil = uncapped. Values <= 0 are rejected at the API layer.
	MaxInflightTasks *int       `json:"maxInflightTasks,omitempty" db:"max_inflight_tasks"`
	LastPolledAt     *time.Time `json:"lastPolledAt,omitempty" db:"last_polled_at"`
	// LastError / LastErrorAt are stamped when the dispatch coordinator
	// self-heals an orphaned watch (e.g. its bound agent profile was
	// deleted). Surfaced to the settings UI so the user sees why a watch
	// was disabled. Empty string / nil when the watch is healthy.
	LastError   string     `json:"lastError,omitempty" db:"last_error"`
	LastErrorAt *time.Time `json:"lastErrorAt,omitempty" db:"last_error_at"`
	CreatedAt   time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time  `json:"updatedAt" db:"updated_at"`
}

// IssueWatchTask deduplicates task creation per (watch, issue) tuple. Keyed on
// short_id (e.g. "PROJ-123") because it is stable, human-readable, and present
// on every Sentry search result.
type IssueWatchTask struct {
	ID           string    `json:"id" db:"id"`
	IssueWatchID string    `json:"issueWatchId" db:"issue_watch_id"`
	IssueShortID string    `json:"issueShortId" db:"issue_short_id"`
	IssueURL     string    `json:"issueUrl" db:"issue_url"`
	TaskID       string    `json:"taskId" db:"task_id"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
}

// NewSentryIssueEvent is published on the bus whenever the poller observes an
// issue matching a watch that has no existing dedup row. The orchestrator
// consumes this to create (and optionally auto-start) a Kandev task.
type NewSentryIssueEvent struct {
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
	Issue            *SentryIssue `json:"issue"`
}

// CreateIssueWatchRequest is the payload for POST /api/v1/sentry/watches/issue.
type CreateIssueWatchRequest struct {
	WorkspaceID         string       `json:"workspaceId"`
	WorkflowID          string       `json:"workflowId"`
	WorkflowStepID      string       `json:"workflowStepId"`
	Filter              SearchFilter `json:"filter"`
	AgentProfileID      string       `json:"agentProfileId"`
	ExecutorProfileID   string       `json:"executorProfileId"`
	Prompt              string       `json:"prompt"`
	PollIntervalSeconds int          `json:"pollIntervalSeconds"`
	MaxInflightTasks    *int         `json:"maxInflightTasks,omitempty"`
	Enabled             *bool        `json:"enabled,omitempty"`
}

// UpdateIssueWatchRequest is the payload for PATCH /api/v1/sentry/watches/issue/:id.
// All fields are pointers so the caller can omit ones it doesn't want to change.
type UpdateIssueWatchRequest struct {
	WorkflowID          *string       `json:"workflowId,omitempty"`
	WorkflowStepID      *string       `json:"workflowStepId,omitempty"`
	Filter              *SearchFilter `json:"filter,omitempty"`
	AgentProfileID      *string       `json:"agentProfileId,omitempty"`
	ExecutorProfileID   *string       `json:"executorProfileId,omitempty"`
	Prompt              *string       `json:"prompt,omitempty"`
	Enabled             *bool         `json:"enabled,omitempty"`
	PollIntervalSeconds *int          `json:"pollIntervalSeconds,omitempty"`
	// MaxInflightTasks is tri-state so a partial PATCH that omits the field
	// leaves the cap unchanged. Absent = unchanged, null = uncapped, positive
	// int = cap.
	MaxInflightTasks optional.Int `json:"maxInflightTasks"`
}
