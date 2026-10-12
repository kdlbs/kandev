package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newListActivityTestHandlers(t *testing.T, phase2 bool) (*Handlers, *coordinator.Store, *sqlx.DB, *coordinator.Coordinator) {
	t.Helper()
	taskSvc, _ := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	conn, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := coordinator.NewStore(conn, conn)
	require.NoError(t, err)
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), taskSvc, testLogger(t), coordinator.WithPhase2(phase2))
	c := &coordinator.Coordinator{WorkspaceID: workspaces[0].ID, Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	require.NoError(t, store.CreateCoordinator(ctx, c))
	h := &Handlers{taskSvc: taskSvc, logger: testLogger(t).WithFields()}
	h.SetCoordinatorService(svc)
	return h, store, conn, c
}

func seedListActivityRow(t *testing.T, store *coordinator.Store, conn *sqlx.DB, c *coordinator.Coordinator, id string, at time.Time) {
	t.Helper()
	actor := "user-1"
	require.NoError(t, store.InsertActivity(context.Background(), conn, coordinator.ActivityRow{
		ID: id, CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, ActionClass: coordinator.ActionMessage,
		Outcome: coordinator.ActivityApproved, Authorization: coordinator.AuthRequiresApproval,
		ActorUserID: &actor, UndoneBy: &actor, Detail: "d", CreatedAt: at, UpdatedAt: at,
	}))
}

func callListActivity(t *testing.T, h *Handlers, ctx context.Context, payload map[string]interface{}) *ws.Message {
	t.Helper()
	resp, err := h.handleListCoordinatorActivity(ctx, makeWSMessage(t, coordinator.ActionListActivity, payload))
	require.NoError(t, err)
	return resp
}

func TestHandleListCoordinatorActivity_OwnRowsWithoutIdentities(t *testing.T) {
	h, store, conn, c := newListActivityTestHandlers(t, true)
	other := &coordinator.Coordinator{WorkspaceID: c.WorkspaceID, Name: "Other", AgentProfileID: "a", ExecutorProfileID: "e"}
	require.NoError(t, store.CreateCoordinator(context.Background(), other))
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	seedListActivityRow(t, store, conn, c, "mine", base)
	seedListActivityRow(t, store, conn, other, "theirs", base.Add(time.Second))

	resp := callListActivity(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), map[string]interface{}{"coordinator_id": other.ID})
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "got: %s", resp.Payload)
	var payload struct {
		Rows []map[string]interface{} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.Rows, 1)
	row := payload.Rows[0]
	require.Equal(t, "mine", row["id"])
	for _, k := range []string{"actor_user_id", "undone_by", "actor_name", "undone_by_name"} {
		require.NotContains(t, row, k)
	}
	require.Contains(t, row, "created_at")
	require.Contains(t, row, "updated_at")
}

func TestHandleListCoordinatorActivity_DefaultLimitAndCursor(t *testing.T) {
	h, store, conn, c := newListActivityTestHandlers(t, true)
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 21; i++ {
		seedListActivityRow(t, store, conn, c, fmt.Sprintf("r%02d", i), base.Add(time.Duration(i)*time.Second))
	}
	ctx := getItemPrincipalContext(c.WorkspaceID, c.ID)
	var first struct {
		Rows       []map[string]interface{} `json:"rows"`
		NextCursor *string                  `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(callListActivity(t, h, ctx, map[string]interface{}{}).Payload, &first))
	require.Len(t, first.Rows, 20)
	require.NotNil(t, first.NextCursor)
	var second struct {
		Rows []map[string]interface{} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(callListActivity(t, h, ctx, map[string]interface{}{"before": *first.NextCursor}).Payload, &second))
	require.Len(t, second.Rows, 1)
	require.Equal(t, "r00", second.Rows[0]["id"])
}

func TestHandleListCoordinatorActivity_BadArguments(t *testing.T) {
	h, _, _, c := newListActivityTestHandlers(t, true)
	ctx := getItemPrincipalContext(c.WorkspaceID, c.ID)
	for name, payload := range map[string]map[string]interface{}{
		"limit zero":     {"limit": 0},
		"limit too big":  {"limit": 51},
		"limit negative": {"limit": -1},
		"limit fraction": {"limit": 1.5},
		"limit string":   {"limit": "ten"},
		"bad cursor":     {"before": "not-a-cursor!"},
		"cursor number":  {"before": 123},
	} {
		t.Run(name, func(t *testing.T) {
			assertWSError(t, callListActivity(t, h, ctx, payload), ws.ErrorCodeBadRequest)
		})
	}
}

func TestHandleListCoordinatorActivity_RefusesWithoutACoordinator(t *testing.T) {
	h, store, _, c := newListActivityTestHandlers(t, true)
	assertWSError(t, callListActivity(t, h, context.Background(), map[string]interface{}{}), ws.ErrorCodeNotFound)
	noBinding := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{WorkspaceID: c.WorkspaceID})
	assertWSError(t, callListActivity(t, h, noBinding, map[string]interface{}{}), ws.ErrorCodeNotFound)
	require.NoError(t, store.DeleteCoordinator(context.Background(), c.WorkspaceID, c.ID))
	assertWSError(t, callListActivity(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), map[string]interface{}{}), ws.ErrorCodeNotFound)
}

func TestHandleListCoordinatorActivity_PhaseOffIsNotFound(t *testing.T) {
	h, _, _, c := newListActivityTestHandlers(t, false)
	assertWSError(t, callListActivity(t, h, getItemPrincipalContext(c.WorkspaceID, c.ID), map[string]interface{}{}), ws.ErrorCodeNotFound)
}
