package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestExecutorStoppedStatusClearsOnlyCurrentLocalPID(t *testing.T) {
	assertExecutorStoppedStatusPID(t, newRepoForSessionTests(t))
}

func TestPostgresExecutorStoppedStatusClearsOnlyCurrentLocalPID(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	assertExecutorStoppedStatusPID(t, repo)
}

func assertExecutorStoppedStatusPID(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-terminal-pid")
	seedExecutorRunningCleanupSession(t, repo, "task-terminal-pid", "session-terminal-pid", models.TaskSessionStateRunning, "current-execution")
	row, err := repo.GetExecutorRunningBySessionID(ctx, "session-terminal-pid")
	require.NoError(t, err)
	row.LocalPID = 424242
	row.ResumeToken = "preserved-resume-token"
	row.Resumable = true
	row.WorktreePath = "/synthetic/worktree"
	require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
	before, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)

	require.ErrorIs(t, repo.UpdateExecutorRunningStatusIfCurrent(ctx, row.SessionID, "stale-execution", models.ExecutorRunningStatusStopped), models.ErrExecutionRotated)
	after, err := repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, before, after, "a stale stop must preserve the successor's complete row")

	require.NoError(t, repo.UpdateExecutorRunningStatusIfCurrent(ctx, row.SessionID, row.AgentExecutionID, models.ExecutorRunningStatusReady))
	after, err = repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Equal(t, row.LocalPID, after.LocalPID, "nonterminal status must preserve process ownership")

	require.NoError(t, repo.UpdateExecutorRunningStatusIfCurrent(ctx, row.SessionID, row.AgentExecutionID, models.ExecutorRunningStatusStopped))
	after, err = repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	require.Zero(t, after.LocalPID, "stopped status must clear the captured process attestation")
	before.Status = models.ExecutorRunningStatusStopped
	before.LocalPID = 0
	before.UpdatedAt = after.UpdatedAt
	require.Equal(t, before, after, "terminal persistence must preserve recovery and unrelated row metadata")
}
