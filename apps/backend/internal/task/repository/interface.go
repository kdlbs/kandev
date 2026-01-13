package repository

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Repository defines the interface for task storage operations
type Repository interface {
	// Workspace operations
	CreateWorkspace(ctx context.Context, workspace *models.Workspace) error
	GetWorkspace(ctx context.Context, id string) (*models.Workspace, error)
	UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error
	DeleteWorkspace(ctx context.Context, id string) error
	ListWorkspaces(ctx context.Context) ([]*models.Workspace, error)

	// Task operations
	CreateTask(ctx context.Context, task *models.Task) error
	GetTask(ctx context.Context, id string) (*models.Task, error)
	UpdateTask(ctx context.Context, task *models.Task) error
	DeleteTask(ctx context.Context, id string) error
	ListTasks(ctx context.Context, boardID string) ([]*models.Task, error)
	ListTasksByColumn(ctx context.Context, columnID string) ([]*models.Task, error)
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error
	AddTaskToBoard(ctx context.Context, taskID, boardID, columnID string, position int) error
	RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error

	// Board operations
	CreateBoard(ctx context.Context, board *models.Board) error
	GetBoard(ctx context.Context, id string) (*models.Board, error)
	UpdateBoard(ctx context.Context, board *models.Board) error
	DeleteBoard(ctx context.Context, id string) error
	ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error)

	// Column operations
	CreateColumn(ctx context.Context, column *models.Column) error
	GetColumn(ctx context.Context, id string) (*models.Column, error)
	GetColumnByState(ctx context.Context, boardID string, state v1.TaskState) (*models.Column, error)
	UpdateColumn(ctx context.Context, column *models.Column) error
	DeleteColumn(ctx context.Context, id string) error
	ListColumns(ctx context.Context, boardID string) ([]*models.Column, error)

	// Comment operations
	CreateComment(ctx context.Context, comment *models.Comment) error
	GetComment(ctx context.Context, id string) (*models.Comment, error)
	ListComments(ctx context.Context, taskID string) ([]*models.Comment, error)
	DeleteComment(ctx context.Context, id string) error

	// Agent Session operations
	CreateAgentSession(ctx context.Context, session *models.AgentSession) error
	GetAgentSession(ctx context.Context, id string) (*models.AgentSession, error)
	GetAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error)
	GetActiveAgentSessionByTaskID(ctx context.Context, taskID string) (*models.AgentSession, error)
	UpdateAgentSession(ctx context.Context, session *models.AgentSession) error
	UpdateAgentSessionStatus(ctx context.Context, id string, status models.AgentSessionStatus, errorMessage string) error
	ListAgentSessions(ctx context.Context, taskID string) ([]*models.AgentSession, error)
	ListActiveAgentSessions(ctx context.Context) ([]*models.AgentSession, error)
	DeleteAgentSession(ctx context.Context, id string) error

	// Repository operations
	CreateRepository(ctx context.Context, repository *models.Repository) error
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	UpdateRepository(ctx context.Context, repository *models.Repository) error
	DeleteRepository(ctx context.Context, id string) error
	ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error)

	// Repository script operations
	CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error
	GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error)
	UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error
	DeleteRepositoryScript(ctx context.Context, id string) error
	ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error)

	// Executor operations
	CreateExecutor(ctx context.Context, executor *models.Executor) error
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
	UpdateExecutor(ctx context.Context, executor *models.Executor) error
	DeleteExecutor(ctx context.Context, id string) error
	ListExecutors(ctx context.Context) ([]*models.Executor, error)

	// Environment operations
	CreateEnvironment(ctx context.Context, environment *models.Environment) error
	GetEnvironment(ctx context.Context, id string) (*models.Environment, error)
	UpdateEnvironment(ctx context.Context, environment *models.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	ListEnvironments(ctx context.Context) ([]*models.Environment, error)

	// Close closes the repository (for database connections)
	Close() error
}
