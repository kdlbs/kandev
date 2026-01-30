package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type mockRepository struct {
	scriptsByRepo map[string][]*models.RepositoryScript
	sessions      map[string]*models.TaskSession
	executors     map[string]*models.Executor
}

func (m *mockRepository) CreateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	return nil
}
func (m *mockRepository) GetWorkspace(ctx context.Context, id string) (*models.Workspace, error) {
	return nil, nil
}
func (m *mockRepository) UpdateWorkspace(ctx context.Context, workspace *models.Workspace) error {
	return nil
}
func (m *mockRepository) DeleteWorkspace(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListWorkspaces(ctx context.Context) ([]*models.Workspace, error) {
	return nil, nil
}
func (m *mockRepository) CreateTask(ctx context.Context, task *models.Task) error {
	return nil
}
func (m *mockRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTask(ctx context.Context, task *models.Task) error {
	return nil
}
func (m *mockRepository) DeleteTask(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListTasks(ctx context.Context, boardID string) ([]*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) ListTasksByWorkflowStep(ctx context.Context, workflowStepID string) ([]*models.Task, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	return nil
}
func (m *mockRepository) AddTaskToBoard(ctx context.Context, taskID, boardID, columnID string, position int) error {
	return nil
}
func (m *mockRepository) RemoveTaskFromBoard(ctx context.Context, taskID, boardID string) error {
	return nil
}
func (m *mockRepository) CreateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	return nil
}
func (m *mockRepository) GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error) {
	return nil, nil
}
func (m *mockRepository) ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTaskRepository(ctx context.Context, taskRepo *models.TaskRepository) error {
	return nil
}
func (m *mockRepository) DeleteTaskRepository(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) DeleteTaskRepositoriesByTask(ctx context.Context, taskID string) error {
	return nil
}
func (m *mockRepository) GetPrimaryTaskRepository(ctx context.Context, taskID string) (*models.TaskRepository, error) {
	return nil, nil
}
func (m *mockRepository) CreateBoard(ctx context.Context, board *models.Board) error {
	return nil
}
func (m *mockRepository) GetBoard(ctx context.Context, id string) (*models.Board, error) {
	return nil, nil
}
func (m *mockRepository) UpdateBoard(ctx context.Context, board *models.Board) error {
	return nil
}
func (m *mockRepository) DeleteBoard(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListBoards(ctx context.Context, workspaceID string) ([]*models.Board, error) {
	return nil, nil
}
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
func (m *mockRepository) UpdateMessage(ctx context.Context, message *models.Message) error {
	return nil
}
func (m *mockRepository) ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error) {
	return nil, nil
}
func (m *mockRepository) ListMessagesPaginated(ctx context.Context, sessionID string, opts models.ListMessagesOptions) ([]*models.Message, bool, error) {
	return nil, false, nil
}
func (m *mockRepository) DeleteMessage(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) CreateTurn(ctx context.Context, turn *models.Turn) error {
	return nil
}
func (m *mockRepository) GetTurn(ctx context.Context, id string) (*models.Turn, error) {
	return nil, nil
}
func (m *mockRepository) GetActiveTurnBySessionID(ctx context.Context, sessionID string) (*models.Turn, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTurn(ctx context.Context, turn *models.Turn) error {
	return nil
}
func (m *mockRepository) CompleteTurn(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error) {
	return nil, nil
}
func (m *mockRepository) CreateTaskSession(ctx context.Context, session *models.TaskSession) error {
	return nil
}
func (m *mockRepository) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if m.sessions != nil {
		if session, ok := m.sessions[id]; ok {
			return session, nil
		}
	}
	return nil, fmt.Errorf("task session not found: %s", id)
}
func (m *mockRepository) GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) UpdateTaskSession(ctx context.Context, session *models.TaskSession) error {
	return nil
}
func (m *mockRepository) UpdateTaskSessionState(ctx context.Context, id string, state models.TaskSessionState, errorMessage string) error {
	return nil
}
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
func (m *mockRepository) DeleteTaskSession(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) CreateTaskSessionWorktree(ctx context.Context, sessionWorktree *models.TaskSessionWorktree) error {
	return nil
}
func (m *mockRepository) ListTaskSessionWorktrees(ctx context.Context, sessionID string) ([]*models.TaskSessionWorktree, error) {
	return nil, nil
}
func (m *mockRepository) DeleteTaskSessionWorktree(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) DeleteTaskSessionWorktreesBySession(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockRepository) CreateRepository(ctx context.Context, repository *models.Repository) error {
	return nil
}
func (m *mockRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	return nil, nil
}
func (m *mockRepository) UpdateRepository(ctx context.Context, repository *models.Repository) error {
	return nil
}
func (m *mockRepository) DeleteRepository(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListRepositories(ctx context.Context, workspaceID string) ([]*models.Repository, error) {
	return nil, nil
}
func (m *mockRepository) CreateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	return nil
}
func (m *mockRepository) GetRepositoryScript(ctx context.Context, id string) (*models.RepositoryScript, error) {
	return nil, nil
}
func (m *mockRepository) UpdateRepositoryScript(ctx context.Context, script *models.RepositoryScript) error {
	return nil
}
func (m *mockRepository) DeleteRepositoryScript(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListRepositoryScripts(ctx context.Context, repositoryID string) ([]*models.RepositoryScript, error) {
	if m.scriptsByRepo == nil {
		return nil, nil
	}
	return m.scriptsByRepo[repositoryID], nil
}
func (m *mockRepository) CreateExecutor(ctx context.Context, executor *models.Executor) error {
	return nil
}
func (m *mockRepository) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	if m.executors != nil {
		if executor, ok := m.executors[id]; ok {
			return executor, nil
		}
	}
	return nil, fmt.Errorf("executor not found: %s", id)
}
func (m *mockRepository) UpdateExecutor(ctx context.Context, executor *models.Executor) error {
	return nil
}
func (m *mockRepository) DeleteExecutor(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListExecutors(ctx context.Context) ([]*models.Executor, error) {
	return nil, nil
}
func (m *mockRepository) ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	return nil, nil
}
func (m *mockRepository) UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error {
	return nil
}
func (m *mockRepository) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	return nil, nil
}
func (m *mockRepository) GetExecutorRunningByTaskID(ctx context.Context, taskID string) (*models.ExecutorRunning, error) {
	return nil, nil
}
func (m *mockRepository) DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockRepository) CreateEnvironment(ctx context.Context, environment *models.Environment) error {
	return nil
}
func (m *mockRepository) GetEnvironment(ctx context.Context, id string) (*models.Environment, error) {
	return nil, nil
}
func (m *mockRepository) UpdateEnvironment(ctx context.Context, environment *models.Environment) error {
	return nil
}
func (m *mockRepository) DeleteEnvironment(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	return nil, nil
}
func (m *mockRepository) Close() error {
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
func (m *mockRepository) DeleteSessionCommit(ctx context.Context, id string) error {
	return nil
}
func (m *mockRepository) GetPrimarySessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error) {
	return nil, nil
}
func (m *mockRepository) GetPrimarySessionIDsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]string, error) {
	return make(map[string]string), nil
}
func (m *mockRepository) SetSessionPrimary(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockRepository) UpdateSessionWorkflowStep(ctx context.Context, sessionID string, stepID string) error {
	return nil
}
func (m *mockRepository) UpdateSessionReviewStatus(ctx context.Context, sessionID string, status string) error {
	return nil
}

func newTestService(t *testing.T, scripts map[string][]*models.RepositoryScript) *service.Service {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	repo := &mockRepository{scriptsByRepo: scripts}
	return service.NewService(repo, nil, log, service.RepositoryDiscoveryConfig{})
}

func addExecution(t *testing.T, mgr *lifecycle.Manager, exec *lifecycle.AgentExecution) {
	t.Helper()
	store := getExecutionStore(t, mgr)
	store.Add(exec)
}

func getExecutionStore(t *testing.T, mgr *lifecycle.Manager) *lifecycle.ExecutionStore {
	t.Helper()
	value := reflect.ValueOf(mgr).Elem().FieldByName("executionStore")
	if !value.IsValid() {
		t.Fatal("executionStore field not found on lifecycle manager")
	}
	value = reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
	store, ok := value.Interface().(*lifecycle.ExecutionStore)
	if !ok {
		t.Fatal("executionStore is not *lifecycle.ExecutionStore")
	}
	return store
}

func newLifecycleManager(t *testing.T, log *logger.Logger) *lifecycle.Manager {
	t.Helper()
	reg := registry.NewRegistry(log)
	return lifecycle.NewManager(reg, nil, nil, nil, nil, nil, nil, lifecycle.RuntimeFallbackWarn, log)
}

func newAgentctlClient(t *testing.T, serverURL string, log *logger.Logger) *agentctlclient.Client {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("failed to parse server port: %v", err)
	}
	return agentctlclient.NewClient(parsed.Hostname(), port, log)
}

func newProcessStopRouter(
	t *testing.T,
	svc *service.Service,
	lifecycleMgr *lifecycle.Manager,
	log *logger.Logger,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterProcessRoutes(router, svc, lifecycleMgr, log)
	return router
}

func TestResolveScriptCommandBuiltins(t *testing.T) {
	svc := newTestService(t, nil)
	repo := &models.Repository{ID: "repo-1", SetupScript: "echo setup", CleanupScript: "echo cleanup", DevScript: "echo dev"}

	cmd, kind, scriptName, err := resolveScriptCommand(context.Background(), svc, repo, "setup", "")
	if err != nil || cmd != "echo setup" || kind != "setup" || scriptName != "" {
		t.Fatalf("unexpected setup result: cmd=%q kind=%q script=%q err=%v", cmd, kind, scriptName, err)
	}

	cmd, kind, scriptName, err = resolveScriptCommand(context.Background(), svc, repo, "cleanup", "")
	if err != nil || cmd != "echo cleanup" || kind != "cleanup" || scriptName != "" {
		t.Fatalf("unexpected cleanup result: cmd=%q kind=%q script=%q err=%v", cmd, kind, scriptName, err)
	}

	cmd, kind, scriptName, err = resolveScriptCommand(context.Background(), svc, repo, "dev", "")
	if err != nil || cmd != "echo dev" || kind != "dev" || scriptName != "" {
		t.Fatalf("unexpected dev result: cmd=%q kind=%q script=%q err=%v", cmd, kind, scriptName, err)
	}
}

func TestResolveScriptCommandCustom(t *testing.T) {
	scripts := map[string][]*models.RepositoryScript{
		"repo-1": {
			{ID: "s1", RepositoryID: "repo-1", Name: "build", Command: "make build"},
		},
	}
	svc := newTestService(t, scripts)
	repo := &models.Repository{ID: "repo-1"}

	cmd, kind, scriptName, err := resolveScriptCommand(context.Background(), svc, repo, "custom", "build")
	if err != nil || cmd != "make build" || kind != "custom" || scriptName != "build" {
		t.Fatalf("unexpected custom result: cmd=%q kind=%q script=%q err=%v", cmd, kind, scriptName, err)
	}
}

func TestResolveScriptCommandErrors(t *testing.T) {
	svc := newTestService(t, nil)
	repo := &models.Repository{ID: "repo-1"}

	if _, _, _, err := resolveScriptCommand(context.Background(), svc, repo, "setup", ""); err == nil {
		t.Fatal("expected error for missing setup script")
	}
	if _, _, _, err := resolveScriptCommand(context.Background(), svc, repo, "cleanup", ""); err == nil {
		t.Fatal("expected error for missing cleanup script")
	}
	if _, _, _, err := resolveScriptCommand(context.Background(), svc, repo, "dev", ""); err == nil {
		t.Fatal("expected error for missing dev script")
	}
	if _, _, _, err := resolveScriptCommand(context.Background(), svc, repo, "custom", ""); err == nil {
		t.Fatal("expected error for missing custom script name")
	}
	if _, _, _, err := resolveScriptCommand(context.Background(), svc, repo, "custom", "unknown"); err == nil {
		t.Fatal("expected error for unknown custom script")
	}
}

func TestStopProcessRejectsDifferentSession(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	stopCalled := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/processes/stop":
			stopCalled.Store(true)
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/processes/"):
			resp := agentctlclient.ProcessInfo{
				ID:        "proc-1",
				SessionID: "session-other",
				Kind:      agentctlclient.ProcessKind("dev"),
				Status:    agentctlclient.ProcessStatus("running"),
			}
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	repo := &mockRepository{
		sessions: map[string]*models.TaskSession{
			"session-a": {ID: "session-a", TaskID: "task-1", ExecutorID: "exec-1"},
		},
		executors: map[string]*models.Executor{
			"exec-1": {ID: "exec-1", Type: models.ExecutorTypeLocalPC},
		},
	}
	svc := service.NewService(repo, nil, log, service.RepositoryDiscoveryConfig{})
	lifecycleMgr := newLifecycleManager(t, log)
	client := newAgentctlClient(t, server.URL, log)
	exec := (&lifecycle.RuntimeInstance{
		InstanceID: "exec-1",
		TaskID:     "task-1",
		SessionID:  "session-a",
		Client:     client,
	}).ToAgentExecution(&lifecycle.RuntimeCreateRequest{
		InstanceID:     "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-a",
		AgentProfileID: "profile-1",
		WorkspacePath:  "/tmp",
	})
	addExecution(t, lifecycleMgr, exec)

	router := newProcessStopRouter(t, svc, lifecycleMgr, log)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-a/processes/proc-1/stop", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatched session, got %d", resp.Code)
	}
	if stopCalled.Load() {
		t.Fatal("expected stop endpoint not to be called on session mismatch")
	}
}

func TestStopProcessAgentctlUnavailable(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	stopCalled := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/processes/stop":
			stopCalled.Store(true)
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/processes/"):
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	repo := &mockRepository{
		sessions: map[string]*models.TaskSession{
			"session-a": {ID: "session-a", TaskID: "task-1", ExecutorID: "exec-1"},
		},
		executors: map[string]*models.Executor{
			"exec-1": {ID: "exec-1", Type: models.ExecutorTypeLocalPC},
		},
	}
	svc := service.NewService(repo, nil, log, service.RepositoryDiscoveryConfig{})
	lifecycleMgr := newLifecycleManager(t, log)
	client := newAgentctlClient(t, server.URL, log)
	exec := (&lifecycle.RuntimeInstance{
		InstanceID: "exec-1",
		TaskID:     "task-1",
		SessionID:  "session-a",
		Client:     client,
	}).ToAgentExecution(&lifecycle.RuntimeCreateRequest{
		InstanceID:     "exec-1",
		TaskID:         "task-1",
		SessionID:      "session-a",
		AgentProfileID: "profile-1",
		WorkspacePath:  "/tmp",
	})
	addExecution(t, lifecycleMgr, exec)

	router := newProcessStopRouter(t, svc, lifecycleMgr, log)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-a/processes/proc-1/stop", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when agentctl is unavailable, got %d", resp.Code)
	}
	if stopCalled.Load() {
		t.Fatal("expected stop endpoint not to be called when get process fails")
	}
}
