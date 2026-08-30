package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type blockingAgentProfileRecentUseRecorder struct {
	started chan struct{}
	release chan struct{}
}

func (r *blockingAgentProfileRecentUseRecorder) RecordAgentProfileRecentUse(
	context.Context,
	usermodels.AgentProfileRecentUseContext,
	string,
) (*usermodels.AgentProfileRecentUse, error) {
	close(r.started)
	<-r.release
	return nil, nil
}

type successfulWSLaunchOrchestrator struct{}

func (successfulWSLaunchOrchestrator) LaunchSession(context.Context, *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	return &orchestrator.LaunchSessionResponse{
		Success:        true,
		SessionID:      "session-1",
		AgentProfileID: "profile-1",
	}, nil
}

func (successfulWSLaunchOrchestrator) EnsureSession(context.Context, string, ...orchestrator.EnsureSessionOptions) (*orchestrator.EnsureSessionResponse, error) {
	return nil, nil
}

// wsTaskRepo owns everything as "user-b" and records every write, so a request
// made as "user-a" both fails and leaves no trace.
type wsTaskRepo struct {
	mockRepository

	updated       []*models.Task
	deleted       []string
	archived      []string
	stateUpdates  []string
	sessionsCalls []string
	created       []*models.Task
}

func (r *wsTaskRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	if id != "task-b" {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return &models.Task{ID: "task-b", WorkspaceID: "ws-b", Title: "Victim", State: v1.TaskStateTODO}, nil
}

func (r *wsTaskRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	if id != "ws-b" {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return &models.Workspace{ID: "ws-b", Name: "Theirs", OwnerID: "user-b"}, nil
}

func (r *wsTaskRepo) UpdateTask(_ context.Context, task *models.Task) error {
	r.updated = append(r.updated, task)
	return nil
}

func (r *wsTaskRepo) DeleteTask(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *wsTaskRepo) ArchiveTask(_ context.Context, id string) error {
	r.archived = append(r.archived, id)
	return nil
}

func (r *wsTaskRepo) UpdateTaskState(_ context.Context, id string, _ v1.TaskState) error {
	r.stateUpdates = append(r.stateUpdates, id)
	return nil
}

func (r *wsTaskRepo) CreateTask(_ context.Context, task *models.Task) error {
	r.created = append(r.created, task)
	return nil
}

func (r *wsTaskRepo) ListTaskSessions(_ context.Context, taskID string) ([]*models.TaskSession, error) {
	r.sessionsCalls = append(r.sessionsCalls, taskID)
	return []*models.TaskSession{{ID: "sess-b", TaskID: taskID}}, nil
}

func (r *wsTaskRepo) GetWorkflow(_ context.Context, id string) (*models.Workflow, error) {
	if id != "wf-b" {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	return &models.Workflow{ID: "wf-b", WorkspaceID: "ws-b", Name: "Board"}, nil
}

func (r *wsTaskRepo) ListTasks(_ context.Context, workflowID string) ([]*models.Task, error) {
	if workflowID != "wf-b" {
		return nil, nil
	}
	return []*models.Task{
		{ID: "task-b", WorkspaceID: "ws-b", Title: "Mine"},
		// A row that leaked in from another workspace: ListTasks filters it out.
		{ID: "task-other", WorkspaceID: "ws-other", Title: "Theirs"},
	}, nil
}

func newWSTaskHandlers(t *testing.T, repo *wsTaskRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

// asUser returns a context carrying a scoped identity. An identity-free
// context is treated as an internal caller and skips scoping entirely, so
// every authorization test must supply one.
func asUser(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

// TestWSTaskReadsDenyForeignTask covers the read side: neither the task itself
// nor its session list may be served to a user who does not own the workspace.
// Each case also runs as the owner, so a denial that came from a broken
// fixture rather than from scoping fails here instead of passing silently.
func TestWSTaskReadsDenyForeignTask(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	foreign, err := h.wsGetTask(asUser("user-a"),
		wsWorkflowRequest(t, ws.ActionTaskGet, map[string]any{"id": "task-b"}))
	require.NoError(t, err)
	require.Equal(t, string(ws.ErrorCodeNotFound), wsWorkflowError(t, foreign).Code)
	require.NotContains(t, string(foreign.Payload), "Victim", "the foreign task must not be serialized")

	owner, err := h.wsGetTask(asUser("user-b"),
		wsWorkflowRequest(t, ws.ActionTaskGet, map[string]any{"id": "task-b"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, owner.Type, "the owner must get past the check")

	foreignSessions, err := h.wsListTaskSessions(asUser("user-a"),
		wsWorkflowRequest(t, ws.ActionTaskSessionList, map[string]any{"task_id": "task-b"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, foreignSessions.Type)
	require.NotContains(t, string(foreignSessions.Payload), "sess-b", "session IDs must not leak")

	ownerSessions, err := h.wsListTaskSessions(asUser("user-b"),
		wsWorkflowRequest(t, ws.ActionTaskSessionList, map[string]any{"task_id": "task-b"}))
	require.NoError(t, err)
	var list dto.ListTaskSessionSummariesResponse
	wsWorkflowResponse(t, ownerSessions, &list)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "sess-b", list.Sessions[0].ID)
}

// TestWSTaskMutationsDenyForeignTask is the mutation half. The recorded-write
// assertions matter more than the error codes: a guard that refused the
// response after committing the write would still be a data-loss bug.
//
// task.state and task.move are deliberately absent — their service methods
// take no authorization step at all, and the two are reported separately
// rather than pinned here as if the current behavior were intended.
//
// Note the error code: these mutations answer InternalError where the read
// surface above answers NotFound for the very same denial, so a client cannot
// tell "refused" from "server broke". The refusal is what matters and is what
// this test guards; the code mismatch is asserted as-is and reported
// separately rather than endorsed here.
func TestWSTaskMutationsDenyForeignTask(t *testing.T) {
	for name, tc := range map[string]struct {
		call    func(*TaskHandlers, context.Context) (*ws.Message, error)
		writes  func(*wsTaskRepo) int
		wantMsg string
	}{
		"update": {
			call: func(h *TaskHandlers, ctx context.Context) (*ws.Message, error) {
				return h.wsUpdateTask(ctx, wsWorkflowRequest(t, ws.ActionTaskUpdate,
					map[string]any{"id": "task-b", "title": "Hijacked"}))
			},
			writes:  func(r *wsTaskRepo) int { return len(r.updated) },
			wantMsg: "Failed to update task",
		},
		"delete": {
			call: func(h *TaskHandlers, ctx context.Context) (*ws.Message, error) {
				return h.wsDeleteTask(ctx, wsWorkflowRequest(t, ws.ActionTaskDelete,
					map[string]any{"id": "task-b"}))
			},
			writes:  func(r *wsTaskRepo) int { return len(r.deleted) },
			wantMsg: "failed to delete task",
		},
		"archive": {
			call: func(h *TaskHandlers, ctx context.Context) (*ws.Message, error) {
				return h.wsArchiveTask(ctx, wsWorkflowRequest(t, ws.ActionTaskArchive,
					map[string]any{"id": "task-b"}))
			},
			writes:  func(r *wsTaskRepo) int { return len(r.archived) },
			wantMsg: "failed to archive task",
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &wsTaskRepo{}
			h := newWSTaskHandlers(t, repo)

			resp, err := tc.call(h, asUser("user-a"))

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
			require.Equal(t, tc.wantMsg, payload.Message)
			require.Zero(t, tc.writes(repo), "a denied mutation must not reach the repository")
		})
	}
}

// TestWSCreateTaskDeniesForeignWorkspace stops a caller from planting a task in
// somebody else's workspace.
func TestWSCreateTaskDeniesForeignWorkspace(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	resp, err := h.wsCreateTask(asUser("user-a"), wsWorkflowRequest(t, ws.ActionTaskCreate, map[string]any{
		"workspace_id": "ws-b", "workflow_id": "wf-b", "title": "Planted",
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	require.Empty(t, repo.created, "a denied create must not insert a task")
}

func TestWSCreateTaskValidatesRequiredFields(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	for name, tc := range map[string]struct {
		payload map[string]any
		wantMsg string
	}{
		"missing workspace": {
			payload: map[string]any{"workflow_id": "wf-1", "title": "T"},
			wantMsg: "workspace_id is required",
		},
		"missing workflow": {
			payload: map[string]any{"workspace_id": "ws-b", "title": "T"},
			wantMsg: "workflow_id is required",
		},
		"missing title": {
			payload: map[string]any{"workspace_id": "ws-b", "workflow_id": "wf-1"},
			wantMsg: "title is required",
		},
		"start agent without profile": {
			payload: map[string]any{
				"workspace_id": "ws-b", "workflow_id": "wf-1", "title": "T", "start_agent": true,
			},
			wantMsg: "agent_profile_id is required to start agent",
		},
		"repository without any identifier": {
			payload: map[string]any{
				"workspace_id": "ws-b", "workflow_id": "wf-1", "title": "T",
				"repositories": []map[string]any{{"base_branch": "main"}},
			},
			wantMsg: "repository_id, local_path, or remote_url is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := h.wsCreateTask(context.Background(),
				wsWorkflowRequest(t, ws.ActionTaskCreate, tc.payload))

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
			require.Equal(t, tc.wantMsg, payload.Message)
		})
	}
	require.Empty(t, repo.created, "no validation failure may create a task")
}

func TestWSTaskHandlersRejectMalformedPayloads(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	for name, call := range map[string]func(context.Context, *ws.Message) (*ws.Message, error){
		"task.list":           h.wsListTasks,
		"task.create":         h.wsCreateTask,
		"task.get":            h.wsGetTask,
		"task.update":         h.wsUpdateTask,
		"task.delete":         h.wsDeleteTask,
		"task.archive":        h.wsArchiveTask,
		"task.move":           h.wsMoveTask,
		"task.state":          h.wsUpdateTaskState,
		"task.session.list":   h.wsListTaskSessions,
		"task.repository.upd": h.wsUpdateTaskRepository,
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := call(context.Background(), wsMalformedRequest(name))

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeBadRequest), payload.Code)
			require.Contains(t, payload.Message, "Invalid payload")
		})
	}
}

func TestWSTaskHandlersValidateRequiredIdentifiers(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	for name, tc := range map[string]struct {
		call    func(context.Context, *ws.Message) (*ws.Message, error)
		action  string
		payload map[string]any
		wantMsg string
	}{
		"list tasks needs workflow": {
			call: h.wsListTasks, action: ws.ActionTaskList,
			payload: map[string]any{}, wantMsg: "workflow_id is required",
		},
		"list sessions needs task": {
			call: h.wsListTaskSessions, action: ws.ActionTaskSessionList,
			payload: map[string]any{}, wantMsg: "task_id is required",
		},
		"get needs id": {
			call: h.wsGetTask, action: ws.ActionTaskGet,
			payload: map[string]any{}, wantMsg: "id is required",
		},
		"update needs id": {
			call: h.wsUpdateTask, action: ws.ActionTaskUpdate,
			payload: map[string]any{"title": "T"}, wantMsg: "id is required",
		},
		"delete needs id": {
			call: h.wsDeleteTask, action: ws.ActionTaskDelete,
			payload: map[string]any{}, wantMsg: "id is required",
		},
		"archive needs id": {
			call: h.wsArchiveTask, action: ws.ActionTaskArchive,
			payload: map[string]any{}, wantMsg: "id is required",
		},
		"move needs id": {
			call: h.wsMoveTask, action: ws.ActionTaskMove,
			payload: map[string]any{"workflow_id": "wf-1", "workflow_step_id": "st-1"},
			wantMsg: "id is required",
		},
		"move needs workflow": {
			call: h.wsMoveTask, action: ws.ActionTaskMove,
			payload: map[string]any{"id": "task-b", "workflow_step_id": "st-1"},
			wantMsg: "workflow_id is required",
		},
		"move needs step": {
			call: h.wsMoveTask, action: ws.ActionTaskMove,
			payload: map[string]any{"id": "task-b", "workflow_id": "wf-1"},
			wantMsg: "workflow_step_id is required",
		},
		"state needs id": {
			call: h.wsUpdateTaskState, action: ws.ActionTaskState,
			payload: map[string]any{"state": "TODO"}, wantMsg: "id is required",
		},
		"state needs state": {
			call: h.wsUpdateTaskState, action: ws.ActionTaskState,
			payload: map[string]any{"id": "task-b"}, wantMsg: "state is required",
		},
		"repository update needs task": {
			call: h.wsUpdateTaskRepository, action: ws.ActionTaskRepoUpdate,
			payload: map[string]any{"task_repository_id": "tr-1"}, wantMsg: "task_id is required",
		},
		"repository update needs row": {
			call: h.wsUpdateTaskRepository, action: ws.ActionTaskRepoUpdate,
			payload: map[string]any{"task_id": "task-b"}, wantMsg: "task_repository_id is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := tc.call(context.Background(), wsWorkflowRequest(t, tc.action, tc.payload))

			require.NoError(t, err)
			payload := wsWorkflowError(t, resp)
			require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
			require.Equal(t, tc.wantMsg, payload.Message)
		})
	}

	require.Empty(t, repo.updated)
	require.Empty(t, repo.deleted)
	require.Empty(t, repo.archived)
	require.Empty(t, repo.stateUpdates)
	require.Empty(t, repo.sessionsCalls, "a request rejected on validation must not query sessions")
}

// TestWSUpdateTaskReportsValidationErrorsVerbatim proves the two-way split in
// wsUpdateTask's error mapping: a validation failure keeps its message so the
// UI can show it, while anything else is flattened to a generic message.
func TestWSUpdateTaskReportsValidationErrorsVerbatim(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)
	overlong := ""
	for range 80 {
		overlong += "x"
	}

	resp, err := h.wsUpdateTask(context.Background(),
		wsWorkflowRequest(t, ws.ActionTaskUpdate, map[string]any{"id": "task-b", "title": overlong}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeValidation), payload.Code)
	require.Contains(t, payload.Message, "too long")
	require.Empty(t, repo.updated)
}

func TestWSListTasksReturnsOnlyTheWorkflowsWorkspaceTasks(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	resp, err := h.wsListTasks(context.Background(),
		wsWorkflowRequest(t, ws.ActionTaskList, map[string]any{"workflow_id": "wf-b"}))

	require.NoError(t, err)
	var list dto.ListTasksResponse
	wsWorkflowResponse(t, resp, &list)
	require.Equal(t, 1, list.Total, "a task from another workspace must be filtered out")
	require.Len(t, list.Tasks, 1)
	require.Equal(t, "task-b", list.Tasks[0].ID)
}

func TestWSListTasksSurfacesUnknownWorkflow(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	resp, err := h.wsListTasks(context.Background(),
		wsWorkflowRequest(t, ws.ActionTaskList, map[string]any{"workflow_id": "wf-missing"}))

	require.NoError(t, err)
	payload := wsWorkflowError(t, resp)
	require.Equal(t, string(ws.ErrorCodeInternalError), payload.Code)
	require.Equal(t, "Failed to list tasks", payload.Message)
}

// TestWSCreateTask_PlanModeStartAgentPreservesPlanMode pins the task.create
// contract on the WS handler. plan_mode describes the execution prompt and
// must survive task persistence and the deferred launch intent.
func TestWSCreateTask_PlanModeStartAgentPreservesPlanMode(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)

	resp, err := h.wsCreateTask(context.Background(), wsWorkflowRequest(t, ws.ActionTaskCreate, map[string]any{
		"workspace_id":     "ws-b",
		"workflow_id":      "wf-b",
		"title":            "Plan then boot",
		"agent_profile_id": "profile-1",
		"start_agent":      true,
		"plan_mode":        true,
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "body: %s", resp.Payload)
	require.Len(t, repo.created, 1, "task.create must persist the task")
	captured := repo.created[0]
	require.NotNil(t, captured.Metadata, "task metadata must hold the deferred intent")

	deferredRaw, ok := captured.Metadata[models.MetaKeyDeferredLaunch]
	require.True(t, ok, "start_agent=true must persist a deferred_launch intent")
	deferred, ok := deferredRaw.(map[string]interface{})
	require.True(t, ok, "deferred_launch intent must be a map[string]interface{}")
	pmFlag, present := deferred["plan_mode"]
	require.True(t, present, "the deferred intent must carry the plan_mode key (to assert the !StartAgent guard)")
	assert.Equal(t, true, pmFlag,
		"plan_mode=true must remain true in the deferred launch intent")
}

func TestWSCreateTaskDoesNotWaitForRecentUsePersistence(t *testing.T) {
	repo := &wsTaskRepo{}
	h := newWSTaskHandlers(t, repo)
	recorder := &blockingAgentProfileRecentUseRecorder{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h.orchestrator = successfulWSLaunchOrchestrator{}
	h.agentProfileRecentUseRecorder = recorder
	defer close(recorder.release)

	result := make(chan struct {
		response *ws.Message
		err      error
	}, 1)
	go func() {
		response, err := h.wsCreateTask(asUser("user-b"), wsWorkflowRequest(t, ws.ActionTaskCreate, map[string]any{
			"workspace_id":     "ws-b",
			"workflow_id":      "wf-b",
			"title":            "Async recency",
			"description":      "Start without waiting for preference persistence",
			"start_agent":      true,
			"agent_profile_id": "profile-1",
		}))
		result <- struct {
			response *ws.Message
			err      error
		}{response: response, err: err}
	}()

	select {
	case <-recorder.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recent-use recorder was not invoked")
	}

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.NotNil(t, got.response)
		require.Equal(t, ws.MessageTypeResponse, got.response.Type)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("successful task.create waited for best-effort recent-use persistence")
	}
}
