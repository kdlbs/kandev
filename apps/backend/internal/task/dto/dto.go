package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type BoardDTO struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkspaceDTO struct {
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
	Color     string       `json:"color"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type RepositoryDTO struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	Name           string    `json:"name"`
	SourceType     string    `json:"source_type"`
	LocalPath      string    `json:"local_path,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	ProviderRepoID string    `json:"provider_repo_id,omitempty"`
	ProviderOwner  string    `json:"provider_owner,omitempty"`
	ProviderName   string    `json:"provider_name,omitempty"`
	DefaultBranch  string    `json:"default_branch,omitempty"`
	SetupScript    string    `json:"setup_script,omitempty"`
	CleanupScript  string    `json:"cleanup_script,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type RepositoryScriptDTO struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Command      string    `json:"command"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TaskDTO struct {
	ID              string                 `json:"id"`
	WorkspaceID     string                 `json:"workspace_id"`
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
	// Worktree information (populated if worktree exists for this task)
	WorktreePath   *string `json:"worktree_path,omitempty"`
	WorktreeBranch *string `json:"worktree_branch,omitempty"`
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

type ListWorkspacesResponse struct {
	Workspaces []WorkspaceDTO `json:"workspaces"`
	Total      int            `json:"total"`
}

type ListColumnsResponse struct {
	Columns []ColumnDTO `json:"columns"`
	Total   int         `json:"total"`
}

type ListRepositoriesResponse struct {
	Repositories []RepositoryDTO `json:"repositories"`
	Total        int             `json:"total"`
}

type ListRepositoryScriptsResponse struct {
	Scripts []RepositoryScriptDTO `json:"scripts"`
	Total   int                   `json:"total"`
}

type RepositoryBranchesResponse struct {
	Branches []string `json:"branches"`
	Total    int      `json:"total"`
}

type LocalRepositoryDTO struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type RepositoryDiscoveryResponse struct {
	Roots        []string             `json:"roots"`
	Repositories []LocalRepositoryDTO `json:"repositories"`
	Total        int                  `json:"total"`
}

type RepositoryPathValidationResponse struct {
	Path          string `json:"path"`
	Exists        bool   `json:"exists"`
	IsGitRepo     bool   `json:"is_git"`
	Allowed       bool   `json:"allowed"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Message       string `json:"message,omitempty"`
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
		WorkspaceID: board.WorkspaceID,
		Name:        board.Name,
		Description: description,
		CreatedAt:   board.CreatedAt,
		UpdatedAt:   board.UpdatedAt,
	}
}

func FromWorkspace(workspace *models.Workspace) WorkspaceDTO {
	var description *string
	if workspace.Description != "" {
		description = &workspace.Description
	}

	return WorkspaceDTO{
		ID:          workspace.ID,
		Name:        workspace.Name,
		Description: description,
		OwnerID:     workspace.OwnerID,
		CreatedAt:   workspace.CreatedAt,
		UpdatedAt:   workspace.UpdatedAt,
	}
}

func FromColumn(column *models.Column) ColumnDTO {
	return ColumnDTO{
		ID:        column.ID,
		BoardID:   column.BoardID,
		Name:      column.Name,
		Position:  column.Position,
		State:     column.State,
		Color:     column.Color,
		CreatedAt: column.CreatedAt,
		UpdatedAt: column.UpdatedAt,
	}
}

func FromRepository(repository *models.Repository) RepositoryDTO {
	return RepositoryDTO{
		ID:             repository.ID,
		WorkspaceID:    repository.WorkspaceID,
		Name:           repository.Name,
		SourceType:     repository.SourceType,
		LocalPath:      repository.LocalPath,
		Provider:       repository.Provider,
		ProviderRepoID: repository.ProviderRepoID,
		ProviderOwner:  repository.ProviderOwner,
		ProviderName:   repository.ProviderName,
		DefaultBranch:  repository.DefaultBranch,
		SetupScript:    repository.SetupScript,
		CleanupScript:  repository.CleanupScript,
		CreatedAt:      repository.CreatedAt,
		UpdatedAt:      repository.UpdatedAt,
	}
}

func FromRepositoryScript(script *models.RepositoryScript) RepositoryScriptDTO {
	return RepositoryScriptDTO{
		ID:           script.ID,
		RepositoryID: script.RepositoryID,
		Name:         script.Name,
		Command:      script.Command,
		Position:     script.Position,
		CreatedAt:    script.CreatedAt,
		UpdatedAt:    script.UpdatedAt,
	}
}

func FromLocalRepository(repo service.LocalRepository) LocalRepositoryDTO {
	return LocalRepositoryDTO{
		Path:          repo.Path,
		Name:          repo.Name,
		DefaultBranch: repo.DefaultBranch,
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
		WorkspaceID:     task.WorkspaceID,
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
