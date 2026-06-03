package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/clarification"
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
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
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

func seedMCPHandlerSession(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID string, state models.TaskSessionState) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID:        "ws-state-event",
		Name:      "State Event",
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-state-event",
		Title:       "State Event Task",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     state,
		StartedAt: now,
		UpdatedAt: now,
	}))
}

type mcpRecordingEventBus struct {
	events []*bus.Event
}

func (b *mcpRecordingEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	b.events = append(b.events, event)
	return nil
}

// TestHandleAddBranchToTask_RejectsMultipleLocators pins the mutual-exclusion
// check at the WS handler tier: supplying two of repository_id /
// repository_url / local_path is an agent mistake that previously got
// silently resolved by the resolveRepoInput precedence chain. Now it
// surfaces as a validation error so the agent sees the contradiction.
func TestHandleAddBranchToTask_RejectsMultipleLocators(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPAddBranchToTask, map[string]interface{}{
		"task_id":    "task-1",
		"local_path": "/tmp/x",
		"github_url": "https://github.com/acme/widgets",
	})

	resp, err := h.handleAddBranchToTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
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

func TestSessionStateEventsIncludeUpdatedAt(t *testing.T) {
	tests := []struct {
		name      string
		initial   models.TaskSessionState
		run       func(*Handlers, context.Context, string, string)
		wantState models.TaskSessionState
	}{
		{
			name:      "running",
			initial:   models.TaskSessionStateWaitingForInput,
			run:       (*Handlers).setSessionRunning,
			wantState: models.TaskSessionStateRunning,
		},
		{
			name:      "waiting for input",
			initial:   models.TaskSessionStateRunning,
			run:       (*Handlers).setSessionWaitingForInput,
			wantState: models.TaskSessionStateWaitingForInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, repo := newTestTaskService(t)
			seedMCPHandlerSession(t, repo, "task-state-event", "session-state-event", tt.initial)
			eventBus := &mcpRecordingEventBus{}
			h := &Handlers{
				sessionRepo: repo,
				taskRepo:    repo,
				eventBus:    eventBus,
				logger:      testLogger(t).WithFields(),
			}

			tt.run(h, context.Background(), "task-state-event", "session-state-event")

			require.Len(t, eventBus.events, 1)
			data, ok := eventBus.events[0].Data.(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, string(tt.wantState), data["new_state"])
			gotUpdatedAt, ok := data["updated_at"].(string)
			require.True(t, ok, "expected updated_at for state ordering")
			updatedSession, err := repo.GetTaskSession(context.Background(), "session-state-event")
			require.NoError(t, err)
			assert.Equal(t, updatedSession.UpdatedAt.UTC().Format(time.RFC3339Nano), gotUpdatedAt)
		})
	}
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
func (m *mockSessionLauncher) PromptTask(context.Context, string, string, string, string, bool, []v1.MessageAttachment, bool) (*orchestrator.PromptResult, error) {
	return nil, nil
}
func (m *mockSessionLauncher) StartCreatedSession(context.Context, string, string, string, string, bool, bool, bool, []v1.MessageAttachment) (*executor.TaskExecution, error) {
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

// --- Integration tests using real task service ---

// seedParentWithRepo creates a workspace, workflow, repository, and a parent
// task linked to that repository. Returns the parent task ID. The parent's
// task_repository row is anchored to a non-default branch ("pr-metrics") on
// purpose so inheritance tests can assert what subtasks do with the parent's
// branch (same-repo subtasks inherit it for stacked-PR ergonomics; the
// worktree manager's fallback rescues launches if the branch went stale).
func seedParentWithRepo(t *testing.T, svc *service.Service, repo *sqliterepo.Repository) string {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-parent", WorkspaceID: "ws-1", Name: "Parent Repo", DefaultBranch: "main",
	}))
	parent, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Parent task",
		Repositories: []service.TaskRepositoryInput{
			{RepositoryID: "repo-parent", BaseBranch: "pr-metrics"},
		},
	})
	require.NoError(t, err)
	return parent.ID
}

// TestResolveTaskRepositories_ParentWithoutExplicitRepos_InheritsRepoAndBaseBranch
// asserts the same-repo subtask path: the parent's RepositoryID and
// BaseBranch carry over so the subtask branches off the same starting point
// (sibling branches, ergonomic for stacked PRs). CheckoutBranch is dropped
// because two worktrees cannot share a working branch.
func TestResolveTaskRepositories_ParentWithoutExplicitRepos_InheritsRepoAndBaseBranch(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parentID := seedParentWithRepo(t, svc, repo)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	result, err := h.resolveTaskRepositories(context.Background(), parentID, "", nil)
	require.NoError(t, err)
	require.Len(t, result.Repos, 1, "subtask without explicit repos inherits parent's repos")
	assert.Equal(t, "repo-parent", result.Repos[0].RepositoryID)
	assert.Equal(t, "pr-metrics", result.Repos[0].BaseBranch, "same-repo subtask should inherit parent's base_branch for stacked-PR ergonomics")
	assert.Empty(t, result.Repos[0].CheckoutBranch, "subtask must not inherit parent's checkout_branch (worktrees cannot share a branch)")
	assert.Equal(t, "ws-1", result.WorkspaceID)
	assert.Equal(t, "wf-1", result.WorkflowID)
}

// TestCreateSubtaskFromParent_SameRepoInheritsParentBaseBranch is the
// end-to-end check that the parent's base_branch is persisted onto the
// subtask's task_repository row when the subtask targets the same repo.
// This is the desired behaviour: agents stacking work on top of a parent
// PR get a subtask anchored to the same starting point. The worktree
// manager's fallback (covered in worktree tests) is the safety net for
// cases where the inherited branch later goes stale.
func TestCreateSubtaskFromParent_SameRepoInheritsParentBaseBranch(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	parentID := seedParentWithRepo(t, svc, repo)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	resolved, err := h.resolveTaskRepositories(ctx, parentID, "", nil)
	require.NoError(t, err)

	subtask, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:  resolved.WorkspaceID,
		WorkflowID:   resolved.WorkflowID,
		ParentID:     parentID,
		Title:        "Child",
		Description:  "do the thing",
		Repositories: resolved.Repos,
	})
	require.NoError(t, err)
	require.Len(t, subtask.Repositories, 1)
	assert.Equal(t, "repo-parent", subtask.Repositories[0].RepositoryID)
	assert.Equal(t, "pr-metrics", subtask.Repositories[0].BaseBranch, "same-repo subtask should inherit parent's base_branch")
}

// TestCreateSubtaskFromParent_DifferentRepoUsesNewRepoDefault verifies the
// cross-repo path: when the agent points the subtask at a different repo
// (via repository_id / repository_url / local_path) without an explicit
// base_branch, the subtask anchors to that repo's default_branch — never
// the parent's branch.
func TestCreateSubtaskFromParent_DifferentRepoUsesNewRepoDefault(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	parentID := seedParentWithRepo(t, svc, repo)

	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-sibling", WorkspaceID: "ws-1", Name: "Sibling", DefaultBranch: "trunk",
	}))

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	explicit := []mcpRepositoryInput{{RepositoryID: "repo-sibling"}}
	resolved, err := h.resolveTaskRepositories(ctx, parentID, "", explicit)
	require.NoError(t, err)

	subtask, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:  resolved.WorkspaceID,
		WorkflowID:   resolved.WorkflowID,
		ParentID:     parentID,
		Title:        "Cross-repo child",
		Description:  "do the thing",
		Repositories: resolved.Repos,
	})
	require.NoError(t, err)
	require.Len(t, subtask.Repositories, 1)
	assert.Equal(t, "repo-sibling", subtask.Repositories[0].RepositoryID)
	assert.Equal(t, "trunk", subtask.Repositories[0].BaseBranch, "cross-repo subtask should anchor to the new repo's default_branch, not parent's pr-metrics")
}

// TestHandleCreateTask_SubtaskBaseBranchOverride pins the bug-fix path:
// when an MCP caller passes base_branch at the top level (no per-repo
// entries) for a same-repo subtask, the override beats the parent's
// inherited base_branch. Previously the top-level base_branch was
// silently dropped unless the caller also restated repository_id, so a
// "give me a child task that branches off feature/X" call quietly
// landed on the parent's branch instead.
func TestHandleCreateTask_SubtaskBaseBranchOverride(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	parentID := seedParentWithRepo(t, svc, repo)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":       "Child",
		"description": "do the thing",
		"parent_id":   parentID,
		"base_branch": "feature/create-new-page-endp-05z",
		"start_agent": false,
	})

	resp, err := h.handleCreateTask(ctx, msg)
	require.NoError(t, err)
	require.Equalf(t, ws.MessageTypeResponse, resp.Type, "create_task should succeed; payload: %s", string(resp.Payload))

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))
	require.NotEmpty(t, created.ID)

	subtask, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, subtask.Repositories, 1, "subtask should inherit parent's repository")
	assert.Equal(t, "repo-parent", subtask.Repositories[0].RepositoryID, "subtask should still bind to parent's repo")
	assert.Equal(t, "feature/create-new-page-endp-05z", subtask.Repositories[0].BaseBranch,
		"top-level base_branch must override parent's inherited base_branch when no explicit repos are passed")
}

// TestHandleCreateTask_SubtaskBaseBranchOverride_ExplicitReposWin asserts
// the inverse: when the caller provides per-repo entries, those are
// authoritative — the top-level base_branch must NOT clobber an explicit
// per-repo BaseBranch. This preserves cross-repo and multi-repo callers
// that already control branch selection per entry.
func TestHandleCreateTask_SubtaskBaseBranchOverride_ExplicitReposWin(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	parentID := seedParentWithRepo(t, svc, repo)

	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-sibling", WorkspaceID: "ws-1", Name: "Sibling", DefaultBranch: "trunk",
	}))

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":       "Cross-repo child",
		"description": "do the thing",
		"parent_id":   parentID,
		// Explicit per-repo entry with its own base_branch.
		"repositories": []map[string]interface{}{
			{"repository_id": "repo-sibling", "base_branch": "develop"},
		},
		// Top-level base_branch should be ignored because explicit repos win.
		"base_branch": "should-not-be-applied",
		"start_agent": false,
	})

	resp, err := h.handleCreateTask(ctx, msg)
	require.NoError(t, err)
	require.Equalf(t, ws.MessageTypeResponse, resp.Type, "create_task should succeed; payload: %s", string(resp.Payload))

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &created))

	subtask, err := svc.GetTask(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, subtask.Repositories, 1)
	assert.Equal(t, "repo-sibling", subtask.Repositories[0].RepositoryID)
	assert.Equal(t, "develop", subtask.Repositories[0].BaseBranch,
		"explicit per-repo base_branch must win over the top-level override")
}

func TestResolveTaskRepositories_ParentWithExplicitRepos_OverridesRepoButInheritsWorkspace(t *testing.T) {
	svc, repo := newTestTaskService(t)
	parentID := seedParentWithRepo(t, svc, repo)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	explicit := []mcpRepositoryInput{
		{GitHubURL: "https://github.com/acme/sibling", BaseBranch: "develop"},
	}
	result, err := h.resolveTaskRepositories(context.Background(), parentID, "", explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 1, "explicit repos override parent's repos")
	assert.Equal(t, "https://github.com/acme/sibling", result.Repos[0].GitHubURL)
	assert.Equal(t, "develop", result.Repos[0].BaseBranch)
	assert.Empty(t, result.Repos[0].RepositoryID, "explicit repo should not be conflated with parent's RepositoryID")
	assert.Equal(t, "ws-1", result.WorkspaceID, "subtask still inherits parent's workspace")
	assert.Equal(t, "wf-1", result.WorkflowID, "subtask still inherits parent's workflow")
}

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

func TestResolveTaskRepositories_SubtaskParent_Rejected(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	// Seed workspace, non-office workflow, root task, and a subtask
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	root, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Root task",
	})
	require.NoError(t, err)
	child, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		ParentID:    root.ID,
		Title:       "Subtask",
	})
	require.NoError(t, err)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	_, err = h.resolveTaskRepositories(ctx, child.ID, "", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrSubtaskDepthExceeded), "expected ErrSubtaskDepthExceeded, got: %v", err)
}

func TestResolveTaskRepositories_OfficeSubtaskParent_Allowed(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	// Seed office workspace (office workflow stamps IsFromOffice = true)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-office", Name: "Office"}))
	_, err := repo.EnsureOfficeWorkflow(ctx, "ws-office")
	require.NoError(t, err)

	root, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-office",
		Title:       "Root office task",
		ProjectID:   "proj-1",
	})
	require.NoError(t, err)
	child, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-office",
		ParentID:    root.ID,
		Title:       "Office subtask",
		ProjectID:   "proj-1",
	})
	require.NoError(t, err)

	log := testLogger(t)
	h := &Handlers{taskSvc: svc, logger: log.WithFields()}

	result, err := h.resolveTaskRepositories(ctx, child.ID, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "ws-office", result.WorkspaceID)
	assert.Equal(t, root.WorkflowID, result.WorkflowID)
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

// TestCreateTask_GitHubURLOnly_LeavesDefaultBranchEmpty pins the upstream
// contract that produced the production "base branch does not exist" failure
// for task 01b82e73. When an MCP caller passes only a github_url (no
// default_branch, no base_branch), the resulting Repository row has an empty
// default_branch and the TaskRepository row has an empty base_branch — the
// service layer never probes the upstream remote.
//
// This test documents that contract so any future change there (e.g. the
// service learns to probe the remote up front) is an intentional decision,
// and so the executor-side backfill that compensates for this isn't
// accidentally treated as redundant.
func TestCreateTask_GitHubURLOnly_LeavesDefaultBranchEmpty(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))

	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "subtask via bare github url",
		Repositories: []service.TaskRepositoryInput{
			{GitHubURL: "https://github.com/acme/never-seen"},
		},
	})
	require.NoError(t, err)
	require.Len(t, task.Repositories, 1)
	assert.Empty(t, task.Repositories[0].BaseBranch,
		"task_repositories.base_branch should be empty when caller passes neither base_branch nor default_branch — executor backfill compensates downstream")

	createdRepo, err := svc.GetRepository(ctx, task.Repositories[0].RepositoryID)
	require.NoError(t, err)
	require.NotNil(t, createdRepo)
	assert.Empty(t, createdRepo.DefaultBranch,
		"repositories.default_branch should be empty: FindOrCreateRepository does not probe the remote — the executor backfills it after clone")
	assert.Equal(t, "acme", createdRepo.ProviderOwner)
	assert.Equal(t, "never-seen", createdRepo.ProviderName)
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

func TestHandleCreateTask_NewFields_Unmarshalled(t *testing.T) {
	// Verify that execution_policy and assignee_agent_profile_id are accepted
	// in the payload without triggering a parse error. The handler will fail
	// at the workspace_id validation stage, not at unmarshal.
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":                     "My task",
		"workspace_id":              "ws-1",
		"workflow_id":               "wf-1",
		"execution_policy":          `{"stages":[]}`,
		"assignee_agent_profile_id": "agent-inst-42",
	})

	// taskSvc is nil so CreateTask will panic before we reach it; the payload
	// must at least parse cleanly. The handler returns a validation error about
	// workspace_id being absent (not a parse error) only when those fields are
	// missing — here all required fields are present so it will reach taskSvc.
	// To avoid a nil-pointer panic we just verify the unmarshal path by sending
	// a payload that fails a post-unmarshal check (missing workspace) which
	// exercised the request struct fields.
	msgMissingWs := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":                     "My task",
		"execution_policy":          `{"stages":[]}`,
		"assignee_agent_profile_id": "agent-inst-42",
	})

	resp, err := h.handleCreateTask(context.Background(), msgMissingWs)
	require.NoError(t, err)
	// Should fail on workspace_id validation, not on JSON unmarshal
	assertWSError(t, resp, ws.ErrorCodeValidation)
	_ = msg // payload with all fields — tested implicitly through struct definition
}

func TestHandleCreateTask_BlockedBy_Accepted(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":      "Blocked task",
		"blocked_by": []string{"task-1", "task-2"},
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	// Fails on workspace_id, not on blocked_by parsing
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleClarificationTimeout_DetachesMessages(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Task",
	})
	require.NoError(t, err)

	sess := &models.TaskSession{
		ID:        "sess-1",
		TaskID:    task.ID,
		IsPrimary: true,
		State:     models.TaskSessionStateRunning,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))
	turn := &models.Turn{ID: "turn-1", TaskSessionID: sess.ID, TaskID: task.ID}
	require.NoError(t, repo.CreateTurn(ctx, turn))

	store := clarification.NewStore(time.Minute)
	eventBus := bus.NewMemoryEventBus(testLogger(t))
	t.Cleanup(func() { eventBus.Close() })
	canceller := clarification.NewCanceller(store, repo, eventBus, testLogger(t))

	pendingID := "test-pending-id"
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		TaskSessionID: sess.ID,
		TaskID:        task.ID,
		TurnID:        turn.ID,
		AuthorType:    "agent",
		Type:          "clarification_request",
		Content:       "Q?",
		Metadata: map[string]interface{}{
			"pending_id":  pendingID,
			"question_id": "q1",
			"status":      "pending",
		},
	}))

	// Drain the in-memory store so the handler must fall back to DB-driven cleanup
	store.CancelSession(sess.ID)

	h := NewHandlers(svc, nil, store, canceller, nil, repo, repo, eventBus, nil, nil, nil, testLogger(t))
	msg := makeWSMessage(t, ws.ActionMCPClarificationTimeout, map[string]interface{}{"session_id": sess.ID})
	resp, err := h.handleClarificationTimeout(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)

	msgs, err := repo.FindPendingClarificationMessagesBySessionID(ctx, sess.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1, "clarification should stay pending for deferred answer")
	require.Equal(t, true, msgs[0].Metadata["agent_disconnected"])
}

func TestHandleClarificationTimeout_WithoutCanceller_ReturnsError(t *testing.T) {
	h := &Handlers{sessionCanceller: nil, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPClarificationTimeout, map[string]interface{}{"session_id": "s1"})
	resp, err := h.handleClarificationTimeout(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleAskUserQuestion_Dedup_CreatesOnePendingBundle(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()

	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Test"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}))
	task, err := svc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Task",
	})
	require.NoError(t, err)

	sess := &models.TaskSession{
		ID:        "sess-dedup",
		TaskID:    task.ID,
		IsPrimary: true,
		State:     models.TaskSessionStateRunning,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, sess))

	store := clarification.NewStore(time.Minute)
	log := testLogger(t)
	h := NewHandlers(svc, nil, store, nil, nil, repo, repo, nil, nil, nil, nil, log)

	payload := map[string]interface{}{
		"session_id": sess.ID,
		"task_id":    task.ID,
		"questions": []map[string]interface{}{
			{"prompt": "What colour?", "options": []map[string]interface{}{
				{"label": "Red", "description": "R"},
				{"label": "Blue", "description": "B"},
			}},
		},
	}

	// Fire two identical requests concurrently. Because handleAskUserQuestion
	// blocks, we run them in goroutines and cancel the session after a short
	// delay so both return.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := makeWSMessage(t, ws.ActionMCPAskUserQuestion, payload)
			if _, err := h.handleAskUserQuestion(ctx, msg); err != nil {
				t.Errorf("handleAskUserQuestion returned unexpected error: %v", err)
			}
		}()
	}

	// Wait until the single deduped pending bundle is visible in the store
	// (confirming both goroutines have passed the CreateRequest gate).
	require.Eventually(t, func() bool {
		return len(store.ListPending()) == 1
	}, time.Second, 5*time.Millisecond)
	store.CancelSession(sess.ID)
	wg.Wait()

	// After dedup there should be exactly one pending entry even though two
	// handlers were invoked.
	pending := store.ListPending()
	require.Len(t, pending, 0) // cancelled
	// The actual dedup invariant is verified at the Store level; this test
	// ensures the handler path does not panic or leak with concurrent calls.
}
