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

// seedProjectWorld creates one workflow with three tasks: one in repo-in, one
// in repo-out and one with no repository. The coordinator watches the
// workflow and the project scope lists repo-in only.
func seedProjectWorld(t *testing.T, f *guardFixture, includeNoRepo bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := f.rawDB.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT)`)
	require.NoError(t, err)
	_, err = f.rawDB.Exec(`INSERT INTO workflows (id, workspace_id, name) VALUES ('wf', ?, 'wf')`, f.c.WorkspaceID)
	require.NoError(t, err)
	require.NoError(t, f.repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf", WorkspaceID: f.c.WorkspaceID, Name: "wf"}))

	for _, id := range []string{"repo-in", "repo-out"} {
		require.NoError(t, f.repo.CreateRepository(ctx, &taskmodels.Repository{
			ID: id, WorkspaceID: f.c.WorkspaceID, Name: id, SourceType: "local", LocalPath: "/tmp/" + id,
		}))
	}
	for _, id := range []string{"in", "out", "none"} {
		require.NoError(t, f.repo.CreateTask(ctx, &taskmodels.Task{
			ID: "task-" + id, WorkspaceID: f.c.WorkspaceID, WorkflowID: "wf", Title: id,
			State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
		}))
	}
	for id, repoID := range map[string]string{"in": "repo-in", "out": "repo-out"} {
		require.NoError(t, f.repo.CreateTaskRepository(ctx, &taskmodels.TaskRepository{
			TaskID: "task-" + id, RepositoryID: repoID, BaseBranch: "main",
		}))
	}

	svc := coordinator.NewService(f.store, coordinator.NewValidator(nil, nil), allowAllAuthorizer{}, testLogger(t),
		coordinator.WithPhase2(true), coordinator.WithPhase31(true))
	svc.SetProjectReader(f.h.taskSvc)
	f.h.SetCoordinatorService(svc)
	f.svc = svc

	body, err := json.Marshal(map[string]any{
		"watches": map[string]any{"scope": "selected", "workflow_ids": []string{"wf"}},
		"projects": map[string]any{
			"scope":                 "selected",
			"entries":               []map[string]string{{"kind": "repository", "id": "repo-in"}},
			"include_no_repository": includeNoRepo,
		},
	})
	require.NoError(t, err)
	_, err = svc.SaveSettings(ctx, f.c.WorkspaceID, f.c.ID, body)
	require.NoError(t, err)
}

func listedTaskIDs(t *testing.T, f *guardFixture) []string {
	t.Helper()
	msg := makeWSMessage(t, ws.ActionMCPListTasks, map[string]interface{}{"workflow_id": "wf"})
	resp, err := f.h.handleListTasks(f.ctxFor(nil, false), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, "%s", resp.Payload)
	var body struct {
		Tasks []struct {
			ID string `json:"id"`
		} `json:"tasks"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	ids := []string{}
	for _, task := range body.Tasks {
		ids = append(ids, task.ID)
	}
	require.Equal(t, len(ids), body.Total, "total is the filtered count")
	return ids
}

func TestListTasks_FilteredToProjectScope(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedProjectWorld(t, f, false)
	require.ElementsMatch(t, []string{"task-in"}, listedTaskIDs(t, f))

	_, err := f.svc.SaveSettings(context.Background(), f.c.WorkspaceID, f.c.ID,
		[]byte(`{"projects":{"scope":"selected","entries":[{"kind":"repository","id":"repo-in"}],"include_no_repository":true}}`))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"task-in", "task-none"}, listedTaskIDs(t, f))

	_, err = f.svc.SaveSettings(context.Background(), f.c.WorkspaceID, f.c.ID, []byte(`{"projects":{"scope":"all"}}`))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"task-in", "task-out", "task-none"}, listedTaskIDs(t, f))
}

func TestListTasks_StoredProjectScopeIsEnforcedWithPhase31Off(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedProjectWorld(t, f, false)
	off := coordinator.NewService(f.store, coordinator.NewValidator(nil, nil), allowAllAuthorizer{}, testLogger(t), coordinator.WithPhase2(true))
	off.SetProjectReader(f.h.taskSvc)
	f.h.SetCoordinatorService(off)
	f.svc = off

	require.ElementsMatch(t, []string{"task-in"}, listedTaskIDs(t, f))
	principal := coordinatorTestPrincipal(f.c.WorkspaceID)
	principal.CoordinatorID = f.c.ID
	require.True(t, f.h.coordinatorWatchesFields(context.Background(), principal, ws.ActionMCPGetTaskConversation, taskField("task-in")))
	require.False(t, f.h.coordinatorWatchesFields(context.Background(), principal, ws.ActionMCPGetTaskConversation, taskField("task-out")))
}

func TestGetCoordinatorItem_StallOfAnOutOfProjectTaskIsNotFound(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedProjectWorld(t, f, false)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []string{"task-in", "task-out"} {
		_, err := f.store.UpsertStall(ctx, &coordinator.Stall{
			TaskID: id, WorkspaceID: f.c.WorkspaceID, StalledForMs: 60_000, LastEventAt: now, DetectedAt: now,
		})
		require.NoError(t, err)
	}
	read := func(id string) *ws.Message {
		msg := makeWSMessage(t, coordinator.ActionGetItem, getItemPayload("stall", id))
		resp, err := f.h.handleGetCoordinatorItem(getItemPrincipalContext(f.c.WorkspaceID, f.c.ID), msg)
		require.NoError(t, err)
		return resp
	}
	require.Equal(t, ws.MessageTypeResponse, read("task-in").Type)
	out := read("task-out")
	assertWSError(t, out, ws.ErrorCodeNotFound)
	var errPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(out.Payload, &errPayload))
	require.Equal(t, "target not found", errPayload.Message)
}

func TestCoordinatorWatchesFields_ProjectScope(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedProjectWorld(t, f, false)
	principal := coordinatorTestPrincipal(f.c.WorkspaceID)
	principal.CoordinatorID = f.c.ID
	watches := func(taskID string) bool {
		return f.h.coordinatorWatchesFields(context.Background(), principal, ws.ActionMCPGetTaskConversation, taskField(taskID))
	}
	require.True(t, watches("task-in"))
	require.False(t, watches("task-out"))
	require.False(t, watches("task-none"))
}

func TestListTasks_ProjectReadFailureFailsClosed(t *testing.T) {
	f := newPhase2GuardFixture(t)
	seedProjectWorld(t, f, false)
	f.svc.SetProjectReader(nil)
	msg := makeWSMessage(t, ws.ActionMCPListTasks, map[string]interface{}{"workflow_id": "wf"})
	resp, err := f.h.handleListTasks(f.ctxFor(nil, false), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	require.Contains(t, string(resp.Payload), "projects")
}
