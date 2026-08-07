package azuredevops

import "context"

const (
	assignCurrentUserAction = "assign_current_user"
	unassignAction          = "unassign"
)

// Team is an Azure DevOps team that owns one or more boards.
type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName,omitempty"`
}

// BoardReference identifies a board available to a team.
type BoardReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type BoardColumn struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	ColumnType    string            `json:"columnType,omitempty"`
	Description   string            `json:"description,omitempty"`
	IsSplit       bool              `json:"isSplit,omitempty"`
	ItemLimit     int               `json:"itemLimit,omitempty"`
	StateMappings map[string]string `json:"stateMappings,omitempty"`
}

type FieldReference struct {
	ReferenceName string `json:"referenceName"`
}

type BoardFields struct {
	ColumnField FieldReference `json:"columnField"`
	DoneField   FieldReference `json:"doneField"`
	RowField    FieldReference `json:"rowField"`
}

type BoardRow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

type Board struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Columns []BoardColumn `json:"columns"`
	Fields  BoardFields   `json:"fields"`
	Rows    []BoardRow    `json:"rows,omitempty"`
}

type BoardWorkItem struct {
	WorkItem
	ColumnID   string `json:"columnId"`
	ColumnDone bool   `json:"columnDone"`
}

type BoardSnapshot struct {
	Board Board           `json:"board"`
	Items []BoardWorkItem `json:"items"`
}

// BoardReader is the optional Azure Boards read surface. It is kept separate
// from Client so existing integrations and test doubles remain source
// compatible while board support rolls out.
type BoardReader interface {
	ListTeams(ctx context.Context, projectID string) ([]Team, error)
	ListBoards(ctx context.Context, projectID, teamID string) ([]BoardReference, error)
	GetBoardSnapshot(ctx context.Context, projectID, teamID, boardID string) (*BoardSnapshot, error)
}

type BoardWorkItemUpdateRequest struct {
	Revision       int     `json:"revision"`
	AssigneeAction *string `json:"assigneeAction,omitempty"`
	ColumnID       *string `json:"columnId,omitempty"`
	ColumnDone     *bool   `json:"columnDone,omitempty"`

	resolvedAssignee    string
	hasResolvedAssignee bool
}

type BoardWriter interface {
	UpdateBoardWorkItem(ctx context.Context, projectID, teamID, boardID string, id int, request BoardWorkItemUpdateRequest) (*BoardWorkItem, error)
}
