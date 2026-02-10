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
	ListTasksByWorkspace(ctx context.Context, workspaceID string, query string, page, pageSize int) ([]*models.Task, int, error)
	ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error)
	UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error
	AddTaskToBoard(ctx context.Context, taskID, boardID, workflowStepID string, position int) error
	RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error

	// TaskRepository operations
	CreateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error
	GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error)
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	UpdateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error
	DeleteTaskRepository(ctx context.Context, id string) error
	DeleteTaskRepositoriesByTask(ctx context.Context, taskID string) error
	GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error)

	// Board operations
	CreateBoard(ctx context.Context, board *models.Board) error
	GetBoard(ctx context.Context, id string) (*models.Board, error)
	UpdateBoard(ctx context.Context, board *models.Board) error
	DeleteBoard(ctx context.Context, id string) error
	ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error)

	// Message operations
	CreateMessage(ctx context.Context, message *models.Message) error
	GetMessage(ctx context.Context, id string) (*models.Message, error)
	GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error)
	GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error)
	UpdateMessage(ctx context.Context, message *models.Message) error
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	ListMessagesPaginated(ctx context.Context, sessionID string, opts models.ListMessagesOptions) ([]*models.Message, bool, error)
	DeleteMessage(ctx context.Context, id string) error

	// Turn operations
	CreateTurn(ctx context.Context, turn *models.Turn) error
	GetTurn(ctx context.Context, id string) (*models.Turn, error)
	GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error)
	UpdateTurn(ctx context.Context, turn *models.Turn) error
	CompleteTurn(ctx context.Context, id string) error
	CompleteRunningToolCallsForTurn(ctx context.Context, turnID string) (int64, error)
	ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error)

	// Task Session operations
	CreateTaskSession(ctx context.Context, session *models.TaskSession) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error
	ClearSessionExecutionID(ctx context.Context, id string) error
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error)
	ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
	HasActiveTaskSessionsByExecutor(ctx context.Context, executorID string) (bool, error)
	HasActiveTaskSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error)
	HasActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (bool, error)
	DeleteTaskSession(ctx context.Context, id string) error

	// Workflow-related session operations
	GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetPrimarySessionIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error)
	GetSessionCountsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error)
	GetPrimarySessionInfoByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error)
	SetSessionPrimary(ctx context.Context, sessionID string) error
	UpdateSessionWorkflowStep(ctx context.Context, sessionID string, stepID string) error
	UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error

	// Task Session Worktree operations
	CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error
	ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error)
	DeleteTaskSessionWorktree(ctx context.Context, id string) error
	DeleteTaskSessionWorktreesBySession(ctx context.Context, sessionID string) error

	// Git Snapshot operations
	CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error
	GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)
	GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error)
	GetGitSnapshotsBySession(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error)

	// Session Commit operations
	CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) error
	GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error)
	GetLatestSessionCommit(ctx context.Context, sessionID string) (*models.SessionCommit, error)
	DeleteSessionCommit(ctx context.Context, id string) error

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

	// Executor running operations
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
	DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error

	// Environment operations
	CreateEnvironment(ctx context.Context, environment *models.Environment) error
	GetEnvironment(ctx context.Context, id string) (*models.Environment, error)
	UpdateEnvironment(ctx context.Context, environment *models.Environment) error
	DeleteEnvironment(ctx context.Context, id string) error
	ListEnvironments(ctx context.Context) ([]*models.Environment, error)

	// Session File Review operations
	UpsertSessionFileReview(ctx context.Context, review *models.SessionFileReview) error
	GetSessionFileReviews(ctx context.Context, sessionID string) ([]*models.SessionFileReview, error)
	DeleteSessionFileReviews(ctx context.Context, sessionID string) error

	// Task Plan operations
	CreateTaskPlan(ctx context.Context, plan *models.TaskPlan) error
	GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error)
	UpdateTaskPlan(ctx context.Context, plan *models.TaskPlan) error
	DeleteTaskPlan(ctx context.Context, taskID string) error

	// Close closes the repository (for database connections)
	Close() error
}
