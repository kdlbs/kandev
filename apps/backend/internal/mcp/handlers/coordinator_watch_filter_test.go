package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func workflowField(id string) map[string]json.RawMessage {
	return map[string]json.RawMessage{"workflow_id": json.RawMessage(`"` + id + `"`)}
}

// The watch filter applies the effective set: a selected set admits only its
// workflows, an unset filter admits reads that name no workflow, and an
// unreadable set refuses.
func TestCoordinatorWatchesFields_EffectiveSet(t *testing.T) {
	f := newPhase2GuardFixture(t)
	ctx := context.Background()
	_, err := f.rawDB.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT)`)
	require.NoError(t, err)
	_, err = f.rawDB.Exec(`INSERT INTO workflows (id, workspace_id, name) VALUES ('wf-a', ?, 'a'), ('wf-b', ?, 'b')`, f.c.WorkspaceID, f.c.WorkspaceID)
	require.NoError(t, err)
	_, err = f.svc.SaveSettings(ctx, f.c.WorkspaceID, f.c.ID, []byte(`{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`))
	require.NoError(t, err)

	principal := coordinatorTestPrincipal(f.c.WorkspaceID)
	principal.CoordinatorID = f.c.ID
	require.True(t, f.h.coordinatorWatchesFields(ctx, principal, ws.ActionMCPListTasks, workflowField("wf-a")))
	require.False(t, f.h.coordinatorWatchesFields(ctx, principal, ws.ActionMCPListTasks, workflowField("wf-b")))
	require.True(t, f.h.coordinatorWatchesFields(ctx, principal, ws.ActionMCPListRepositories, nil))

	_, err = f.rawDB.Exec(`DROP TABLE workflows`)
	require.NoError(t, err)
	require.False(t, f.h.coordinatorWatchesFields(ctx, principal, ws.ActionMCPListTasks, workflowField("wf-a")),
		"an unreadable watch set must fail closed")
}
