// Package jira implements the Jira/Atlassian Cloud integration: workspace-scoped
// configuration storage, a REST client for tickets and transitions, and the HTTP
// and WebSocket handlers that expose these capabilities to the frontend.
package jira

import "time"

// Auth method identifiers persisted in JiraConfig.AuthMethod.
const (
	AuthMethodAPIToken      = "api_token"
	AuthMethodSessionCookie = "session_cookie"
)

// JiraConfig is the workspace-scoped configuration for the Jira integration.
// The secret value (API token or session cookie) is stored separately in the
// encrypted secret store under the key returned by SecretKeyForWorkspace.
type JiraConfig struct {
	WorkspaceID       string `json:"workspaceId" db:"workspace_id"`
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

// SetConfigRequest is the payload sent by the UI to create or update the
// workspace's Jira configuration. When Secret is empty on update, the existing
// secret is retained; when non-empty it replaces the stored value.
type SetConfigRequest struct {
	WorkspaceID       string `json:"workspaceId"`
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

// SearchResult is a page of tickets from a JQL search, plus pagination metadata
// so the UI can render page controls without a second count request.
type SearchResult struct {
	Tickets    []JiraTicket `json:"tickets"`
	Total      int          `json:"total"`
	StartAt    int          `json:"startAt"`
	MaxResults int          `json:"maxResults"`
}

// SecretKeyForWorkspace returns the secret-store key used for the Jira token of
// a given workspace. Centralised so that the service and the store agree.
func SecretKeyForWorkspace(workspaceID string) string {
	return "jira:" + workspaceID + ":token"
}
