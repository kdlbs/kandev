package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuiltinApplicationClaimFencesSavedRevision(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task")
	seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput, "execution")
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	now := time.Now().UTC()
	_, err = repo.db.Exec(`UPDATE executors_running SET status='running', resumable=TRUE, resume_token='conversation' WHERE session_id='session'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_preferences (task_id,workspace_id,schema_version,revision,packages_json,updated_by,updated_at) VALUES ('task',?,1,3,'{}','human',?)`, task.WorkspaceID, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO task_builtin_mcp_applications (session_id,task_id,workspace_id,target_revision,policy_revision,selection_hash,requested_by,request_id,created_at,updated_at) VALUES ('session','task',?,3,0,'hash','human','request',?,?)`, task.WorkspaceID, now, now)
	require.NoError(t, err)
	claimer, ok := any(repo).(interface {
		ClaimBuiltinMCPApplication(context.Context, string, string, int64) (*models.BuiltinMCPApplication, error)
	})
	require.True(t, ok, "application claim must fence the saved revision and source execution")
	row, err := claimer.ClaimBuiltinMCPApplication(ctx, "session", "stale-execution", 1)
	require.NoError(t, err)
	require.Nil(t, row)
	row, err = claimer.ClaimBuiltinMCPApplication(ctx, "session", "execution", 1)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, "preparing", row.Stage)
	require.EqualValues(t, 1, row.AttemptGeneration)
	require.EqualValues(t, 3, row.AttemptRevision)
	again, err := claimer.ClaimBuiltinMCPApplication(ctx, "session", "execution", 1)
	require.NoError(t, err)
	require.Nil(t, again, "duplicate attempts must not own the same source execution")
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_applications SET stage='pending', row_version=3 WHERE session_id='session'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_preferences SET revision=4 WHERE task_id='task'`)
	require.NoError(t, err)
	stale, err := claimer.ClaimBuiltinMCPApplication(ctx, "session", "execution", 3)
	require.NoError(t, err)
	require.Nil(t, stale, "a changed saved preference invalidates the prepared target")
	_, err = repo.db.Exec(`UPDATE task_builtin_mcp_preferences SET revision=3 WHERE task_id='task'`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE executors_running SET agent_execution_id='' WHERE session_id='session'`)
	require.NoError(t, err)
	missing, err := claimer.ClaimBuiltinMCPApplication(ctx, "session", "", 3)
	require.NoError(t, err)
	require.Nil(t, missing, "a missing source identity cannot authorize an application attempt")
}

func TestBuiltinApplicationClaimPersistsOperationIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
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
	claim, err := repo.ClaimBuiltinMCPApplication(ctx, "session", "execution", 1)
	require.NoError(t, err)
	require.NotNil(t, claim)
	payload, err := json.Marshal(claim)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(payload, &view))
	require.NotEmpty(t, view["operation_id"], "the operation must be durable before helper communication")
	stored, err := repo.GetBuiltinMCPApplication(ctx, "session")
	require.NoError(t, err)
	storedPayload, err := json.Marshal(stored)
	require.NoError(t, err)
	require.JSONEq(t, string(payload), string(storedPayload))
}
