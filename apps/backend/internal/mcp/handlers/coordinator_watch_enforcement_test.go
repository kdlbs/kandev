package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/coordinator"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// seedWatchWorld creates workflows wf-a and wf-b in both the task store and
// the coordinator's own store, tasks in each plus a workflow-less task, and
// saves Watches = selected [wf-a].
func seedWatchWorld(t *testing.T, f *guardFixture) {
	t.Helper()
	ctx := context.Background()
	_, err := f.rawDB.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT)`)
	require.NoError(t, err)
	now := time.Now().UTC()
	for _, id := range []string{"wf-a", "wf-b"} {
		_, err = f.rawDB.Exec(`INSERT INTO workflows (id, workspace_id, name) VALUES (?, ?, ?)`, id, f.c.WorkspaceID, id)
		require.NoError(t, err)
		require.NoError(t, f.repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: id, WorkspaceID: f.c.WorkspaceID, Name: id}))
		require.NoError(t, f.repo.CreateTask(ctx, &taskmodels.Task{
			ID: "task-" + id, WorkspaceID: f.c.WorkspaceID, WorkflowID: id, Title: id,
			State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}))
	}
	require.NoError(t, f.repo.CreateTask(ctx, &taskmodels.Task{
		ID: "task-none", WorkspaceID: f.c.WorkspaceID, Title: "no workflow",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	_, err = f.svc.SaveSettings(ctx, f.c.WorkspaceID, f.c.ID, []byte(`{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`))
	require.NoError(t, err)
}

func taskField(id string) map[string]json.RawMessage {
	return map[string]json.RawMessage{"task_id": json.RawMessage(`"` + id + `"`)}
}

func itemField(kind, id string) map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"kind": json.RawMessage(`"` + kind + `"`), "id": json.RawMessage(`"` + id + `"`),
	}
}

func TestCoordinatorWatchesFields_TaskAndItemTargets(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedWatchWorld(t, f)
	ctx := context.Background()
	principal := coordinatorTestPrincipal(f.c.WorkspaceID)
	principal.CoordinatorID = f.c.ID
	watches := func(action string, fields map[string]json.RawMessage) bool {
		return f.h.coordinatorWatchesFields(ctx, principal, action, fields)
	}

	require.True(t, watches(ws.ActionMCPGetTaskConversation, taskField("task-wf-a")))
	require.False(t, watches(ws.ActionMCPGetTaskConversation, taskField("task-wf-b")))
	require.False(t, watches(ws.ActionMCPGetTaskConversation, taskField("task-none")),
		"a task with no workflow is never watched")
	require.False(t, watches(ws.ActionMCPGetTaskConversation, taskField("missing")))

	require.True(t, watches(coordinator.ActionGetItem, itemField("stall", "task-wf-a")))
	require.False(t, watches(coordinator.ActionGetItem, itemField("stall", "task-wf-b")))
	require.False(t, watches(coordinator.ActionGetItem, itemField("stall", "task-none")))
	require.True(t, watches(coordinator.ActionGetItem, itemField("proposal", "task-wf-b")),
		"a proposal item names no task")
}

func TestListWorkflows_FilteredToEffectiveWatchSet(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedWatchWorld(t, f)
	ctx := f.ctxFor(nil, false)
	list := func() []string {
		msg := makeWSMessage(t, ws.ActionMCPListWorkflows, map[string]interface{}{"workspace_id": f.c.WorkspaceID})
		resp, err := f.h.handleListWorkflows(ctx, msg)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type, "%s", resp.Payload)
		var body struct {
			Workflows []struct {
				ID string `json:"id"`
			} `json:"workflows"`
			Total int `json:"total"`
		}
		require.NoError(t, json.Unmarshal(resp.Payload, &body))
		ids := []string{}
		for _, w := range body.Workflows {
			ids = append(ids, w.ID)
		}
		require.Equal(t, len(ids), body.Total)
		return ids
	}

	require.Equal(t, []string{"wf-a"}, list())

	_, err := f.rawDB.Exec(`DELETE FROM workflows WHERE id = 'wf-a'`)
	require.NoError(t, err)
	require.Empty(t, list(), "a selected set whose workflows are gone lists nothing")

	_, err = f.svc.SaveSettings(context.Background(), f.c.WorkspaceID, f.c.ID, []byte(`{"watches":{"scope":"all"}}`))
	require.NoError(t, err)
	require.Subset(t, list(), []string{"wf-a", "wf-b"})
}

func TestGetCoordinatorItem_StallOutsideWatchSetIsNotFound(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedWatchWorld(t, f)
	ctx := f.ctxFor(nil, false)
	for _, id := range []string{"task-wf-b", "task-none"} {
		f.seedStall(t, id)
		guarded, _, err := f.h.authorizeCoordinatorRequest(ctx, makeWSMessage(t, coordinator.ActionGetItem, map[string]interface{}{"kind": "stall", "id": id}))
		require.NoError(t, err)
		assertWSError(t, guarded, ws.ErrorCodeNotFound)

		resp, err := f.h.handleGetCoordinatorItem(ctx, makeWSMessage(t, coordinator.ActionGetItem, map[string]interface{}{"kind": "stall", "id": id}))
		require.NoError(t, err)
		assertWSError(t, resp, ws.ErrorCodeNotFound)
	}
	f.seedStall(t, "task-wf-a")
	resp, err := f.h.handleGetCoordinatorItem(ctx, makeWSMessage(t, coordinator.ActionGetItem, map[string]interface{}{"kind": "stall", "id": "task-wf-a"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "%s", resp.Payload)
}

func (f *guardFixture) seedStall(t *testing.T, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := f.store.UpsertStall(context.Background(), &coordinator.Stall{
		TaskID: taskID, WorkspaceID: f.c.WorkspaceID, StalledForMs: 60_000, LastEventAt: now, DetectedAt: now,
	})
	require.NoError(t, err)
}
