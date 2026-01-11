package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type BoardDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ColumnDTO struct {
	ID        string       `json:"id"`
	BoardID   string       `json:"board_id"`
	Name      string       `json:"name"`
	Position  int          `json:"position"`
	State     v1.TaskState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type TaskDTO struct {
	ID              string                 `json:"id"`
	BoardID         string                 `json:"board_id"`
	ColumnID        string                 `json:"column_id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	State           v1.TaskState           `json:"state"`
	Priority        int                    `json:"priority"`
	AgentType       *string                `json:"agent_type,omitempty"`
	RepositoryURL   *string                `json:"repository_url,omitempty"`
	Branch          *string                `json:"branch,omitempty"`
	AssignedAgentID *string                `json:"assigned_agent_id,omitempty"`
	Position        int                    `json:"position"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type BoardSnapshotDTO struct {
	Board   BoardDTO    `json:"board"`
	Columns []ColumnDTO `json:"columns"`
	Tasks   []TaskDTO   `json:"tasks"`
}

type ListBoardsResponse struct {
	Boards []BoardDTO `json:"boards"`
	Total  int        `json:"total"`
}

type ListColumnsResponse struct {
	Columns []ColumnDTO `json:"columns"`
	Total   int         `json:"total"`
}

type ListTasksResponse struct {
	Tasks []TaskDTO `json:"tasks"`
	Total int       `json:"total"`
}

type SuccessResponse struct {
	Success bool `json:"success"`
}

func FromBoard(board *models.Board) BoardDTO {
	var description *string
	if board.Description != "" {
		description = &board.Description
	}

	return BoardDTO{
		ID:          board.ID,
		Name:        board.Name,
		Description: description,
		OwnerID:     board.OwnerID,
		CreatedAt:   board.CreatedAt,
		UpdatedAt:   board.UpdatedAt,
	}
}

func FromColumn(column *models.Column) ColumnDTO {
	return ColumnDTO{
		ID:        column.ID,
		BoardID:   column.BoardID,
		Name:      column.Name,
		Position:  column.Position,
		State:     column.State,
		CreatedAt: column.CreatedAt,
		UpdatedAt: column.UpdatedAt,
	}
}

func FromTask(task *models.Task) TaskDTO {
	var agentType *string
	if task.AgentType != "" {
		agentType = &task.AgentType
	}
	var repositoryURL *string
	if task.RepositoryURL != "" {
		repositoryURL = &task.RepositoryURL
	}
	var branch *string
	if task.Branch != "" {
		branch = &task.Branch
	}
	var assignedAgentID *string
	if task.AssignedTo != "" {
		assignedAgentID = &task.AssignedTo
	}

	return TaskDTO{
		ID:              task.ID,
		BoardID:         task.BoardID,
		ColumnID:        task.ColumnID,
		Title:           task.Title,
		Description:     task.Description,
		State:           task.State,
		Priority:        task.Priority,
		AgentType:       agentType,
		RepositoryURL:   repositoryURL,
		Branch:          branch,
		AssignedAgentID: assignedAgentID,
		Position:        task.Position,
		CreatedAt:       task.CreatedAt,
		UpdatedAt:       task.UpdatedAt,
		Metadata:        task.Metadata,
	}
}
