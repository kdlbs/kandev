package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestQueueRecoveryDeniesUntrustedDispatch(t *testing.T) {
	dispatcher := ws.NewDispatcher()
	h := &Handlers{logger: testLogger(t)}
	h.RegisterHandlers(dispatcher)
	for _, action := range []string{"mcp.get_terminal_session_message_queue", "mcp.get_child_queue_inventory", "mcp.get_child_session_message_queue"} {
		request := makeWSMessage(t, action, map[string]interface{}{"task_id": "foreign", "session_id": "foreign-session", "caller_task_id": "forged"})
		response, err := dispatcher.Dispatch(context.Background(), request)
		require.NoError(t, err)
		assertWSError(t, response, ws.ErrorCodeForbidden)
	}
}

func queueRecoveryFixture(t *testing.T) (*Handlers, *sqliterepo.Repository, *models.Task, *models.Task, *models.TaskSession, context.Context) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	parent, child, session := seedChildTaskWithSession(t, svc, repo, models.TaskSessionStateCompleted)
	caller := &models.TaskSession{ID: "parent-session", TaskID: parent.ID, AgentProfileID: "profile", State: models.TaskSessionStateRunning, StartedAt: time.Now().UTC()}
	require.NoError(t, repo.CreateTaskSession(context.Background(), caller))
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
		row, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return messagequeue.QueueSessionIdentity{}, err
		}
		return messagequeue.QueueSessionIdentity{TaskID: row.TaskID, SessionID: row.ID, SessionIncarnationID: row.QueueIncarnationID}, nil
	})
	queue := messagequeue.NewService(queueRepo, 0, testLogger(t))
	_, err := queue.QueueMessage(context.Background(), session.ID, child.ID, "private instruction", "", messagequeue.QueuedByAgent, false, nil)
	require.NoError(t, err)
	h := &Handlers{taskSvc: svc, sessionRepo: repo, sessionLauncher: &fakeOrchestrator{queue: queue}, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{CallerTaskID: parent.ID, CallerSessionID: caller.ID, WorkspaceID: parent.WorkspaceID, Surface: mcpprofile.SurfaceKanbanTask})
	return h, repo, parent, child, session, ctx
}

func TestChildQueueRecoveryInventoryAndPinnedBody(t *testing.T) {
	h, _, _, child, session, ctx := queueRecoveryFixture(t)
	request := makeWSMessage(t, ws.ActionMCPGetChildQueueInventory, map[string]interface{}{"task_id": child.ID})
	response, err := h.handleChildQueueInventory(ctx, request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	require.NotContains(t, string(response.Payload), "private instruction")
	var inventory struct {
		Sessions []struct {
			Manifest messagequeue.QueueRecoveryManifest `json:"manifest"`
		} `json:"sessions"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &inventory))
	require.Len(t, inventory.Sessions, 1)
	digest := inventory.Sessions[0].Manifest.SnapshotSHA256
	pageRequest := makeWSMessage(t, ws.ActionMCPGetChildSessionMessageQueue, map[string]interface{}{"task_id": child.ID, "session_id": session.ID, "expected_snapshot_sha256": digest})
	page, err := h.handleQueueRecoveryPage(ctx, pageRequest)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, page.Type)
	require.Contains(t, string(page.Payload), "private instruction")
	_, err = h.sessionLauncher.GetMessageQueue().QueueMessage(context.Background(), session.ID, child.ID, "additional", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	page, err = h.handleQueueRecoveryPage(ctx, pageRequest)
	require.NoError(t, err)
	assertWSError(t, page, ws.ErrorCodeConflict)
	require.NotContains(t, string(page.Payload), "private instruction")
}

func TestChildQueueRecoveryDeniesMissingSelfAndWrongSession(t *testing.T) {
	h, _, parent, child, _, ctx := queueRecoveryFixture(t)
	for _, taskID := range []string{parent.ID, "missing"} {
		response, err := h.handleChildQueueInventory(ctx, makeWSMessage(t, ws.ActionMCPGetChildQueueInventory, map[string]interface{}{"task_id": taskID}))
		require.NoError(t, err)
		assertWSError(t, response, ws.ErrorCodeForbidden)
	}
	response, err := h.handleQueueRecoveryPage(ctx, makeWSMessage(t, ws.ActionMCPGetChildSessionMessageQueue, map[string]interface{}{"task_id": child.ID, "session_id": "parent-session"}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeForbidden)
}

func TestTerminalQueueRecoveryOwnPredecessorAndLiveConflict(t *testing.T) {
	h, repo, _, child, session, _ := queueRecoveryFixture(t)
	caller := &models.TaskSession{ID: "own-caller", TaskID: child.ID, AgentProfileID: "profile", State: models.TaskSessionStateRunning, StartedAt: time.Now().UTC()}
	require.NoError(t, repo.CreateTaskSession(context.Background(), caller))
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{CallerTaskID: child.ID, CallerSessionID: caller.ID, WorkspaceID: child.WorkspaceID, Surface: mcpprofile.SurfaceKanbanTask})
	request := makeWSMessage(t, ws.ActionMCPGetTerminalSessionMessageQueue, map[string]interface{}{"task_id": child.ID, "session_id": session.ID})
	for _, state := range []models.TaskSessionState{models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled} {
		require.NoError(t, repo.UpdateTaskSessionState(context.Background(), session.ID, state, ""))
		response, err := h.handleQueueRecoveryPage(ctx, request)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	}
	require.NoError(t, repo.UpdateTaskSessionState(context.Background(), session.ID, models.TaskSessionStateRunning, ""))
	response, err := h.handleQueueRecoveryPage(ctx, request)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeConflict)
}

func TestChildQueueRecoverySurfaceAndGraphDenials(t *testing.T) {
	h, repo, parent, child, session, ctx := queueRecoveryFixture(t)
	request := makeWSMessage(t, ws.ActionMCPGetChildSessionMessageQueue, map[string]interface{}{"task_id": child.ID, "session_id": session.ID})
	for _, surface := range []mcpprofile.Surface{mcpprofile.SurfaceAutomation, mcpprofile.SurfaceConfiguration, mcpprofile.SurfaceExternal, mcpprofile.SurfaceManagedConversation, mcpprofile.SurfaceCoordinator} {
		deniedCtx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{CallerTaskID: parent.ID, CallerSessionID: "parent-session", WorkspaceID: parent.WorkspaceID, Surface: surface})
		response, err := h.handleQueueRecoveryPage(deniedCtx, request)
		require.NoError(t, err)
		assertWSError(t, response, ws.ErrorCodeForbidden)
		require.NotContains(t, string(response.Payload), "private instruction")
	}
	changed, err := repo.DetachTask(context.Background(), child.ID)
	require.NoError(t, err)
	require.True(t, changed)
	response, err := h.handleQueueRecoveryPage(ctx, request)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeForbidden)
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "foreign", Name: "Foreign"}))
	require.NoError(t, repo.CreateWorkflow(context.Background(), &models.Workflow{ID: "foreign-workflow", WorkspaceID: "foreign", Name: "Foreign"}))
	foreign, err := h.taskSvc.CreateTask(context.Background(), &service.CreateTaskRequest{WorkspaceID: "foreign", WorkflowID: "foreign-workflow", Title: "Foreign"})
	require.NoError(t, err)
	response, err = h.handleQueueRecoveryPage(ctx, makeWSMessage(t, ws.ActionMCPGetChildSessionMessageQueue, map[string]interface{}{"task_id": foreign.Task.ID, "session_id": session.ID}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeForbidden)
}
