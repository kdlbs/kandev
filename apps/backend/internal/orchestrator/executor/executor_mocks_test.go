package executor

import (
	"context"
	"fmt"
	"sync"
	"testing"

	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// mockAgentManager implements AgentManagerClient for testing
type mockAgentManager struct {
	launchAgentFunc                  func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error)
	startAgentProcessFunc            func(ctx context.Context, agentExecutionID string) error
	stopAgentFunc                    func(ctx context.Context, agentExecutionID string, force bool) error
	resolveAgentProfileFunc          func(ctx context.Context, profileID string) (*AgentProfileInfo, error)
	setExecutionDescriptionFunc      func(ctx context.Context, agentExecutionID string, description string) error
	getExecutionIDForSessionFunc     func(ctx context.Context, sessionID string) (string, error)
	isAgentRunningForSessionFunc     func(ctx context.Context, sessionID string) bool
	cleanupStaleExecutionFunc        func(ctx context.Context, sessionID string) error
	launchAgentCallCount             int
	cleanupStaleExecutionCallCount   int
	isAgentRunningForSessionCallArgs []string
}

func (m *mockAgentManager) LaunchAgent(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
	m.launchAgentCallCount++
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

func (m *mockAgentManager) SetMcpMode(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockAgentManager) StartAgentProcess(ctx context.Context, agentExecutionID string) error {
	if m.startAgentProcessFunc != nil {
		return m.startAgentProcessFunc(ctx, agentExecutionID)
	}
	return nil
}

func (m *mockAgentManager) StopAgent(ctx context.Context, agentExecutionID string, force bool) error {
	if m.stopAgentFunc != nil {
		return m.stopAgentFunc(ctx, agentExecutionID, force)
	}
	return nil
}

func (m *mockAgentManager) StopAgentWithReason(ctx context.Context, agentExecutionID string, reason string, force bool) error {
	return m.StopAgent(ctx, agentExecutionID, force)
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

func (m *mockAgentManager) RestartAgentProcess(ctx context.Context, agentExecutionID string) error {
	return nil
}
func (m *mockAgentManager) ResetAgentContext(ctx context.Context, agentExecutionID string) error {
	return nil
}

func (m *mockAgentManager) SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error {
	return fmt.Errorf("not supported")
}

func (m *mockAgentManager) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	m.isAgentRunningForSessionCallArgs = append(m.isAgentRunningForSessionCallArgs, sessionID)
	if m.isAgentRunningForSessionFunc != nil {
		return m.isAgentRunningForSessionFunc(ctx, sessionID)
	}
	return false
}

func (m *mockAgentManager) WasSessionInitialized(_ string) bool { return false }
func (m *mockAgentManager) GetSessionAuthMethods(_ string) []streams.AuthMethodInfo {
	return nil
}
func (m *mockAgentManager) IsPassthroughSession(ctx context.Context, sessionID string) bool {
	return false
}
func (m *mockAgentManager) WritePassthroughStdin(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *mockAgentManager) MarkPassthroughRunning(_ string) error {
	return nil
}

func (m *mockAgentManager) GetRemoteRuntimeStatusBySession(ctx context.Context, sessionID string) (*RemoteRuntimeStatus, error) {
	return nil, nil
}
func (m *mockAgentManager) PollRemoteStatusForRecords(ctx context.Context, records []RemoteStatusPollRequest) {
}
func (m *mockAgentManager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	m.cleanupStaleExecutionCallCount++
	if m.cleanupStaleExecutionFunc != nil {
		return m.cleanupStaleExecutionFunc(ctx, sessionID)
	}
	return nil
}
func (m *mockAgentManager) EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) error {
	return nil
}
func (m *mockAgentManager) GetExecutionIDForSession(ctx context.Context, sessionID string) (string, error) {
	if m.getExecutionIDForSessionFunc != nil {
		return m.getExecutionIDForSessionFunc(ctx, sessionID)
	}
	return "", fmt.Errorf("no execution found for session %s", sessionID)
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

func (m *mockAgentManager) GetGitLog(ctx context.Context, sessionID, baseCommit string, limit int, targetBranch string) (*client.GitLogResult, error) {
	return nil, nil
}

func (m *mockAgentManager) GetCumulativeDiff(ctx context.Context, sessionID, baseCommit string) (*client.CumulativeDiffResult, error) {
	return nil, nil
}

func (m *mockAgentManager) GetGitStatus(ctx context.Context, sessionID string) (*client.GitStatusResult, error) {
	// Return a mock git status with a head commit for base commit capture
	return &client.GitStatusResult{
		Success:    true,
		Branch:     "main",
		HeadCommit: "abc123def456",
	}, nil
}

func (m *mockAgentManager) WaitForAgentctlReady(ctx context.Context, sessionID string) error {
	// Mock returns immediately
	return nil
}

// mockRepository implements executorStore for testing
type mockRepository struct {
	mu                   sync.Mutex
	sessions             map[string]*models.TaskSession
	tasks                map[string]*models.Task
	taskRepositories     map[string]*models.TaskRepository
	repositories         map[string]*models.Repository
	executors            map[string]*models.Executor
	executorsRunning     map[string]*models.ExecutorRunning
	taskEnvironments     map[string]*models.TaskEnvironment
	taskEnvironmentRepos map[string][]*models.TaskEnvironmentRepo // env_id → rows
	sessionWorktrees     []*models.TaskSessionWorktree

	// Optional hook to inject behavior into GetTaskSession (e.g. simulate a
	// transient DB error); if nil, the default map lookup is used.
	getTaskSessionFunc func(ctx context.Context, id string) (*models.TaskSession, error)

	// Track calls for verification
	createTaskSessionCalls     []*models.TaskSession
	updateTaskSessionCalls     []*models.TaskSession
	setSessionPrimaryCalls     []string
	createTaskEnvironmentCalls []*models.TaskEnvironment
	updateTaskEnvironmentCalls []*models.TaskEnvironment
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		sessions:             make(map[string]*models.TaskSession),
		tasks:                make(map[string]*models.Task),
		taskRepositories:     make(map[string]*models.TaskRepository),
		repositories:         make(map[string]*models.Repository),
		executors:            make(map[string]*models.Executor),
		executorsRunning:     make(map[string]*models.ExecutorRunning),
		taskEnvironments:     make(map[string]*models.TaskEnvironment),
		taskEnvironmentRepos: make(map[string][]*models.TaskEnvironmentRepo),
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createTaskSessionCalls = append(m.createTaskSessionCalls, session)
	m.sessions[session.ID] = session
	return nil
}

func (m *mockRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	m.mu.Lock()
	fn := m.getTaskSessionFunc
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, id)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[id]; ok {
		return session, nil
	}
	return nil, nil
}

func (m *mockRepository) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateTaskSessionCalls = append(m.updateTaskSessionCalls, session)
	m.sessions[session.ID] = session
	return nil
}

func (m *mockRepository) SetSessionPrimary(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setSessionPrimaryCalls = append(m.setSessionPrimaryCalls, sessionID)
	return nil
}

func (m *mockRepository) UpdateTaskSessionState(ctx context.Context, sessionID string, state models.TaskSessionState, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[sessionID]; ok {
		session.State = state
		session.ErrorMessage = errorMessage
	}
	return nil
}

func (m *mockRepository) UpdateTaskSessionBaseCommit(ctx context.Context, sessionID string, baseCommitSHA string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.sessions[sessionID]; ok {
		session.BaseCommitSHA = baseCommitSHA
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

func (m *mockRepository) CreateTaskSessionWorktree(_ context.Context, worktree *models.TaskSessionWorktree) error {
	m.sessionWorktrees = append(m.sessionWorktrees, worktree)
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
	if task, ok := m.tasks[id]; ok {
		return task, nil
	}
	return nil, nil
}
func (m *mockRepository) UpdateTask(ctx context.Context, task *models.Task) error { return nil }
func (m *mockRepository) DeleteTask(ctx context.Context, id string) error         { return nil }
func (m *mockRepository) ListTasks(ctx context.Context, workflowID string) ([]*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) ListTasksByWorkspace(ctx context.Context, workspaceID, workflowID, repositoryID, query string, page, pageSize int, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool) ([]*models.Task, int, error) {
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
	var out []*models.TaskRepository
	for _, tr := range m.taskRepositories {
		if tr.TaskID == taskID {
			out = append(out, tr)
		}
	}
	// Stable order by Position so callers (and tests) see deterministic results.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Position < out[j-1].Position; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
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
func (m *mockRepository) ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*models.Workflow, error) {
	return nil, nil
}
func (m *mockRepository) ReorderWorkflows(ctx context.Context, workspaceID string, workflowIDs []string) error {
	return nil
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
func (m *mockRepository) SearchMessages(ctx context.Context, sessionID string, opts models.SearchMessagesOptions) ([]*models.Message, error) {
	return nil, nil
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
func (m *mockRepository) CompletePendingToolCallsForTurn(ctx context.Context, turnID string) (int64, error) {
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
func (m *mockRepository) GetActiveTaskInfoByAgentProfile(ctx context.Context, agentProfileID string) ([]agentdto.ActiveTaskInfo, error) {
	return nil, nil
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
func (m *mockRepository) DeleteEphemeralTasksByAgentProfile(ctx context.Context, agentProfileID string) (int64, error) {
	return 0, nil
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
func (m *mockRepository) UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error {
	return nil
}
func (m *mockRepository) SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error {
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
	if running, ok := m.executorsRunning[sessionID]; ok {
		return running, nil
	}
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

// Task environment operations
func (m *mockRepository) GetTaskEnvironmentByTaskID(_ context.Context, taskID string) (*models.TaskEnvironment, error) {
	for _, env := range m.taskEnvironments {
		if env.TaskID == taskID {
			return env, nil
		}
	}
	return nil, nil
}
func (m *mockRepository) CreateTaskEnvironment(_ context.Context, env *models.TaskEnvironment) error {
	if env.ID == "" {
		env.ID = "env-" + env.TaskID
	}
	m.createTaskEnvironmentCalls = append(m.createTaskEnvironmentCalls, env)
	m.taskEnvironments[env.ID] = env
	for _, r := range env.Repos {
		r.TaskEnvironmentID = env.ID
		if r.ID == "" {
			r.ID = env.ID + "-repo-" + r.RepositoryID
		}
	}
	if len(env.Repos) > 0 {
		m.taskEnvironmentRepos[env.ID] = append(m.taskEnvironmentRepos[env.ID], env.Repos...)
	}
	return nil
}
func (m *mockRepository) UpdateTaskEnvironment(_ context.Context, env *models.TaskEnvironment) error {
	if env.ID == "" {
		return nil
	}
	m.updateTaskEnvironmentCalls = append(m.updateTaskEnvironmentCalls, env)
	m.taskEnvironments[env.ID] = env
	return nil
}
func (m *mockRepository) CreateTaskEnvironmentRepo(_ context.Context, repo *models.TaskEnvironmentRepo) error {
	m.taskEnvironmentRepos[repo.TaskEnvironmentID] = append(m.taskEnvironmentRepos[repo.TaskEnvironmentID], repo)
	return nil
}
func (m *mockRepository) ListTaskEnvironmentRepos(_ context.Context, envID string) ([]*models.TaskEnvironmentRepo, error) {
	return m.taskEnvironmentRepos[envID], nil
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

// mockCapabilities implements ExecutorTypeCapabilities for testing.
type mockCapabilities struct{}

func (m *mockCapabilities) RequiresCloneURL(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeRemoteDocker, models.ExecutorTypeSprites:
		return true
	default:
		return false
	}
}

func (m *mockCapabilities) ShouldApplyPreferredShell(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorTypeMockRemote:
		return true
	default:
		return false
	}
}

// Helper to create a test executor
func newTestExecutor(t *testing.T, agentManager AgentManagerClient, repo *mockRepository) *Executor {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	exec := NewExecutor(agentManager, repo, log, ExecutorConfig{
		ShellPrefs: &mockShellPrefs{},
	})
	exec.SetCapabilities(&mockCapabilities{})
	return exec
}
