package dto

import (
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type ListBoardsRequest struct {
	WorkspaceID string
}

type ListWorkspacesRequest struct{}

type GetWorkspaceRequest struct {
	ID string
}

type CreateWorkspaceRequest struct {
	Name                  string
	Description           string
	OwnerID               string
	DefaultExecutorID     *string
	DefaultEnvironmentID  *string
	DefaultAgentProfileID *string
}

type UpdateWorkspaceRequest struct {
	ID                    string
	Name                  *string
	Description           *string
	DefaultExecutorID     *string
	DefaultEnvironmentID  *string
	DefaultAgentProfileID *string
}

type DeleteWorkspaceRequest struct {
	ID string
}

type GetBoardRequest struct {
	ID string
}

type CreateBoardRequest struct {
	WorkspaceID string
	Name        string
	Description string
}

type UpdateBoardRequest struct {
	ID          string
	Name        *string
	Description *string
}

type DeleteBoardRequest struct {
	ID string
}

type ListColumnsRequest struct {
	BoardID string
}

type GetColumnRequest struct {
	ID string
}

type CreateColumnRequest struct {
	BoardID  string
	Name     string
	Position int
	State    v1.TaskState
	Color    string
}

type UpdateColumnRequest struct {
	ID       string
	Name     *string
	Position *int
	State    *v1.TaskState
	Color    *string
}

type ListRepositoriesRequest struct {
	WorkspaceID string
}

type GetRepositoryRequest struct {
	ID string
}

type CreateRepositoryRequest struct {
	WorkspaceID    string
	Name           string
	SourceType     string
	LocalPath      string
	Provider       string
	ProviderRepoID string
	ProviderOwner  string
	ProviderName   string
	DefaultBranch  string
	SetupScript    string
	CleanupScript  string
}

type UpdateRepositoryRequest struct {
	ID             string
	Name           *string
	SourceType     *string
	LocalPath      *string
	Provider       *string
	ProviderRepoID *string
	ProviderOwner  *string
	ProviderName   *string
	DefaultBranch  *string
	SetupScript    *string
	CleanupScript  *string
}

type DeleteRepositoryRequest struct {
	ID string
}

type ListRepositoryScriptsRequest struct {
	RepositoryID string
}

type GetRepositoryScriptRequest struct {
	ID string
}

type ListExecutorsRequest struct{}

type GetExecutorRequest struct {
	ID string
}

type CreateExecutorRequest struct {
	Name     string
	Type     models.ExecutorType
	Status   models.ExecutorStatus
	IsSystem bool
	Config   map[string]string
}

type UpdateExecutorRequest struct {
	ID     string
	Name   *string
	Type   *models.ExecutorType
	Status *models.ExecutorStatus
	Config map[string]string
}

type DeleteExecutorRequest struct {
	ID string
}

type ListEnvironmentsRequest struct{}

type GetEnvironmentRequest struct {
	ID string
}

type CreateEnvironmentRequest struct {
	Name         string
	Kind         models.EnvironmentKind
	WorktreeRoot string
	ImageTag     string
	Dockerfile   string
	BuildConfig  map[string]string
}

type UpdateEnvironmentRequest struct {
	ID           string
	Name         *string
	Kind         *models.EnvironmentKind
	WorktreeRoot *string
	ImageTag     *string
	Dockerfile   *string
	BuildConfig  map[string]string
}

type DeleteEnvironmentRequest struct {
	ID string
}

type CreateRepositoryScriptRequest struct {
	RepositoryID string
	Name         string
	Command      string
	Position     int
}

type UpdateRepositoryScriptRequest struct {
	ID       string
	Name     *string
	Command  *string
	Position *int
}

type DeleteRepositoryScriptRequest struct {
	ID string
}

type ListRepositoryBranchesRequest struct {
	ID string
}

type ListLocalRepositoryBranchesRequest struct {
	WorkspaceID string
	Path        string
}

type DiscoverRepositoriesRequest struct {
	WorkspaceID string
	Root        string
}

type ValidateRepositoryPathRequest struct {
	WorkspaceID string
	Path        string
}

type GetBoardSnapshotRequest struct {
	BoardID string
}

type GetWorkspaceBoardSnapshotRequest struct {
	WorkspaceID string
	BoardID     string
}

type ListTasksRequest struct {
	BoardID string
}

type ListTaskSessionsRequest struct {
	TaskID string
}

type GetTaskSessionRequest struct {
	TaskSessionID string
}

type GetTaskRequest struct {
	ID string
}

type TaskRepositoryInput struct {
	RepositoryID  string
	BaseBranch    string
	LocalPath     string
	Name          string
	DefaultBranch string
}

type CreateTaskRequest struct {
	WorkspaceID  string
	BoardID      string
	ColumnID     string
	Title        string
	Description  string
	Priority     int
	State        *v1.TaskState
	Repositories []TaskRepositoryInput
	Position     int
	Metadata     map[string]interface{}
}

type UpdateTaskRequest struct {
	ID           string
	Title        *string
	Description  *string
	Priority     *int
	State        *v1.TaskState
	Repositories []TaskRepositoryInput
	Position     *int
	Metadata     map[string]interface{}
}

type DeleteTaskRequest struct {
	ID string
}

type ListMessagesRequest struct {
	TaskSessionID string
	Limit         int
	Before        string
	After         string
	Sort          string
}

type CreateMessageRequest struct {
	TaskSessionID string
	Content       string
	AuthorType    string
	AuthorID      string
	Type          string
	RequestsInput bool
	Metadata      map[string]interface{}
	TaskID        string
}

type MoveTaskRequest struct {
	ID       string
	BoardID  string
	ColumnID string
	Position int
}

type UpdateTaskStateRequest struct {
	ID    string
	State v1.TaskState
}
