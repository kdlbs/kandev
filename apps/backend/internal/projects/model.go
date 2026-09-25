package projects

import (
	"time"
)

const MaxWorkerChangeRequests = 10

// Project is the durable workspace-level Agent Project configuration.
type Project struct {
	ID                   string     `json:"id"`
	WorkspaceID          string     `json:"workspace_id"`
	Name                 string     `json:"name"`
	RepositoryIDs        []string   `json:"repository_ids"`
	PrimaryRepositoryID  string     `json:"primary_repository_id"`
	CoordinatorProfileID string     `json:"coordinator_profile_id"`
	EconomyProfileID     string     `json:"economy_profile_id"`
	FrontierProfileID    string     `json:"frontier_profile_id"`
	ExecutorProfileID    string     `json:"executor_profile_id"`
	MainTaskID           string     `json:"main_task_id"`
	Revision             int64      `json:"revision"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// TaskSummary is the bounded task projection used in project lists.
type TaskSummary struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	State      string     `json:"state"`
	ParentID   string     `json:"parent_id,omitempty"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ProjectView combines the project record with its coordinator and workers.
type ProjectView struct {
	Project
	Tasks []TaskSummary `json:"tasks"`
}

// WorkerRepository summarizes the repository branch reserved for one worker.
type WorkerRepository struct {
	RepositoryID   string `json:"repository_id"`
	Name           string `json:"name"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch,omitempty"`
}

// WorkerChangeRequest is the provider-neutral, bounded change request summary
// available to a project coordinator during worker inspection.
type WorkerChangeRequest struct {
	Provider string `json:"provider"`
	Number   int    `json:"number"`
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	State    string `json:"state"`
	Draft    bool   `json:"draft"`
}

// WorkerView is the coordinator-safe projection of one direct worker task.
type WorkerView struct {
	TaskSummary
	Tier           string                `json:"tier"`
	ProfileID      string                `json:"profile_id"`
	SessionState   string                `json:"session_state,omitempty"`
	SessionError   string                `json:"session_error,omitempty"`
	LatestResult   string                `json:"latest_result,omitempty"`
	Repositories   []WorkerRepository    `json:"repositories,omitempty"`
	ChangeRequests []WorkerChangeRequest `json:"change_requests,omitempty"`
}

// WorkerRepositoryInput selects one project repository and an optional base
// branch. Empty input selects every repository currently configured on the
// project.
type WorkerRepositoryInput struct {
	RepositoryID string `json:"repository_id"`
	BaseBranch   string `json:"base_branch,omitempty"`
}

// CreateWorkerRequest contains only coordinator-controlled worker fields.
// Profile, project, parent, workspace, and executor are resolved server-side.
type CreateWorkerRequest struct {
	Tier         string                  `json:"tier"`
	Title        string                  `json:"title"`
	Prompt       string                  `json:"prompt"`
	Repositories []WorkerRepositoryInput `json:"repositories,omitempty"`
}

// CreateRequest creates one project and its workflow-free coordinator.
type CreateRequest struct {
	WorkspaceID          string   `json:"workspace_id"`
	Name                 string   `json:"name"`
	RepositoryIDs        []string `json:"repository_ids"`
	PrimaryRepositoryID  string   `json:"primary_repository_id"`
	CoordinatorProfileID string   `json:"coordinator_profile_id"`
	EconomyProfileID     string   `json:"economy_profile_id"`
	FrontierProfileID    string   `json:"frontier_profile_id"`
	RequestKey           string   `json:"request_key"`
}

// UpdateRequest applies a revision-checked edit. Executor changes require an
// explicit request to apply the current workspace default.
type UpdateRequest struct {
	Revision                      int64    `json:"revision"`
	ApplyWorkspaceDefaultExecutor bool     `json:"apply_workspace_default_executor,omitempty"`
	Name                          *string  `json:"name,omitempty"`
	RepositoryIDs                 []string `json:"repository_ids,omitempty"`
	PrimaryRepositoryID           *string  `json:"primary_repository_id,omitempty"`
	CoordinatorProfileID          *string  `json:"coordinator_profile_id,omitempty"`
	EconomyProfileID              *string  `json:"economy_profile_id,omitempty"`
	FrontierProfileID             *string  `json:"frontier_profile_id,omitempty"`
}
