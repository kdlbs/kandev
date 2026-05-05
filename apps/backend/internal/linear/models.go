// Package linear implements the Linear integration: a single install-wide
// configuration, a GraphQL client for issues and workflow states, and the HTTP
// and WebSocket handlers that expose these capabilities to the frontend.
package linear

import "time"

// AuthMethodAPIKey is the only auth method Linear supports today: a Personal
// API Key sent as the `Authorization` header (no Bearer prefix). The constant
// exists so the wire format mirrors the Jira integration's `authMethod` field
// and leaves room for OAuth in the future.
const AuthMethodAPIKey = "api_key"

// LinearConfig is the install-wide configuration for the Linear integration.
// The API key is stored separately in the encrypted secret store under
// SecretKey.
type LinearConfig struct {
	AuthMethod     string `json:"authMethod" db:"auth_method"`
	DefaultTeamKey string `json:"defaultTeamKey" db:"default_team_key"`
	HasSecret      bool   `json:"hasSecret" db:"-"`
	// OrgSlug is captured from the most recent successful probe so the UI can
	// build canonical issue URLs (linear.app/<slug>/issue/<id>) without an
	// extra round-trip. Empty until the first probe succeeds.
	OrgSlug string `json:"orgSlug,omitempty" db:"org_slug"`
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
// Linear configuration. When Secret is empty on update, the existing secret
// is retained; when non-empty it replaces the stored value.
type SetConfigRequest struct {
	AuthMethod     string `json:"authMethod"`
	DefaultTeamKey string `json:"defaultTeamKey"`
	Secret         string `json:"secret"`
}

// TestConnectionResult reports what the backend learned when pinging Linear
// with the supplied credentials.
type TestConnectionResult struct {
	OK          bool   `json:"ok"`
	UserID      string `json:"userId,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	OrgSlug     string `json:"orgSlug,omitempty"`
	OrgName     string `json:"orgName,omitempty"`
	Error       string `json:"error,omitempty"`
}

// LinearIssue is the subset of Linear's issue payload that Kandev consumes.
// Kept small intentionally: the UI needs enough to prefill a task, show the
// current state, and surface a few familiar fields (assignee, priority, team)
// in the popover.
type LinearIssue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"` // e.g. "ENG-123"
	Title       string `json:"title"`
	Description string `json:"description"`
	// State mirrors Jira's status tuple (id, name, category) so frontend code
	// styling status pills can branch on Category without per-integration
	// switches. Linear's StateType values map onto: backlog/unstarted → "new",
	// started → "indeterminate", completed/canceled → "done".
	StateID       string                `json:"stateId"`
	StateName     string                `json:"stateName"`
	StateType     string                `json:"stateType"` // backlog | unstarted | started | completed | canceled | triage
	StateCategory string                `json:"stateCategory"`
	TeamID        string                `json:"teamId"`
	TeamKey       string                `json:"teamKey"`
	Priority      int                   `json:"priority"` // 0=none, 1=urgent, 2=high, 3=med, 4=low
	PriorityLabel string                `json:"priorityLabel,omitempty"`
	AssigneeName  string                `json:"assigneeName,omitempty"`
	AssigneeEmail string                `json:"assigneeEmail,omitempty"`
	AssigneeIcon  string                `json:"assigneeIcon,omitempty"`
	CreatorName   string                `json:"creatorName,omitempty"`
	CreatorIcon   string                `json:"creatorIcon,omitempty"`
	Updated       string                `json:"updated,omitempty"`
	URL           string                `json:"url"`
	States        []LinearWorkflowState `json:"states"`
}

// LinearWorkflowState is one of the team workflow states an issue can be
// transitioned into. Unlike Jira transitions (which are edges), Linear states
// are nodes — to "transition" we set the issue's stateId to one of the team's
// states. State IDs are stable per team.
type LinearWorkflowState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // backlog | unstarted | started | completed | canceled | triage
	Color    string `json:"color,omitempty"`
	Position int    `json:"position"`
}

// LinearTeam is the minimal shape used by the team selector on the settings
// page and by the issue browser to scope searches.
type LinearTeam struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// SearchFilter is a structured search filter used by SearchIssues. Linear has
// no JQL equivalent, so we expose a small set of structured fields that map
// cleanly to GraphQL filter inputs.
type SearchFilter struct {
	Query    string   `json:"query,omitempty"`    // free-text title/description/identifier match
	TeamKey  string   `json:"teamKey,omitempty"`  // restrict to one team
	StateIDs []string `json:"stateIds,omitempty"` // restrict to specific workflow states
	Assigned string   `json:"assigned,omitempty"` // "me" | "unassigned" | "" (any)
}

// SearchResult is a page of issues from a search. Linear uses cursor-based
// pagination (endCursor + hasNextPage), which we expose here under the same
// shape as the Jira SearchResult so the frontend pagination component can be
// reused.
type SearchResult struct {
	Issues        []LinearIssue `json:"issues"`
	MaxResults    int           `json:"maxResults"`
	IsLast        bool          `json:"isLast"`
	NextPageToken string        `json:"nextPageToken,omitempty"`
}

// SecretKey is the secret-store key used for the install-wide Linear API key.
// Centralised so the service, store and provider migration agree.
const SecretKey = "linear:singleton:token"

// LegacySecretKeyForWorkspace returns the pre-singleton per-workspace secret
// key. Only used by the one-shot startup migration in provider.go to copy an
// existing token over to SecretKey.
func LegacySecretKeyForWorkspace(workspaceID string) string {
	return "linear:" + workspaceID + ":token"
}

// DefaultIssueWatchPollInterval is the polling cadence assigned to a watcher
// when the caller does not specify one. Five minutes balances freshness
// against Linear rate limits when many workspaces have watches configured.
const DefaultIssueWatchPollInterval = 300

// IssueWatch configures periodic Linear search-polling. The filter is a
// structured SearchFilter (Linear has no JQL equivalent) persisted as JSON;
// the poller deserialises it back to SearchFilter at the store boundary.
//
// As with Jira, Linear issues have no repository affinity — the target
// workflow step's defaults determine where the resulting task runs.
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
	LastPolledAt        *time.Time   `json:"lastPolledAt,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time    `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time    `json:"updatedAt" db:"updated_at"`
}

// IssueWatchTask deduplicates task creation per (watch, issue) tuple. The
// UNIQUE constraint on (issue_watch_id, issue_identifier) prevents two
// concurrent pollers from racing to create duplicate tasks for the same
// issue. We key on Identifier (e.g. "ENG-123") rather than the GraphQL UUID
// because it's stable, what humans see, and present on every search result.
type IssueWatchTask struct {
	ID              string    `json:"id" db:"id"`
	IssueWatchID    string    `json:"issueWatchId" db:"issue_watch_id"`
	IssueIdentifier string    `json:"issueIdentifier" db:"issue_identifier"`
	IssueURL        string    `json:"issueUrl" db:"issue_url"`
	TaskID          string    `json:"taskId" db:"task_id"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
}

// NewLinearIssueEvent is published on the bus whenever the poller observes an
// issue matching a watch that has no existing dedup row. The orchestrator
// consumes this to create (and optionally auto-start) a Kandev task.
type NewLinearIssueEvent struct {
	IssueWatchID      string       `json:"issueWatchId"`
	WorkspaceID       string       `json:"workspaceId"`
	WorkflowID        string       `json:"workflowId"`
	WorkflowStepID    string       `json:"workflowStepId"`
	AgentProfileID    string       `json:"agentProfileId"`
	ExecutorProfileID string       `json:"executorProfileId"`
	Prompt            string       `json:"prompt"`
	Issue             *LinearIssue `json:"issue"`
}

// CreateIssueWatchRequest is the payload for POST /api/v1/linear/watches/issue.
type CreateIssueWatchRequest struct {
	WorkspaceID         string       `json:"workspaceId"`
	WorkflowID          string       `json:"workflowId"`
	WorkflowStepID      string       `json:"workflowStepId"`
	Filter              SearchFilter `json:"filter"`
	AgentProfileID      string       `json:"agentProfileId"`
	ExecutorProfileID   string       `json:"executorProfileId"`
	Prompt              string       `json:"prompt"`
	PollIntervalSeconds int          `json:"pollIntervalSeconds"`
	Enabled             *bool        `json:"enabled,omitempty"`
}

// UpdateIssueWatchRequest is the payload for PATCH /api/v1/linear/watches/issue/:id.
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
}
