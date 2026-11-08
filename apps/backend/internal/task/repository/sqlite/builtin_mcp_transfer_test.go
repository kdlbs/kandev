package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuiltinTaskGrantDoesNotCrossWorkspaceTransfer(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `INSERT INTO task_builtin_mcp_preferences
		(task_id,workspace_id,schema_version,revision,packages_json,updated_by,updated_at) VALUES (?,?,1,1,?,'owner',?)`,
		task.ID, task.WorkspaceID, `{"configuration":"on"}`, time.Now().UTC())
	_, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	require.NoError(t, err)

	var preferences struct {
		WorkspaceID string `db:"workspace_id"`
		Packages    string `db:"packages_json"`
		Revision    int64  `db:"revision"`
	}
	require.NoError(t, repo.db.Get(&preferences, `SELECT workspace_id, packages_json, revision FROM task_builtin_mcp_preferences WHERE task_id = ?`, task.ID))
	require.Equal(t, "ws-destination", preferences.WorkspaceID)
	require.JSONEq(t, `{}`, preferences.Packages, "the new workspace requires a new explicit package choice")
	require.EqualValues(t, 2, preferences.Revision)
}
