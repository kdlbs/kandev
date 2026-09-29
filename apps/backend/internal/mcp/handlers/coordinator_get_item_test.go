package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// newGetItemTestHandlers builds a *Handlers wired the way production does
// when features.coordinator is on: a real taskSvc (for the stall kind's task
// lookup) sharing the same workspace as a real *coordinator.Service, plus a
// direct handle on the coordinator store for seeding proposals and stalls.
func newGetItemTestHandlers(t *testing.T) (*Handlers, *coordinator.Store, *coordinator.Coordinator, *sqliterepo.Repository) {
	t.Helper()
	taskSvc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	workspaceID := workspaces[0].ID

	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	validator := coordinator.NewValidator(nil, nil)
	svc := coordinator.NewService(store, validator, taskSvc, testLogger(t))

	c := &coordinator.Coordinator{
		WorkspaceID: workspaceID, Name: "Ops", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
	}
	require.NoError(t, store.CreateCoordinator(ctx, c))

	h := &Handlers{taskSvc: taskSvc, logger: testLogger(t).WithFields()}
	h.SetCoordinatorService(svc)
	return h, store, c, repo
}

func getItemPrincipalContext(workspaceID, coordinatorID string) context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		CoordinatorID:   coordinatorID,
		WorkspaceID:     workspaceID,
		CallerTaskID:    "coordinator-conversation-task",
		CallerSessionID: "coordinator-session",
		Surface:         mcpprofile.SurfaceCoordinator,
	})
}

func getItemPayload(kind, id string) map[string]interface{} {
	return map[string]interface{}{"kind": kind, "id": id}
}

// TestHandleGetCoordinatorItem_Proposal covers the proposal kind's test
// table (copilot-tools.md#item-read): own proposal (any status), foreign
// coordinator's proposal, another workspace's proposal, and missing.
func TestHandleGetCoordinatorItem_Proposal(t *testing.T) {
	h, store, c, _ := newGetItemTestHandlers(t)
	ctx := context.Background()

	own := &coordinator.Proposal{WorkspaceID: c.WorkspaceID, CoordinatorID: c.ID, Spec: coordinator.ProposalSpec{Title: "Own"}}
	require.NoError(t, store.InsertProposal(ctx, own))
	// Settle it, so "any status" is exercised too.
	rejected, err := store.RejectProposal(ctx, own.ID, "no longer needed", "user-1", time.Now().UTC())
	require.NoError(t, err)
	require.True(t, rejected)

	foreignCoordinator := &coordinator.Coordinator{
		WorkspaceID: c.WorkspaceID, Name: "Other", AgentProfileID: "agent-2", ExecutorProfileID: "executor-2",
	}
	require.NoError(t, store.CreateCoordinator(ctx, foreignCoordinator))
	foreignOwned := &coordinator.Proposal{
		WorkspaceID: c.WorkspaceID, CoordinatorID: foreignCoordinator.ID, Spec: coordinator.ProposalSpec{Title: "Foreign"},
	}
	require.NoError(t, store.InsertProposal(ctx, foreignOwned))

	t.Run("own proposal, settled status", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", own.ID))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "expected success, got: %s", resp.Payload)

		var payload struct {
			Kind     string                  `json:"kind"`
			Proposal coordinator.ProposalDTO `json:"proposal"`
		}
		require.NoError(t, json.Unmarshal(resp.Payload, &payload))
		require.Equal(t, "proposal", payload.Kind)
		require.Equal(t, own.ID, payload.Proposal.ID)
		require.Equal(t, coordinator.ProposalStatusRejected, payload.Proposal.Status)
	})

	t.Run("foreign coordinator's proposal is not found", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", foreignOwned.ID))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})

	t.Run("another workspace's proposal is not found", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", own.ID))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext("ws-other", c.ID), msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})

	t.Run("missing proposal is not found", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", "does-not-exist"))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})
}

// TestHandleGetCoordinatorItem_Stall covers the stall kind's test table:
// own-workspace stalled task, own-workspace task with no stall row, foreign
// task, and missing task.
func TestHandleGetCoordinatorItem_Stall(t *testing.T) {
	h, store, c, repo := newGetItemTestHandlers(t)
	ctx := context.Background()
	now := time.Now().UTC()

	stalledTask := &taskmodels.Task{
		ID: "task-stalled", WorkspaceID: c.WorkspaceID, Title: "Stalled",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTask(ctx, stalledTask))
	require.NoError(t, repo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-no-stall", WorkspaceID: c.WorkspaceID, Title: "No stall",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	_, err := store.UpsertStall(ctx, &coordinator.Stall{
		TaskID: stalledTask.ID, WorkspaceID: c.WorkspaceID, StalledForMs: 60_000, LastEventAt: now, DetectedAt: now,
	})
	require.NoError(t, err)

	foreignWorkspace := &taskmodels.Workspace{ID: "ws-foreign-get-item", Name: "Foreign", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkspace(ctx, foreignWorkspace))
	foreignTask := &taskmodels.Task{
		ID: "task-foreign", WorkspaceID: foreignWorkspace.ID, Title: "Foreign",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTask(ctx, foreignTask))

	t.Run("own-workspace stalled task", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("stall", stalledTask.ID))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "expected success, got: %s", resp.Payload)

		var payload struct {
			Kind  string               `json:"kind"`
			Stall coordinatorStallItem `json:"stall"`
		}
		require.NoError(t, json.Unmarshal(resp.Payload, &payload))
		require.Equal(t, "stall", payload.Kind)
		require.Equal(t, stalledTask.ID, payload.Stall.TaskID)
		require.Equal(t, c.WorkspaceID, payload.Stall.WorkspaceID)
		require.Equal(t, int64(60_000), payload.Stall.StalledForMs)
	})

	t.Run("own-workspace task with no stall row", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("stall", "task-no-stall"))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
		var errPayload ws.ErrorPayload
		require.NoError(t, json.Unmarshal(resp.Payload, &errPayload))
		require.Equal(t, "no stall record for this task", errPayload.Message)
	})

	t.Run("foreign task is not found", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("stall", foreignTask.ID))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})

	t.Run("missing task is not found", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("stall", "does-not-exist"))
		resp, err := h.handleGetCoordinatorItem(getItemPrincipalContext(c.WorkspaceID, c.ID), msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	})
}

// TestHandleGetCoordinatorItem_Validation covers kind/id validation ordering
// and content: "task" kind, an unknown kind, and an empty id.
func TestHandleGetCoordinatorItem_Validation(t *testing.T) {
	h, _, c, _ := newGetItemTestHandlers(t)
	ctx := getItemPrincipalContext(c.WorkspaceID, c.ID)

	t.Run(`"task" kind is refused`, func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("task", "task-1"))
		resp, err := h.handleGetCoordinatorItem(ctx, msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeBadRequest)
	})

	t.Run("unknown kind is refused", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("bogus", "task-1"))
		resp, err := h.handleGetCoordinatorItem(ctx, msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeBadRequest)
	})

	t.Run("empty id is refused", func(t *testing.T) {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", "   "))
		resp, err := h.handleGetCoordinatorItem(ctx, msg)
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeBadRequest)
	})
}

// TestHandleGetCoordinatorItem_NonCoordinatorPrincipalForbidden proves the
// handler itself (not just the guard) refuses a caller without a coordinator
// principal, defense in depth against a guard bypass, matching
// handleProposeTask.
func TestHandleGetCoordinatorItem_NonCoordinatorPrincipalForbidden(t *testing.T) {
	h, _, _, _ := newGetItemTestHandlers(t)
	msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", "prop-1"))

	resp, err := h.handleGetCoordinatorItem(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

// TestHandleGetCoordinatorItem_CoordinatorServiceUnavailable proves a nil
// coordinatorSvc fails closed with a distinct error rather than panicking.
func TestHandleGetCoordinatorItem_CoordinatorServiceUnavailable(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	ctx := getItemPrincipalContext("ws-1", "coordinator-1")
	msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("proposal", "prop-1"))

	resp, err := h.handleGetCoordinatorItem(ctx, msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeUnavailable)
}
