package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresBackgroundSettlementSerializesDelayedRunObservation(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const sessionID = "session-background-lock"
	seedSessionForTurns(t, repo, "task-background-lock", sessionID)
	require.NoError(t, repo.UpsertBackgroundWorkload(ctx, &models.BackgroundWorkload{
		ID: "work", SessionID: sessionID, TaskID: "task-background-lock", State: "running", Revision: 1,
	}))
	blocker, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	_, err = blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "background-work:"+sessionID)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		done <- repo.UpsertBackgroundRun(ctx, &models.BackgroundRun{ID: "late", WorkloadID: "work", SessionID: sessionID, State: "running"})
	}()
	waitForPostgresLockWait(t, ctx, repo, "pg_advisory_xact_lock")
	finished := time.Now().UTC().Truncate(time.Microsecond)
	_, err = blocker.ExecContext(ctx, `UPDATE task_session_background_work SET state='ended',finished_at=$1,revision=2 WHERE session_id=$2`, finished, sessionID)
	require.NoError(t, err)
	require.NoError(t, blocker.Commit())
	require.NoError(t, <-done)
	run, err := repo.GetBackgroundRun(ctx, sessionID, "late")
	require.NoError(t, err)
	require.Equal(t, "ended", run.State)
	require.True(t, finished.Equal(*run.FinishedAt))
	// A later provider result remains authoritative after the delayed start.
	code := 7
	require.NoError(t, repo.UpsertBackgroundRun(ctx, &models.BackgroundRun{
		ID: "late", WorkloadID: "work", SessionID: sessionID, State: "failed", ExitCode: &code, FinishedAt: &finished,
	}))
	run, err = repo.GetBackgroundRun(ctx, sessionID, "late")
	require.NoError(t, err)
	require.Equal(t, "failed", run.State)
	require.Equal(t, &code, run.ExitCode)
	require.NoError(t, repo.UpsertBackgroundRun(ctx, &models.BackgroundRun{ID: "late", WorkloadID: "work", SessionID: sessionID, State: "waiting"}))
	run, err = repo.GetBackgroundRun(ctx, sessionID, "late")
	require.NoError(t, err)
	require.Equal(t, "failed", run.State)
	require.Equal(t, &code, run.ExitCode)
}
