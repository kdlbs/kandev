package executor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockAgentManager implements AgentManagerClient for testing
type mockAgentManager struct {
	launchAgentFunc             func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error)
	startAgentProcessFunc       func(ctx context.Context, agentExecutionID string) error
	resolveAgentProfileFunc     func(ctx context.Context, profileID string) (*AgentProfileInfo, error)
	setExecutionDescriptionFunc func(ctx context.Context, agentExecutionID string, description string) error
}

func (m *mockAgentManager) LaunchAgent(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
	if m.launchAgentFunc != nil {
		return m.launchAgentFunc(ctx, req)
	}
	return &LaunchAgentResponse{
		AgentExecutionID: "exec-123",
		ContainerID:      "container-123",
		Status:           v1.AgentStatusStarting,
	}, nil
}

func (m *mockAgentManager) SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error {
	if m.setExecutionDescriptionFunc != nil {
		return m.setExecutionDescriptionFunc(ctx, agentExecutionID, description)
	}
	return nil
}

func (m *mockAgentManager) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	if m.startAgentProcessFunc != nil {
		return m.startAgentProcessFunc(ctx, agentExecutionID)
	}
	return nil
}

func (m *mockAgentManager) StopAgent(ctx context.Context, agentExecutionID string, force bool) error {
	return nil
}

func (m *mockAgentManager) PromptAgent(ctx context.Context, agentExecutionID string, prompt string, _ []v1.MessageAttachment) (*PromptResult, error) {
	return nil, nil
}

func (m *mockAgentManager) CancelAgent(ctx context.Context, sessionID string) error {
	return nil
}

func (m *mockAgentManager) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	return nil
}

func (m *mockAgentManager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	return false
}

func (m *mockAgentManager) IsPassthroughSession(ctx context.Context, sessionID string) bool {
	return false
}

func (m *mockAgentManager) ResolveAgentProfile(ctx context.Context, profileID string) (*AgentProfileInfo, error) {
	if m.resolveAgentProfileFunc != nil {
		return m.resolveAgentProfileFunc(ctx, profileID)
	}
	return &AgentProfileInfo{
		ProfileID:   profileID,
		ProfileName: "Test Profile",
		AgentID:     "agent-123",
		AgentName:   "Test Agent",
		Model:       "claude-3-opus",
	}, nil
}

// mockRepository implements executorStore for testing
type mockRepository struct {
	sessions         map[string]*models.TaskSession
	taskRepositories map[string]*models.TaskRepository
	repositories     map[string]*models.Repository
	executors        map[string]*models.Executor

	// Track calls for verification
	createTaskSessionCalls []*models.TaskSession
	updateTaskSessionCalls []*models.TaskSession
	setSessionPrimaryCalls []string
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		sessions:         make(map[string]*models.TaskSession),
		taskRepositories: make(map[string]*models.TaskRepository),
		repositories:     make(map[string]*models.Repository),
		executors:        make(map[string]*models.Executor),
	}
}

// Implement required repository methods

func (m *mockRepository) GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error) {
	// Return first matching repository for the task (matches sqlite implementation)
	for _, tr := range m.taskRepositories {
		if tr.TaskID == taskID {
			return tr, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	if repo, ok := m.repositories[id]; ok {
		return repo, nil
	}
	return nil, nil
}

func (m *mockRepository) CreateTaskSession(ctx context.Context, session *models.TaskSession) error {
	m.createTaskSessionCalls = append(m.createTaskSessionCalls, session)
	m.sessions[session.ID] = session
	return nil
}

func (m *mockRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if session, ok := m.sessions[id]; ok {
		return session, nil
	}
	return nil, nil
}

func (m *mockRepository) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
	m.updateTaskSessionCalls = append(m.updateTaskSessionCalls, session)
	m.sessions[session.ID] = session
	return nil
}

func (m *mockRepository) SetSessionPrimary(ctx context.Context, sessionID string) error {
	m.setSessionPrimaryCalls = append(m.setSessionPrimaryCalls, sessionID)
	return nil
}

func (m *mockRepository) UpdateTaskSessionState(ctx context.Context, sessionID string, state models.TaskSessionState, errorMessage string) error {
	if session, ok := m.sessions[sessionID]; ok {
		session.State = state
		session.ErrorMessage = errorMessage
	}
	return nil
}

func (m *mockRepository) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	if exec, ok := m.executors[id]; ok {
		return exec, nil
	}
	return nil, nil
}

func (m *mockRepository) UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error {
	return nil
}

func (m *mockRepository) CreateTaskSessionWorktree(ctx context.Context, worktree *models.TaskSessionWorktree) error {
	return nil
}

func (m *mockRepository) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	return nil
}
func (m *mockRepository) ArchiveTask(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListTasksForAutoArchive(ctx context.Context) ([]*models.Task, error) {
	return nil, nil
}

func (m *mockRepository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	return nil, nil
}

// Stub implementations for additional repository methods

// Workspace operations
func (m *mockRepository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	return nil
}
func (m *mockRepository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	return nil
}
func (m *mockRepository) DeleteWorkspace(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	return nil, nil
}

// Task operations
func (m *mockRepository) CreateTask(ctx context.Context, task *models.Task) error { return nil }
func (m *mockRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTask(ctx context.Context, task *models.Task) error { return nil }
func (m *mockRepository) DeleteTask(ctx context.Context, id string) error         { return nil }
func (m *mockRepository) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) ListTasksByWorkspace(ctx context.Context, workspaceID string, query string, page, pageSize int, includeArchived bool) ([]*models.Task, int, error) {
	return nil, 0, nil
}
func (m *mockRepository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) AddTaskToWorkflow(ctx context.Context, taskID, workflowID, workflowStepID string, position int) error {
	return nil
}
func (m *mockRepository) RemoveTaskFromWorkflow(ctx context.Context, taskID, workflowID string) error {
	return nil
}

// TaskRepository operations
func (m *mockRepository) CreateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	return nil
}
func (m *mockRepository) GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error) {
	return nil, nil
}
func (m *mockRepository) ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
	return nil, nil
}
func (m *mockRepository) ListTaskRepositoriesByTaskIDs(_ context.Context, _ []string) (map[string][]*models.TaskRepository, error) {
	return make(map[string][]*models.TaskRepository), nil
}
func (m *mockRepository) UpdateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	return nil
}
func (m *mockRepository) DeleteTaskRepository(ctx context.Context, id string) error { return nil }
func (m *mockRepository) DeleteTaskRepositoriesByTask(ctx context.Context, taskID string) error {
	return nil
}

// Workflow operations
func (m *mockRepository) CreateWorkflow(ctx context.Context, workflow *models.Workflow) error {
	return nil
}
func (m *mockRepository) GetWorkflow(ctx context.Context, id string) (*models.Workflow, error) {
	return nil, nil
}
func (m *mockRepository) UpdateWorkflow(ctx context.Context, workflow *models.Workflow) error {
	return nil
}
func (m *mockRepository) DeleteWorkflow(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListWorkflows(ctx context.Context, workspaceID string) ([]*models.Workflow, error) {
	return nil, nil
}

// Message operations
func (m *mockRepository) CreateMessage(ctx context.Context, message *models.Message) error {
	return nil
}
func (m *mockRepository) GetMessage(ctx context.Context, id string) (*models.Message, error) {
	return nil, nil
}
func (m *mockRepository) GetMessageByToolCallID(ctx context.Context, sessionID, toolCallID string) (*models.Message, error) {
	return nil, nil
}
func (m *mockRepository) GetMessageByPendingID(ctx context.Context, sessionID, pendingID string) (*models.Message, error) {
	return nil, nil
}
func (m *mockRepository) FindMessageByPendingID(ctx context.Context, pendingID string) (*models.Message, error) {
	return nil, nil
}
func (m *mockRepository) UpdateMessage(ctx context.Context, message *models.Message) error {
	return nil
}
func (m *mockRepository) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	return nil, nil
}
func (m *mockRepository) ListMessagesPaginated(ctx context.Context, sessionID string, opts models.ListMessagesOptions) ([]*models.Message, bool, error) {
	return nil, false, nil
}
func (m *mockRepository) DeleteMessage(ctx context.Context, id string) error { return nil }

// Turn operations
func (m *mockRepository) CreateTurn(ctx context.Context, turn *models.Turn) error { return nil }
func (m *mockRepository) GetTurn(ctx context.Context, id string) (*models.Turn, error) {
	return nil, nil
}
func (m *mockRepository) GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTurn(ctx context.Context, turn *models.Turn) error { return nil }
func (m *mockRepository) CompleteTurn(ctx context.Context, id string) error       { return nil }
func (m *mockRepository) CompleteRunningToolCallsForTurn(ctx context.Context, turnID string) (int64, error) {
	return 0, nil
}
func (m *mockRepository) ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error) {
	return nil, nil
}

// Task Session operations
func (m *mockRepository) GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) ClearSessionExecutionID(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) ListActiveTaskSessions(ctx context.Context) ([]*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) ListActiveTaskSessionsByTaskID(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error) {
	return false, nil
}
func (m *mockRepository) HasActiveTaskSessionsByExecutor(ctx context.Context, executorID string) (bool, error) {
	return false, nil
}
func (m *mockRepository) HasActiveTaskSessionsByEnvironment(ctx context.Context, environmentID string) (bool, error) {
	return false, nil
}
func (m *mockRepository) HasActiveTaskSessionsByRepository(ctx context.Context, repositoryID string) (bool, error) {
	return false, nil
}
func (m *mockRepository) DeleteTaskSession(ctx context.Context, id string) error { return nil }

// Workflow-related session operations
func (m *mockRepository) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) GetPrimarySessionIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error) {
	return nil, nil
}
func (m *mockRepository) GetSessionCountsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error) {
	return nil, nil
}
func (m *mockRepository) GetPrimarySessionInfoByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) UpdateSessionWorkflowStep(ctx context.Context, sessionID string, stepID string) error {
	return nil
}
func (m *mockRepository) UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error {
	return nil
}

// Task Session Worktree operations
func (m *mockRepository) ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error) {
	return nil, nil
}
func (m *mockRepository) ListWorktreesBySessionIDs(_ context.Context, _ []string) (map[string][]*models.TaskSessionWorktree, error) {
	return make(map[string][]*models.TaskSessionWorktree), nil
}
func (m *mockRepository) DeleteTaskSessionWorktree(ctx context.Context, id string) error { return nil }
func (m *mockRepository) DeleteTaskSessionWorktreesBySession(ctx context.Context, sessionID string) error {
	return nil
}

// Git Snapshot operations
func (m *mockRepository) CreateGitSnapshot(ctx context.Context, snapshot *models.GitSnapshot) error {
	return nil
}
func (m *mockRepository) GetLatestGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return nil, nil
}
func (m *mockRepository) GetFirstGitSnapshot(ctx context.Context, sessionID string) (*models.GitSnapshot, error) {
	return nil, nil
}
func (m *mockRepository) GetGitSnapshotsBySession(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error) {
	return nil, nil
}

// Session Commit operations
func (m *mockRepository) CreateSessionCommit(ctx context.Context, commit *models.SessionCommit) error {
	return nil
}
func (m *mockRepository) GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error) {
	return nil, nil
}
func (m *mockRepository) GetLatestSessionCommit(ctx context.Context, sessionID string) (*models.SessionCommit, error) {
	return nil, nil
}
func (m *mockRepository) DeleteSessionCommit(ctx context.Context, id string) error { return nil }

// Repository operations
func (m *mockRepository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	return nil
}
func (m *mockRepository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	return nil
}
func (m *mockRepository) DeleteRepository(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	return nil, nil
}

// Repository script operations
func (m *mockRepository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	return nil
}
func (m *mockRepository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	return nil, nil
}
func (m *mockRepository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	return nil
}
func (m *mockRepository) DeleteRepositoryScript(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	return nil, nil
}
func (m *mockRepository) ListScriptsByRepositoryIDs(_ context.Context, _ []string) (map[string][]*models.RepositoryScript, error) {
	return make(map[string][]*models.RepositoryScript), nil
}
func (m *mockRepository) GetRepositoryByProviderInfo(_ context.Context, _, _, _, _ string) (*models.Repository, error) {
	return nil, nil
}

// Executor operations
func (m *mockRepository) CreateExecutor(ctx context.Context, executor *models.Executor) error {
	return nil
}
func (m *mockRepository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	return nil
}
func (m *mockRepository) DeleteExecutor(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	return nil, nil
}

// Executor running operations
func (m *mockRepository) ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	return nil, nil
}
func (m *mockRepository) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	return nil, nil
}
func (m *mockRepository) DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error {
	return nil
}

// Environment operations
func (m *mockRepository) CreateEnvironment(ctx context.Context, environment *models.Environment) error {
	return nil
}
func (m *mockRepository) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	return nil, nil
}
func (m *mockRepository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
	return nil
}
func (m *mockRepository) DeleteEnvironment(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	return nil, nil
}

// Task Plan operations
func (m *mockRepository) CreateTaskPlan(ctx context.Context, plan *models.TaskPlan) error { return nil }
func (m *mockRepository) GetTaskPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTaskPlan(ctx context.Context, plan *models.TaskPlan) error { return nil }
func (m *mockRepository) DeleteTaskPlan(ctx context.Context, taskID string) error         { return nil }

// Session File Review operations
func (m *mockRepository) UpsertSessionFileReview(ctx context.Context, review *models.SessionFileReview) error {
	return nil
}
func (m *mockRepository) GetSessionFileReviews(ctx context.Context, sessionID string) ([]*models.SessionFileReview, error) {
	return nil, nil
}
func (m *mockRepository) DeleteSessionFileReviews(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockRepository) CountTasksByWorkflow(ctx context.Context, workflowID string) (int, error) {
	return 0, nil
}
func (m *mockRepository) CountTasksByWorkflowStep(ctx context.Context, stepID string) (int, error) {
	return 0, nil
}

// Executor profile operations
func (m *mockRepository) CreateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	return nil
}
func (m *mockRepository) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	return nil, nil
}
func (m *mockRepository) UpdateExecutorProfile(ctx context.Context, profile *models.ExecutorProfile) error {
	return nil
}
func (m *mockRepository) DeleteExecutorProfile(ctx context.Context, id string) error { return nil }
func (m *mockRepository) ListExecutorProfiles(ctx context.Context, executorID string) ([]*models.ExecutorProfile, error) {
	return nil, nil
}
func (m *mockRepository) ListAllExecutorProfiles(ctx context.Context) ([]*models.ExecutorProfile, error) {
	return nil, nil
}

// Close operation
func (m *mockRepository) Close() error { return nil }

// mockShellPrefs implements ShellPreferenceProvider
type mockShellPrefs struct{}

func (m *mockShellPrefs) PreferredShell(ctx context.Context) (string, error) {
	return "/bin/bash", nil
}

// Helper to create a test executor
func newTestExecutor(t *testing.T, agentManager AgentManagerClient, repo *mockRepository) *Executor {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return NewExecutor(agentManager, repo, log, ExecutorConfig{
		ShellPrefs: &mockShellPrefs{},
	})
}

// Tests

func TestPrepareSession_Success(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	sessionID, err := executor.PrepareSession(context.Background(), task, "profile-123", "executor-123", "", "step-123")
	if err != nil {
		t.Fatalf("PrepareSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Verify session was created
	if len(repo.createTaskSessionCalls) != 1 {
		t.Errorf("Expected 1 CreateTaskSession call, got %d", len(repo.createTaskSessionCalls))
	}

	createdSession := repo.createTaskSessionCalls[0]
	if createdSession.TaskID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, createdSession.TaskID)
	}
	if createdSession.AgentProfileID != "profile-123" {
		t.Errorf("Expected agent profile ID profile-123, got %s", createdSession.AgentProfileID)
	}
	if createdSession.State != models.TaskSessionStateCreated {
		t.Errorf("Expected state CREATED, got %s", createdSession.State)
	}
	if !createdSession.IsPrimary {
		t.Error("Expected session to be primary")
	}

	// Verify SetSessionPrimary was called
	if len(repo.setSessionPrimaryCalls) != 1 {
		t.Errorf("Expected 1 SetSessionPrimary call, got %d", len(repo.setSessionPrimaryCalls))
	}
}

func TestPrepareSession_NoAgentProfileID(t *testing.T) {
	repo := newMockRepository()
	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
	}

	_, err := executor.PrepareSession(context.Background(), task, "", "executor-123", "", "step-123")
	if err != ErrNoAgentProfileID {
		t.Errorf("Expected ErrNoAgentProfileID, got %v", err)
	}
}

func TestPrepareSession_WithRepository(t *testing.T) {
	repo := newMockRepository()
	repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
		ID:           "task-repo-1",
		TaskID:       "task-123",
		RepositoryID: "repo-123",
		BaseBranch:   "main",
	}
	repo.repositories["repo-123"] = &models.Repository{
		ID:        "repo-123",
		LocalPath: "/path/to/repo",
	}

	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
	}

	sessionID, err := executor.PrepareSession(context.Background(), task, "profile-123", "", "", "")
	if err != nil {
		t.Fatalf("PrepareSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Verify session has repository info
	createdSession := repo.createTaskSessionCalls[0]
	if createdSession.RepositoryID != "repo-123" {
		t.Errorf("Expected repository ID repo-123, got %s", createdSession.RepositoryID)
	}
	if createdSession.BaseBranch != "main" {
		t.Errorf("Expected base branch main, got %s", createdSession.BaseBranch)
	}
}

func TestLaunchPreparedSession_Success(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session (as PrepareSession would)
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session

	launchCalled := false
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			if req.SessionID != "session-123" {
				t.Errorf("Expected session ID session-123, got %s", req.SessionID)
			}
			if req.TaskID != "task-123" {
				t.Errorf("Expected task ID task-123, got %s", req.TaskID)
			}
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", "profile-123", "", "test prompt", "", true)
	if err != nil {
		t.Fatalf("LaunchPreparedSession failed: %v", err)
	}

	if !launchCalled {
		t.Error("Expected LaunchAgent to be called")
	}

	if execution.SessionID != "session-123" {
		t.Errorf("Expected session ID session-123, got %s", execution.SessionID)
	}
	if execution.AgentExecutionID != "exec-123" {
		t.Errorf("Expected agent execution ID exec-123, got %s", execution.AgentExecutionID)
	}
	if execution.SessionState != v1.TaskSessionStateStarting {
		t.Errorf("Expected session state STARTING, got %s", execution.SessionState)
	}
}

func TestLaunchPreparedSession_SessionNotBelongsToTask(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session with different task ID
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "other-task",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
	}
	repo.sessions[session.ID] = session

	agentManager := &mockAgentManager{}
	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
	}

	_, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", "profile-123", "", "test prompt", "", true)
	if err == nil {
		t.Error("Expected error when session doesn't belong to task")
	}
}

func TestLaunchPreparedSession_WorkspaceOnly(t *testing.T) {
	repo := newMockRepository()

	// Pre-create session (as PrepareSession would)
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateCreated,
		StartedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	repo.sessions[session.ID] = session

	launchCalled := false
	startAgentCalled := false
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
		startAgentProcessFunc: func(ctx context.Context, id string) error {
			startAgentCalled = true
			return nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	// startAgent=false: should launch workspace but NOT start agent
	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", "profile-123", "", "", "", false)
	if err != nil {
		t.Fatalf("LaunchPreparedSession(startAgent=false) failed: %v", err)
	}

	if !launchCalled {
		t.Error("Expected LaunchAgent to be called (workspace setup)")
	}

	// Give goroutines a moment to run (there shouldn't be any)
	time.Sleep(50 * time.Millisecond)

	if startAgentCalled {
		t.Error("Expected StartAgentProcess NOT to be called when startAgent=false")
	}

	if execution.SessionState != v1.TaskSessionStateCreated {
		t.Errorf("Expected session state CREATED, got %s", execution.SessionState)
	}

	// Session in DB should remain CREATED
	updatedSession := repo.sessions["session-123"]
	if updatedSession.State != models.TaskSessionStateCreated {
		t.Errorf("Expected DB session state CREATED, got %s", updatedSession.State)
	}
}

func TestLaunchPreparedSession_ExistingWorkspace_StartAgent(t *testing.T) {
	repo := newMockRepository()

	// Session already has an AgentExecutionID (workspace previously launched)
	session := &models.TaskSession{
		ID:               "session-123",
		TaskID:           "task-123",
		AgentProfileID:   "profile-123",
		AgentExecutionID: "exec-existing",
		State:            models.TaskSessionStateCreated,
		StartedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	repo.sessions[session.ID] = session

	var startAgentCalled atomic.Bool
	descriptionSet := ""
	agentManager := &mockAgentManager{
		startAgentProcessFunc: func(ctx context.Context, id string) error {
			startAgentCalled.Store(true)
			if id != "exec-existing" {
				t.Errorf("Expected execution ID exec-existing, got %s", id)
			}
			return nil
		},
	}
	agentManager.setExecutionDescriptionFunc = func(ctx context.Context, id, desc string) error {
		descriptionSet = desc
		return nil
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
	}

	execution, err := executor.LaunchPreparedSession(context.Background(), task, "session-123", "profile-123", "", "build the feature", "", true)
	if err != nil {
		t.Fatalf("LaunchPreparedSession(existing workspace) failed: %v", err)
	}

	// Should use the existing execution ID
	if execution.AgentExecutionID != "exec-existing" {
		t.Errorf("Expected agent execution ID exec-existing, got %s", execution.AgentExecutionID)
	}

	if execution.SessionState != v1.TaskSessionStateStarting {
		t.Errorf("Expected session state STARTING, got %s", execution.SessionState)
	}

	// Description should have been set
	if descriptionSet != "build the feature" {
		t.Errorf("Expected description 'build the feature', got %q", descriptionSet)
	}

	// Wait for async goroutine
	time.Sleep(100 * time.Millisecond)

	if !startAgentCalled.Load() {
		t.Error("Expected StartAgentProcess to be called")
	}
}

func TestExecuteWithProfile_UsesPrepareThenLaunch(t *testing.T) {
	repo := newMockRepository()

	launchCalled := false
	agentManager := &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			launchCalled = true
			return &LaunchAgentResponse{
				AgentExecutionID: "exec-123",
				ContainerID:      "container-123",
				Status:           v1.AgentStatusStarting,
			}, nil
		},
	}

	executor := newTestExecutor(t, agentManager, repo)

	task := &v1.Task{
		ID:          "task-123",
		WorkspaceID: "workspace-123",
		Title:       "Test Task",
		Description: "Test description",
	}

	execution, err := executor.ExecuteWithProfile(context.Background(), task, "profile-123", "", "test prompt", "")
	if err != nil {
		t.Fatalf("ExecuteWithProfile failed: %v", err)
	}

	// Verify session was created (PrepareSession was called)
	if len(repo.createTaskSessionCalls) != 1 {
		t.Errorf("Expected 1 CreateTaskSession call (from PrepareSession), got %d", len(repo.createTaskSessionCalls))
	}

	// Verify agent was launched (LaunchPreparedSession was called)
	if !launchCalled {
		t.Error("Expected LaunchAgent to be called (from LaunchPreparedSession)")
	}

	if execution.TaskID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, execution.TaskID)
	}
}

func TestShouldUseWorktree(t *testing.T) {
	tests := []struct {
		executorType string
		want         bool
	}{
		{"worktree", true},
		{"local", false},
		{"local_docker", false},
		{"remote_docker", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := shouldUseWorktree(tt.executorType); got != tt.want {
			t.Errorf("shouldUseWorktree(%q) = %v, want %v", tt.executorType, got, tt.want)
		}
	}
}

func TestRepositoryCloneURL(t *testing.T) {
	tests := []struct {
		name string
		repo *models.Repository
		want string
	}{
		{
			name: "github repo",
			repo: &models.Repository{Provider: "github", ProviderOwner: "acme", ProviderName: "app"},
			want: "https://github.com/acme/app.git",
		},
		{
			name: "gitlab repo",
			repo: &models.Repository{Provider: "gitlab", ProviderOwner: "acme", ProviderName: "app"},
			want: "https://gitlab.com/acme/app.git",
		},
		{
			name: "bitbucket repo",
			repo: &models.Repository{Provider: "bitbucket", ProviderOwner: "acme", ProviderName: "app"},
			want: "https://bitbucket.org/acme/app.git",
		},
		{
			name: "unknown provider returns empty",
			repo: &models.Repository{Provider: "custom", ProviderOwner: "acme", ProviderName: "app"},
			want: "",
		},
		{
			name: "empty provider defaults to github",
			repo: &models.Repository{ProviderOwner: "acme", ProviderName: "app"},
			want: "https://github.com/acme/app.git",
		},
		{
			name: "missing owner returns empty",
			repo: &models.Repository{Provider: "github", ProviderName: "app"},
			want: "",
		},
		{
			name: "missing name returns empty",
			repo: &models.Repository{Provider: "github", ProviderOwner: "acme"},
			want: "",
		},
		{
			name: "both missing returns empty",
			repo: &models.Repository{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repositoryCloneURL(tt.repo); got != tt.want {
				t.Errorf("repositoryCloneURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
