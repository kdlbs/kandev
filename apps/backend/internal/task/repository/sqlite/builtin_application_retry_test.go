package sqlite

import (
	"context"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBuiltinApplicationRetryIsExplicitAndVersioned(t *testing.T) {
	repo := newRepoForSessionTests(t)
	_, _, err := settingsstore.Provide(repo.db, repo.db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task")
	seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, "execution")
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = repo.db.Exec(`UPDATE executors_running SET status='running',resumable=TRUE,resume_token='native' WHERE session_id='session'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_preferences (task_id,workspace_id,schema_version,revision,packages_json,updated_by,updated_at) VALUES ('task',?,1,3,'{}','human',?)`, task.WorkspaceID, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_applications (session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,created_at,updated_at) VALUES ('session','task',?,3,0,'hash','human','request',?,?)`, task.WorkspaceID, now, now)
	require.NoError(t, err)
	app, err := repo.ClaimBuiltinMCPApplication(ctx, "session", "execution", 1)
	require.NoError(t, err)
	require.NotNil(t, app)
	app.AttemptSource = providerBindingSourceJSON(t)
	advanced, err := repo.AdvanceBuiltinMCPApplication(ctx, app, "retiring", "")
	require.NoError(t, err)
	require.True(t, advanced)
	app, err = repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	advanced, err = repo.AdvanceBuiltinMCPApplication(ctx, app, "failed", "provider_start_failed")
	require.NoError(t, err)
	require.True(t, advanced)
	app, err = repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	operation := app.OperationID
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET stage='accepting',reason='acceptance_unconfirmed' WHERE session_id='session'`)
	require.NoError(t, err)
	retry, ok := any(repo).(interface {
		RetryBuiltinMCPApplication(context.Context, string, string, string, int64, models.BuiltinMCPRetryTarget) (bool, error)
	})
	require.True(t, ok, "retry needs one explicit versioned request")
	accepted, err := retry.RetryBuiltinMCPApplication(ctx, "session", "human", "wrong-request", app.RowVersion, models.BuiltinMCPRetryTarget{TaskRevision: 3, PolicyRevision: 0, SelectionHash: "hash"})
	require.NoError(t, err)
	require.False(t, accepted)
	_, err = repo.DB().Exec(`INSERT INTO workspace_builtin_mcp_policies (workspace_id,schema_version,revision,packages_json,updated_by,updated_at) VALUES (?,1,1,'{}','admin',?)`, task.WorkspaceID, now)
	require.NoError(t, err)
	accepted, err = retry.RetryBuiltinMCPApplication(ctx, "session", "human", app.RequestID, app.RowVersion, models.BuiltinMCPRetryTarget{TaskRevision: 3, PolicyRevision: 0, SelectionHash: "hash"})
	require.NoError(t, err)
	require.False(t, accepted, "a policy edit during Retry cannot authorize a stale target")
	accepted, err = retry.RetryBuiltinMCPApplication(ctx, "session", "human", app.RequestID, app.RowVersion, models.BuiltinMCPRetryTarget{TaskRevision: 3, PolicyRevision: 1, SelectionHash: "rebased"})
	require.NoError(t, err)
	require.True(t, accepted, "an uncertain final acceptance must remain recoverable without losing its operation evidence")
	replay, err := retry.RetryBuiltinMCPApplication(ctx, "session", "human", app.RequestID, app.RowVersion, models.BuiltinMCPRetryTarget{TaskRevision: 3, PolicyRevision: 0, SelectionHash: "hash"})
	require.NoError(t, err)
	require.False(t, replay)
	current, err := repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, "pending", current.Stage)
	require.Equal(t, operation, current.RetryOfOperationID)
	require.Equal(t, "rebased", current.SelectionHash)
	require.EqualValues(t, 1, current.PolicyRevision)
}
