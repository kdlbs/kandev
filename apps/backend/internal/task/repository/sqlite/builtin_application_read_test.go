package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuiltinApplicationReadIsScopedToCurrentWorkspace(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task")
	seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, "execution")
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_applications (session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,created_at,updated_at) VALUES (?,?,?,3,0,'hash','human','request',?,?)`, "session", "task", task.WorkspaceID, time.Now().UTC(), time.Now().UTC())
	require.NoError(t, err)
	reader, ok := any(repo).(interface {
		GetBuiltinMCPApplication(context.Context, string) (*models.BuiltinMCPApplication, error)
	})
	require.True(t, ok, "task repository must expose persisted application status")
	status, err := reader.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.EqualValues(t, 3, status.TargetRevision)
	require.Equal(t, "pending", status.Stage)
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET workspace_id='old-workspace' WHERE session_id='session'`)
	require.NoError(t, err)
	status, err = reader.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.Nil(t, status, "a transferred task must not reuse the old workspace's application intent")
}
