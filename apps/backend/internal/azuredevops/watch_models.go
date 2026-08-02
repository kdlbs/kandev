package azuredevops

import (
	"errors"
	"time"
)

const (
	CleanupPolicyAuto   = "auto"
	CleanupPolicyAlways = "always"
	CleanupPolicyNever  = "never"

	defaultWatchPollIntervalSeconds = 300
	minWatchPollIntervalSeconds     = 60
)

var (
	ErrWatchNotFound       = errors.New("azure devops: watcher not found")
	ErrWatchOwnershipLost  = errors.New("azure devops: watcher ownership lost")
	ErrReservationNotFound = errors.New("azure devops: watcher reservation not found")
)

func IsValidCleanupPolicy(value string) bool {
	switch value {
	case "", CleanupPolicyAuto, CleanupPolicyAlways, CleanupPolicyNever:
		return true
	default:
		return false
	}
}

func normalizeCleanupPolicy(value string) string {
	if value == "" {
		return CleanupPolicyAuto
	}
	return value
}

func normalizePollInterval(value int) int {
	if value <= 0 {
		return defaultWatchPollIntervalSeconds
	}
	if value < minWatchPollIntervalSeconds {
		return minWatchPollIntervalSeconds
	}
	return value
}

func validateMaxInflightTasks(value *int) error {
	if value != nil && *value < 0 {
		return errors.New("max_inflight_tasks must be zero or greater")
	}
	return nil
}

// WorkItemWatch configures periodic matching of Azure Boards work items.
type WorkItemWatch struct {
	ID                  string     `json:"id" db:"id"`
	WorkspaceID         string     `json:"workspaceId" db:"workspace_id"`
	WorkflowID          string     `json:"workflowId" db:"workflow_id"`
	WorkflowStepID      string     `json:"workflowStepId" db:"workflow_step_id"`
	ProjectID           string     `json:"projectId" db:"project_id"`
	WIQL                string     `json:"wiql" db:"wiql"`
	RepositoryID        string     `json:"repositoryId" db:"repository_id"`
	BaseBranch          string     `json:"baseBranch" db:"base_branch"`
	AgentProfileID      string     `json:"agentProfileId" db:"agent_profile_id"`
	ExecutorProfileID   string     `json:"executorProfileId" db:"executor_profile_id"`
	Prompt              string     `json:"prompt" db:"prompt"`
	Enabled             bool       `json:"enabled" db:"enabled"`
	PollIntervalSeconds int        `json:"pollIntervalSeconds" db:"poll_interval_seconds"`
	CleanupPolicy       string     `json:"cleanupPolicy" db:"cleanup_policy"`
	MaxInflightTasks    *int       `json:"maxInflightTasks,omitempty" db:"max_inflight_tasks"`
	Generation          int64      `json:"-" db:"generation"`
	Deleting            bool       `json:"-" db:"deleting"`
	LastError           string     `json:"lastError,omitempty" db:"last_error"`
	LastErrorAt         *time.Time `json:"lastErrorAt,omitempty" db:"last_error_at"`
	LastPolledAt        *time.Time `json:"lastPolledAt,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time  `json:"updatedAt" db:"updated_at"`
}

// PullRequestWatch configures periodic matching of Azure Repos pull requests.
type PullRequestWatch struct {
	ID                  string     `json:"id" db:"id"`
	WorkspaceID         string     `json:"workspaceId" db:"workspace_id"`
	WorkflowID          string     `json:"workflowId" db:"workflow_id"`
	WorkflowStepID      string     `json:"workflowStepId" db:"workflow_step_id"`
	ProjectID           string     `json:"projectId" db:"project_id"`
	AzureRepositoryID   string     `json:"azureRepositoryId,omitempty" db:"azure_repository_id"`
	Status              string     `json:"status" db:"status"`
	CreatorID           string     `json:"creatorId,omitempty" db:"creator_id"`
	ReviewerID          string     `json:"reviewerId,omitempty" db:"reviewer_id"`
	RepositoryID        string     `json:"repositoryId" db:"repository_id"`
	BaseBranch          string     `json:"baseBranch" db:"base_branch"`
	AgentProfileID      string     `json:"agentProfileId" db:"agent_profile_id"`
	ExecutorProfileID   string     `json:"executorProfileId" db:"executor_profile_id"`
	Prompt              string     `json:"prompt" db:"prompt"`
	Enabled             bool       `json:"enabled" db:"enabled"`
	PollIntervalSeconds int        `json:"pollIntervalSeconds" db:"poll_interval_seconds"`
	CleanupPolicy       string     `json:"cleanupPolicy" db:"cleanup_policy"`
	MaxInflightTasks    *int       `json:"maxInflightTasks,omitempty" db:"max_inflight_tasks"`
	Generation          int64      `json:"-" db:"generation"`
	Deleting            bool       `json:"-" db:"deleting"`
	LastError           string     `json:"lastError,omitempty" db:"last_error"`
	LastErrorAt         *time.Time `json:"lastErrorAt,omitempty" db:"last_error_at"`
	LastPolledAt        *time.Time `json:"lastPolledAt,omitempty" db:"last_polled_at"`
	CreatedAt           time.Time  `json:"createdAt" db:"created_at"`
	UpdatedAt           time.Time  `json:"updatedAt" db:"updated_at"`
}

type WatchResetResult struct {
	Generation int64
	TaskIDs    []string
}

type CreateWorkItemWatchRequest struct {
	WorkspaceID         string `json:"workspaceId"`
	WorkflowID          string `json:"workflowId"`
	WorkflowStepID      string `json:"workflowStepId"`
	ProjectID           string `json:"projectId"`
	WIQL                string `json:"wiql"`
	RepositoryID        string `json:"repositoryId"`
	BaseBranch          string `json:"baseBranch"`
	AgentProfileID      string `json:"agentProfileId"`
	ExecutorProfileID   string `json:"executorProfileId"`
	Prompt              string `json:"prompt"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	CleanupPolicy       string `json:"cleanupPolicy"`
	MaxInflightTasks    *int   `json:"maxInflightTasks,omitempty"`
}

type UpdateWorkItemWatchRequest struct {
	WorkflowID          *string `json:"workflowId,omitempty"`
	WorkflowStepID      *string `json:"workflowStepId,omitempty"`
	ProjectID           *string `json:"projectId,omitempty"`
	WIQL                *string `json:"wiql,omitempty"`
	RepositoryID        *string `json:"repositoryId,omitempty"`
	BaseBranch          *string `json:"baseBranch,omitempty"`
	AgentProfileID      *string `json:"agentProfileId,omitempty"`
	ExecutorProfileID   *string `json:"executorProfileId,omitempty"`
	Prompt              *string `json:"prompt,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
	PollIntervalSeconds *int    `json:"pollIntervalSeconds,omitempty"`
	CleanupPolicy       *string `json:"cleanupPolicy,omitempty"`
	MaxInflightTasks    *int    `json:"maxInflightTasks,omitempty"`
}

type CreatePullRequestWatchRequest struct {
	WorkspaceID         string `json:"workspaceId"`
	WorkflowID          string `json:"workflowId"`
	WorkflowStepID      string `json:"workflowStepId"`
	ProjectID           string `json:"projectId"`
	AzureRepositoryID   string `json:"azureRepositoryId"`
	Status              string `json:"status"`
	CreatorID           string `json:"creatorId"`
	ReviewerID          string `json:"reviewerId"`
	RepositoryID        string `json:"repositoryId"`
	BaseBranch          string `json:"baseBranch"`
	AgentProfileID      string `json:"agentProfileId"`
	ExecutorProfileID   string `json:"executorProfileId"`
	Prompt              string `json:"prompt"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	CleanupPolicy       string `json:"cleanupPolicy"`
	MaxInflightTasks    *int   `json:"maxInflightTasks,omitempty"`
}

type UpdatePullRequestWatchRequest struct {
	WorkflowID          *string `json:"workflowId,omitempty"`
	WorkflowStepID      *string `json:"workflowStepId,omitempty"`
	ProjectID           *string `json:"projectId,omitempty"`
	AzureRepositoryID   *string `json:"azureRepositoryId,omitempty"`
	Status              *string `json:"status,omitempty"`
	CreatorID           *string `json:"creatorId,omitempty"`
	ReviewerID          *string `json:"reviewerId,omitempty"`
	RepositoryID        *string `json:"repositoryId,omitempty"`
	BaseBranch          *string `json:"baseBranch,omitempty"`
	AgentProfileID      *string `json:"agentProfileId,omitempty"`
	ExecutorProfileID   *string `json:"executorProfileId,omitempty"`
	Prompt              *string `json:"prompt,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
	PollIntervalSeconds *int    `json:"pollIntervalSeconds,omitempty"`
	CleanupPolicy       *string `json:"cleanupPolicy,omitempty"`
	MaxInflightTasks    *int    `json:"maxInflightTasks,omitempty"`
}

type WorkItemWatchEvent struct {
	WatchID           string
	WatchGeneration   int64
	WorkspaceID       string
	WorkflowID        string
	WorkflowStepID    string
	RepositoryID      string
	BaseBranch        string
	AgentProfileID    string
	ExecutorProfileID string
	Prompt            string
	CleanupPolicy     string
	MaxInflightTasks  *int
	ProjectID         string
	// ReservationClaimed indicates that the bounded provider check already
	// claimed the current-generation reservation before publishing this event.
	// The orchestrator source still claims reservations for manually published
	// events, but must not claim this one a second time.
	ReservationClaimed bool
	WorkItem           WorkItem
}

type PullRequestWatchEvent struct {
	WatchID           string
	WatchGeneration   int64
	WorkspaceID       string
	WorkflowID        string
	WorkflowStepID    string
	RepositoryID      string
	BaseBranch        string
	AgentProfileID    string
	ExecutorProfileID string
	Prompt            string
	CleanupPolicy     string
	MaxInflightTasks  *int
	ProjectID         string
	AzureRepositoryID string
	// ReservationClaimed has the same meaning as the work-item event field.
	ReservationClaimed bool
	PullRequest        PullRequest
}

// WorkItemWatchTask records a generation-scoped reservation made for an
// Azure Boards work item. A non-empty TaskID means the shared watcher
// dispatcher successfully created the Kandev task for this reservation.
type WorkItemWatchTask struct {
	ID          string    `db:"id"`
	WatchID     string    `db:"watch_id"`
	ProjectID   string    `db:"project_id"`
	WorkItemID  int       `db:"work_item_id"`
	WorkItemURL string    `db:"work_item_url"`
	TaskID      string    `db:"task_id"`
	Generation  int64     `db:"generation"`
	CreatedAt   time.Time `db:"created_at"`
}

// PullRequestWatchTask is the PR equivalent of WorkItemWatchTask.
type PullRequestWatchTask struct {
	ID                string    `db:"id"`
	WatchID           string    `db:"watch_id"`
	ProjectID         string    `db:"project_id"`
	AzureRepositoryID string    `db:"azure_repository_id"`
	PullRequestID     int       `db:"pull_request_id"`
	PullRequestURL    string    `db:"pull_request_url"`
	TaskID            string    `db:"task_id"`
	Generation        int64     `db:"generation"`
	CreatedAt         time.Time `db:"created_at"`
}
