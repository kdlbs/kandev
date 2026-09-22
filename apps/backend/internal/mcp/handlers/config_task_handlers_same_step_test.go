package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type sameStepMoveSession struct {
	id      string
	state   models.TaskSessionState
	primary bool
}

type sameStepMoveFixture struct {
	svc          *service.Service
	repo         *sqliterepo.Repository
	workflowCtrl *workflowcontroller.Controller
	workflowRepo *workflowrepo.Repository
	db           *sqlx.DB
	eventBus     *bus.MemoryEventBus
	taskID       string
	workspaceID  string
	workflowID   string
	stepID       string
}

func newSameStepMoveFixture(t *testing.T, sessions ...sameStepMoveSession) *sameStepMoveFixture {
	t.Helper()
	svc, repo, workflowCtrl, workflowRepo, db, eventBus := newTestTaskServiceWithWorkflowDBAndEventBus(t)
	fixture := &sameStepMoveFixture{
		svc:          svc,
		repo:         repo,
		workflowCtrl: workflowCtrl,
		workflowRepo: workflowRepo,
		db:           db,
		eventBus:     eventBus,
		taskID:       "task-same-step",
		workspaceID:  "ws-same-step",
		workflowID:   "wf-same-step",
		stepID:       "step-work",
	}
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: fixture.workspaceID, Name: "Same step", OwnerID: "task-owner", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: fixture.workflowID, WorkspaceID: fixture.workspaceID, Name: "Board", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: fixture.stepID, WorkflowID: fixture.workflowID, Name: "Work", Position: 0,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: fixture.taskID, WorkspaceID: fixture.workspaceID, WorkflowID: fixture.workflowID,
		WorkflowStepID: fixture.stepID, Position: 7, Title: "Task", State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{"same_step_test": "preserve"}, CreatedAt: now, UpdatedAt: now,
	}))
	_, err := db.Exec(db.Rebind(`UPDATE tasks SET position = ? WHERE id = ?`), 7, fixture.taskID)
	require.NoError(t, err)
	for _, session := range sessions {
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: session.id, TaskID: fixture.taskID, State: session.state, IsPrimary: session.primary,
			Metadata: map[string]interface{}{"session_test": session.id}, StartedAt: now, UpdatedAt: now,
		}))
	}
	return fixture
}

func (f *sameStepMoveFixture) handler(t *testing.T, queue MessageQueuer) *Handlers {
	return &Handlers{
		taskSvc:      f.svc,
		workflowCtrl: f.workflowCtrl,
		messageQueue: queue,
		logger:       testLogger(t),
	}
}

func (f *sameStepMoveFixture) message(t *testing.T, fields map[string]interface{}) *ws.Message {
	t.Helper()
	payload := map[string]interface{}{
		"task_id": f.taskID, "workflow_id": f.workflowID, "workflow_step_id": f.stepID,
	}
	for key, value := range fields {
		payload[key] = value
	}
	return makeWSMessage(t, ws.ActionMCPMoveTask, payload)
}

func decodeMoveTaskResponse(t *testing.T, response *ws.Message) dto.MoveTaskResponse {
	t.Helper()
	require.Equal(t, ws.MessageTypeResponse, response.Type, "payload: %s", string(response.Payload))
	var result dto.MoveTaskResponse
	require.NoError(t, json.Unmarshal(response.Payload, &result))
	return result
}

func transitionCount(t *testing.T, db *sqlx.DB, taskID string) int {
	t.Helper()
	var count int
	require.NoError(t, db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`), taskID))
	return count
}

// @covers AC-TASKS-MCP-MOVE-RESULTS-001.1
func TestHandleMoveTask_SameStepApplied(t *testing.T) {
	cases := []struct {
		name     string
		sessions []sameStepMoveSession
	}{
		{name: "running primary", sessions: []sameStepMoveSession{{id: "session-running", state: models.TaskSessionStateRunning, primary: true}}},
		{name: "starting primary", sessions: []sameStepMoveSession{{id: "session-starting", state: models.TaskSessionStateStarting, primary: true}}},
		{name: "idle primary", sessions: []sameStepMoveSession{{id: "session-idle", state: models.TaskSessionStateIdle, primary: true}}},
		{name: "no session"},
		{name: "mixed sibling sessions", sessions: []sameStepMoveSession{
			{id: "session-primary-idle", state: models.TaskSessionStateIdle, primary: true},
			{id: "session-sibling-running", state: models.TaskSessionStateRunning},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSameStepMoveFixture(t, tc.sessions...)
			response, err := fixture.handler(t, nil).handleMoveTask(context.Background(), fixture.message(t, nil))
			require.NoError(t, err)
			result := decodeMoveTaskResponse(t, response)
			assert.Equal(t, moveDispositionApplied, result.Disposition)
			assert.Equal(t, fixture.taskID, result.Task.ID)
			assert.Equal(t, fixture.stepID, result.Task.WorkflowStepID)
			assert.Equal(t, 7, result.Task.Position)
			assert.Empty(t, result.MoveID)
			assert.Nil(t, result.EntryOptions)
			assert.Empty(t, result.WorkflowEntryIdentity)
		})
	}
}

// @covers AC-TASKS-MCP-MOVE-RESULTS-001.2
func TestHandleMoveTask_SameStepReturnsStoredTask(t *testing.T) {
	fixture := newSameStepMoveFixture(t)
	for _, tc := range []struct {
		name   string
		fields map[string]interface{}
	}{
		{name: "position omitted"},
		{name: "requested position ignored", fields: map[string]interface{}{"position": 99}},
		{name: "empty entry options normalized", fields: map[string]interface{}{"entry_options": map[string]interface{}{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := fixture.handler(t, nil).handleMoveTask(context.Background(), fixture.message(t, tc.fields))
			require.NoError(t, err)
			result := decodeMoveTaskResponse(t, response)
			assert.Equal(t, 7, result.Task.Position)
			assert.Equal(t, map[string]interface{}{"same_step_test": "preserve"}, result.Task.Metadata)
			assert.Empty(t, result.MoveID)
			assert.Nil(t, result.EntryOptions)
		})
	}
}

// @covers AC-TASKS-MCP-MOVE-RESULTS-001.3
func TestHandleMoveTask_SameStepPreservesEffects(t *testing.T) {
	fixture := newSameStepMoveFixture(t, sameStepMoveSession{
		id: "session-running", state: models.TaskSessionStateRunning, primary: true,
	})
	ctx := context.Background()
	queue := &pendingMoveRecordingQueuer{
		recordingMessageQueuer: recordingMessageQueuer{calls: []messagequeue.QueuedMessage{{
			ID: "existing-prompt", SessionID: "other-session", TaskID: "other-task", Content: "keep this prompt",
		}}},
		pendingSessionID: "other-session",
		pendingMoves: []messagequeue.PendingMove{{
			MoveID: "existing-move", TaskID: "other-task", WorkflowID: "other-workflow", WorkflowStepID: "other-step", Position: 4,
		}},
	}
	beforeMessages := append([]messagequeue.QueuedMessage(nil), queue.calls...)
	beforeMoves := append([]messagequeue.PendingMove(nil), queue.pendingMoves...)
	beforeTask, err := fixture.svc.GetTask(ctx, fixture.taskID)
	require.NoError(t, err)
	beforeSession, err := fixture.repo.GetTaskSession(ctx, "session-running")
	require.NoError(t, err)
	beforeTransitions := transitionCount(t, fixture.db, fixture.taskID)
	eventCount := 0
	subscription, err := fixture.eventBus.Subscribe(">", func(context.Context, *bus.Event) error {
		eventCount++
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = subscription.Unsubscribe() })

	handler := fixture.handler(t, queue)
	for range 47 {
		response, err := handler.handleMoveTask(ctx, fixture.message(t, map[string]interface{}{
			"entry_options": map[string]interface{}{},
		}))
		require.NoError(t, err)
		result := decodeMoveTaskResponse(t, response)
		assert.Equal(t, moveDispositionApplied, result.Disposition)
	}

	afterTask, err := fixture.svc.GetTask(ctx, fixture.taskID)
	require.NoError(t, err)
	afterSession, err := fixture.repo.GetTaskSession(ctx, "session-running")
	require.NoError(t, err)
	assert.Equal(t, beforeTask, afterTask)
	assert.Equal(t, beforeSession, afterSession)
	assert.Equal(t, beforeMessages, queue.calls)
	assert.Equal(t, beforeMoves, queue.pendingMoves)
	assert.Equal(t, beforeTransitions, transitionCount(t, fixture.db, fixture.taskID))
	assert.Zero(t, eventCount)
}

// @covers AC-TASKS-MCP-MOVE-RESULTS-001.3
func TestHandleMoveTask_SameStepPersistentQueue(t *testing.T) {
	fixture := newSameStepMoveFixture(t, sameStepMoveSession{
		id: "session-running", state: models.TaskSessionStateRunning, primary: true,
	})
	ctx := context.Background()
	queueRepo, err := messagequeue.NewSQLiteRepository(fixture.db, fixture.db)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, 0, testLogger(t))
	now := time.Now().UTC()
	require.NoError(t, fixture.repo.CreateTask(ctx, &models.Task{
		ID: "task-unrelated", WorkspaceID: fixture.workspaceID, WorkflowID: fixture.workflowID,
		WorkflowStepID: fixture.stepID, Position: 2, Title: "Unrelated task", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-unrelated", TaskID: "task-unrelated", IsPrimary: true,
		State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, queueRepo.SetPendingMove(ctx, "session-unrelated", &messagequeue.PendingMove{
		MoveID: "unrelated-move", TaskID: "task-unrelated", WorkflowID: fixture.workflowID,
		WorkflowStepID: "step-next", Position: 3, Actor: "agent", SenderSessionID: "sender-session",
		EntryOptions: &workflowmove.EntryOptions{Instructions: "retain this override"},
	}))
	beforeMoves, err := queueRepo.ListPendingMoves(ctx)
	require.NoError(t, err)
	require.Len(t, beforeMoves, 1)
	beforeTransitions := transitionCount(t, fixture.db, fixture.taskID)

	dispatcher := ws.NewDispatcher()
	fixture.handler(t, queue).RegisterHandlers(dispatcher)
	for range 47 {
		response, err := dispatcher.Dispatch(ctx, fixture.message(t, nil))
		require.NoError(t, err)
		result := decodeMoveTaskResponse(t, response)
		assert.Equal(t, moveDispositionApplied, result.Disposition)
	}
	afterMoves, err := queueRepo.ListPendingMoves(ctx)
	require.NoError(t, err)
	assert.Equal(t, beforeMoves, afterMoves)
	currentMove, err := queueRepo.GetPendingMove(ctx, "session-running")
	require.NoError(t, err)
	assert.Nil(t, currentMove)
	task, err := fixture.svc.GetTask(ctx, fixture.taskID)
	require.NoError(t, err)
	assert.Equal(t, fixture.stepID, task.WorkflowStepID)
	assert.Equal(t, 7, task.Position)
	assert.Equal(t, beforeTransitions, transitionCount(t, fixture.db, fixture.taskID))
}

// @covers AC-TASKS-MCP-MOVE-RESULTS-001.4
func TestHandleMoveTask_SameStepValidation(t *testing.T) {
	type testCase struct {
		name          string
		fields        map[string]interface{}
		prepare       func(*testing.T, *sameStepMoveFixture) context.Context
		withoutTask   bool
		withoutTarget bool
		wantCode      string
	}
	cases := []testCase{
		{name: "task read failure", fields: map[string]interface{}{"task_id": "missing-task"}, wantCode: ws.ErrorCodeInternalError},
		{name: "task write denied", prepare: func(t *testing.T, f *sameStepMoveFixture) context.Context {
			require.NoError(t, f.repo.UpsertWorkspaceMember(context.Background(), &models.WorkspaceMember{
				WorkspaceID: f.workspaceID, UserID: "viewer", Role: string(authz.WorkspaceRoleViewer),
			}))
			return authn.WithIdentity(context.Background(), authn.Identity{UserID: "viewer", Role: authn.RoleMember})
		}, wantCode: ws.ErrorCodeForbidden},
		{name: "task outside caller scope", prepare: func(*testing.T, *sameStepMoveFixture) context.Context {
			return authn.WithIdentity(context.Background(), authn.Identity{UserID: "outsider", Role: authn.RoleMember})
		}, wantCode: ws.ErrorCodeInternalError},
		{name: "archived task", prepare: func(t *testing.T, f *sameStepMoveFixture) context.Context {
			require.NoError(t, f.repo.ArchiveTask(context.Background(), f.taskID))
			return context.Background()
		}, wantCode: ws.ErrorCodeConflict},
		{name: "missing step", prepare: func(t *testing.T, f *sameStepMoveFixture) context.Context {
			require.NoError(t, f.workflowRepo.DeleteStep(context.Background(), f.stepID))
			return context.Background()
		}, wantCode: ws.ErrorCodeValidation},
		{name: "step belongs to another workflow", prepare: func(t *testing.T, f *sameStepMoveFixture) context.Context {
			ctx := context.Background()
			require.NoError(t, f.repo.CreateWorkflow(ctx, &models.Workflow{
				ID: "wf-other", WorkspaceID: f.workspaceID, Name: "Other", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}))
			require.NoError(t, f.workflowRepo.DeleteStep(ctx, f.stepID))
			require.NoError(t, f.workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
				ID: f.stepID, WorkflowID: "wf-other", Name: "Foreign step", Position: 0,
				CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}))
			return ctx
		}, wantCode: ws.ErrorCodeValidation},
		{name: "missing workflow", fields: map[string]interface{}{"workflow_id": "missing-workflow"}, prepare: func(t *testing.T, f *sameStepMoveFixture) context.Context {
			_, err := f.db.Exec(f.db.Rebind(`UPDATE tasks SET workflow_id = ? WHERE id = ?`), "missing-workflow", f.taskID)
			require.NoError(t, err)
			return context.Background()
		}, wantCode: ws.ErrorCodeValidation},
		{name: "workflow belongs to another workspace", prepare: func(t *testing.T, f *sameStepMoveFixture) context.Context {
			ctx := context.Background()
			require.NoError(t, f.repo.CreateWorkspace(ctx, &models.Workspace{
				ID: "ws-other", Name: "Other", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}))
			_, err := f.db.Exec(f.db.Rebind(`UPDATE workflows SET workspace_id = ? WHERE id = ?`), "ws-other", f.workflowID)
			require.NoError(t, err)
			return ctx
		}, wantCode: ws.ErrorCodeValidation},
		{name: "task service missing", withoutTask: true, wantCode: ws.ErrorCodeInternalError},
		{name: "workflow validation missing", withoutTarget: true, wantCode: ws.ErrorCodeInternalError},
		{name: "instructions require a step change", fields: map[string]interface{}{"entry_options": map[string]interface{}{"instructions": "handoff"}}, wantCode: ws.ErrorCodeValidation},
		{name: "reset requires a step change", fields: map[string]interface{}{"entry_options": map[string]interface{}{"reset_context": true}}, wantCode: ws.ErrorCodeValidation},
		{name: "skip prompt requires a step change", fields: map[string]interface{}{"entry_options": map[string]interface{}{"skip_step_prompt": true}}, wantCode: ws.ErrorCodeValidation},
		{name: "legacy prompt requires a step change", fields: map[string]interface{}{"prompt": "handoff"}, wantCode: ws.ErrorCodeValidation},
		{name: "conflicting prompt aliases", fields: map[string]interface{}{
			"prompt": "legacy", "entry_options": map[string]interface{}{"instructions": "nested"},
		}, wantCode: ws.ErrorCodeValidation},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSameStepMoveFixture(t)
			ctx := context.Background()
			if tc.prepare != nil {
				ctx = tc.prepare(t, fixture)
			}
			handler := fixture.handler(t, nil)
			if tc.withoutTask {
				handler.taskSvc = nil
			}
			if tc.withoutTarget {
				handler.workflowCtrl = nil
			}
			response, err := handler.handleMoveTask(ctx, fixture.message(t, tc.fields))
			require.NoError(t, err)
			assertWSError(t, response, tc.wantCode)
		})
	}
}
