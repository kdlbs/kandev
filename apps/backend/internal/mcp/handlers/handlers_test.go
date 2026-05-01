package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestTaskService creates a real task service with a temporary file-backed SQLite DB for integration tests.
// Returns the service and the raw repo (for seeding data).
func newTestTaskService(t *testing.T) (*service.Service, *sqliterepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB)
	require.NoError(t, err)
	_, err = worktree.NewSQLiteStore(sqlxDB, sqlxDB)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlxDB.Close()
		_ = cleanup()
	})

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	svc := service.NewService(service.Repos{
		Workspaces:   repo,
		Tasks:        repo,
		TaskRepos:    repo,
		Workflows:    repo,
		Messages:     repo,
		Turns:        repo,
		Sessions:     repo,
		GitSnapshots: repo,
		RepoEntities: repo,
		Executors:    repo,
		Environments: repo,
		Reviews:      repo,
	}, eventBus, log, service.RepositoryDiscoveryConfig{})
	return svc, repo
}

func TestHandleCreateTask_MissingTitle(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateTask_SubtaskMissingDescription(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":     "Fix bug",
		"parent_id": "task-parent",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateTask_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPCreateTask,
		Payload: json.RawMessage(`{invalid`),
	}

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleCreateTask_TopLevel_MissingWorkspaceID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":       "New task",
		"workflow_id": "wf-1",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateTask_TopLevel_MissingWorkflowID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":        "New task",
		"workspace_id": "ws-1",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

// mockSessionLauncher captures LaunchSession calls for testing autoStartTask.
type mockSessionLauncher struct {
	mu     sync.Mutex
	req    *orchestrator.LaunchSessionRequest
	called chan struct{}
}

func newMockSessionLauncher() *mockSessionLauncher {
	return &mockSessionLauncher{called: make(chan struct{})}
}

func (m *mockSessionLauncher) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	m.mu.Lock()
	m.req = req
	m.mu.Unlock()
	close(m.called)
	return &orchestrator.LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: "session-1",
	}, nil
}

func (m *mockSessionLauncher) getRequest() *orchestrator.LaunchSessionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.req
}

// The following methods satisfy the SessionLauncher interface but are not used by
// the autoStartTask tests. handleMessageTask tests use a dedicated fakeOrchestrator
// (see message_task_test.go) that exercises these paths.
func (m *mockSessionLauncher) PromptTask(context.Context, string, string, string, string, bool, []v1.MessageAttachment) (*orchestrator.PromptResult, error) {
	return nil, nil
}
func (m *mockSessionLauncher) StartCreatedSession(context.Context, string, string, string, string, bool, bool, []v1.MessageAttachment) (*executor.TaskExecution, error) {
	return nil, nil
}
func (m *mockSessionLauncher) ResumeTaskSession(context.Context, string, string) (*executor.TaskExecution, error) {
	return nil, nil
}
func (m *mockSessionLauncher) GetMessageQueue() *messagequeue.Service { return nil }

func TestAutoStartTask_DefaultsToWorktreeExecutor(t *testing.T) {
	launcher := newMockSessionLauncher()
	log := testLogger(t)
	h := &Handlers{
		sessionLauncher: launcher,
		logger:          log.WithFields(),
	}

	task := &models.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
	}

	// Call with agent profile but no executor info
	h.autoStartTask(task, "agent-profile-1", "", "")

	select {
	case <-launcher.called:
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchSession was not called within timeout")
	}

	req := launcher.getRequest()
	assert.Equal(t, models.ExecutorIDWorktree, req.ExecutorID, "should default to exec-worktree")
	assert.Equal(t, "", req.ExecutorProfileID)
	assert.Equal(t, "agent-profile-1", req.AgentProfileID)
}

func TestAutoStartTask_ExplicitExecutorProfilePreserved(t *testing.T) {
	launcher := newMockSessionLauncher()
	log := testLogger(t)
	h := &Handlers{
		sessionLauncher: launcher,
		logger:          log.WithFields(),
	}

	task := &models.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
	}

	// Call with explicit executor profile
	h.autoStartTask(task, "agent-profile-1", "exec-profile-docker", "")

	select {
	case <-launcher.called:
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchSession was not called within timeout")
	}

	req := launcher.getRequest()
	assert.Equal(t, "exec-profile-docker", req.ExecutorProfileID, "explicit executor profile should be preserved")
	assert.Equal(t, "", req.ExecutorID, "executorID should be empty when profile is set")
}

func TestResolveTaskRepositories_ExplicitRepos(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	explicit := []mcpRepositoryInput{
		{RepositoryID: "repo-1", BaseBranch: "main"},
		{LocalPath: "/tmp/myrepo"},
	}
	result, err := h.resolveTaskRepositories(context.Background(), "", "", explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 2)
	assert.Equal(t, "repo-1", result.Repos[0].RepositoryID)
	assert.Equal(t, "main", result.Repos[0].BaseBranch)
	assert.Equal(t, "/tmp/myrepo", result.Repos[1].LocalPath)
	assert.Empty(t, result.WorkspaceID, "workspace should not be set for explicit repos")
	assert.Empty(t, result.WorkflowID, "workflow should not be set for explicit repos")
}

func TestResolveTaskRepositories_ExplicitGitHubURL(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	explicit := []mcpRepositoryInput{
		{GitHubURL: "https://github.com/acme/widgets", BaseBranch: "main"},
	}
	result, err := h.resolveTaskRepositories(context.Background(), "", "", explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 1)
	assert.Equal(t, "https://github.com/acme/widgets", result.Repos[0].GitHubURL)
	assert.Equal(t, "main", result.Repos[0].BaseBranch)
}

func TestResolveTaskRepositories_NoInputs_ReturnsEmpty(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	result, err := h.resolveTaskRepositories(context.Background(), "", "", nil)
	require.NoError(t, err)
	assert.Empty(t, result.Repos)
}

func TestResolveTaskRepositories_ExplicitRepos_SkipsParentLookup(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	// Even with a parentID, explicit repos should be used without looking up the parent
	// (taskSvc is nil — if it tried to call GetTask it would panic).
	explicit := []mcpRepositoryInput{{RepositoryID: "repo-explicit"}}
	result, err := h.resolveTaskRepositories(context.Background(), "some-parent", "some-source", explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 1)
	assert.Equal(t, "repo-explicit", result.Repos[0].RepositoryID)
}

// --- Integration tests using real task service ---

func TestResolveTaskRepositories_EphemeralParent_Rejected(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	// Seed workspace and ephemeral task
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Quick Chat",
		IsEphemeral: true,
	})
	require.NoError(t, err)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	_, err = h.resolveTaskRepositories(ctx, task.ID, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ephemeral")
}

func TestResolveTaskRepositories_ExplicitRepos_InheritsSourceWorkspace(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	// Seed workspace and source task to inherit from.
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	_, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Source task",
	})
	require.NoError(t, err)
	tasks, err := svc.ListTasks(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	sourceTaskID := tasks[0].ID

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	explicit := []mcpRepositoryInput{
		{GitHubURL: "https://github.com/acme/widgets", BaseBranch: "main"},
	}
	result, err := h.resolveTaskRepositories(ctx, "", sourceTaskID, explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 1)
	assert.Equal(t, "https://github.com/acme/widgets", result.Repos[0].GitHubURL)
	assert.Equal(t, "ws-1", result.WorkspaceID, "should inherit source task workspace even with explicit repos")
}

func TestResolveTaskRepositories_SourceTask_InheritsWorkspace(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	// Seed workspace, workflow, and source task
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	_, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Source task",
	})
	require.NoError(t, err)
	tasks, err := svc.ListTasks(ctx, "wf-1")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	sourceTaskID := tasks[0].ID

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	result, err := h.resolveTaskRepositories(ctx, "", sourceTaskID, nil)
	require.NoError(t, err)
	assert.Equal(t, "ws-1", result.WorkspaceID, "should inherit workspace from source task")
	assert.Empty(t, result.WorkflowID, "should NOT inherit workflow from source task")
}

func TestHandleCreateTask_AutoResolvesWorkspaceAndWorkflow(t *testing.T) {
	svc, _ := newTestTaskService(t)
	ctx := context.Background()

	// The DB is seeded with a default workspace and workflow by repository.Provide.
	// Verify exactly 1 of each exists so auto-resolve works.
	workspaces, wsErr := svc.ListWorkspaces(ctx)
	require.NoError(t, wsErr)
	require.Len(t, workspaces, 1, "should have exactly 1 default workspace")

	workflows, wfErr := svc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, wfErr)
	require.Len(t, workflows, 1, "should have exactly 1 default workflow")

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}
	// No workspace_id or workflow_id provided — should auto-resolve from defaults
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":       "Auto-resolved task",
		"start_agent": false,
	})

	resp, err := h.handleCreateTask(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	if resp.Type == ws.MessageTypeError {
		t.Logf("error payload: %s", string(resp.Payload))
	}
	assert.Equal(t, ws.MessageTypeResponse, resp.Type, "should succeed with auto-resolved workspace and workflow")
}

func TestHandleCreateTask_AutoResolveFailsWithMultipleWorkflows(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	// The DB already has 1 default workspace + 1 default workflow.
	// Add a second workflow to make auto-resolution ambiguous.
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-extra", WorkspaceID: workspaces[0].ID, Name: "Extra Board"}))

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":       "Task",
		"start_agent": false,
	})

	resp, err := h.handleCreateTask(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
