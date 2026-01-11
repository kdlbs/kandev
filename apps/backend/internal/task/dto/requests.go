package dto

import v1 "github.com/kandev/kandev/pkg/api/v1"

type ListBoardsRequest struct{}

type GetBoardRequest struct {
	ID string
}

type CreateBoardRequest struct {
	Name        string
	Description string
	OwnerID     string
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
}

type GetBoardSnapshotRequest struct {
	BoardID string
}

type ListTasksRequest struct {
	BoardID string
}

type GetTaskRequest struct {
	ID string
}

type CreateTaskRequest struct {
	BoardID       string
	ColumnID      string
	Title         string
	Description   string
	Priority      int
	AgentType     string
	RepositoryURL string
	Branch        string
	AssignedTo    string
	Position      int
	Metadata      map[string]interface{}
}

type UpdateTaskRequest struct {
	ID          string
	Title       *string
	Description *string
	Priority    *int
	AgentType   *string
	AssignedTo  *string
	Position    *int
	Metadata    map[string]interface{}
}

type DeleteTaskRequest struct {
	ID string
}

type MoveTaskRequest struct {
	ID       string
	ColumnID string
	Position int
}

type UpdateTaskStateRequest struct {
	ID    string
	State v1.TaskState
}
