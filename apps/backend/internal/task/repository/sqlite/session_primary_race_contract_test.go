package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func exercisePrimaryPromotionAndStaleWrite(t *testing.T, repo, promoter *Repository) {
	t.Helper()
	ctx := t.Context()
	const taskID, previousID, selectedID = "primary-race-task", "previous-primary", "selected-primary"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Primary race"}))
	for _, id := range []string{previousID, selectedID} {
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: id, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
		}))
	}
	require.NoError(t, repo.SetSessionPrimary(ctx, previousID))
	stale, err := repo.GetTaskSession(ctx, previousID)
	require.NoError(t, err)
	require.True(t, stale.IsPrimary)
	stale.ErrorMessage = "ordinary write survives promotion"

	start := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		<-start
		done <- repo.UpdateTaskSession(ctx, stale)
	}()
	go func() {
		<-start
		done <- promoter.SetSessionPrimary(ctx, selectedID)
	}()
	close(start)
	for range 2 {
		require.NoError(t, <-done)
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	require.NoError(t, err)
	var primaries []string
	for _, session := range sessions {
		if session.IsPrimary {
			primaries = append(primaries, session.ID)
		}
		if session.ID == previousID {
			require.Equal(t, "ordinary write survives promotion", session.ErrorMessage)
		}
	}
	require.Equal(t, []string{selectedID}, primaries)

	// Give the captured snapshot an explicit prior revision, independent of
	// the host clock resolution. Promotion must supersede that revision.
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET updated_at = ? WHERE id = ?`),
		time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), selectedID)
	require.NoError(t, err)
	snapshot, err := repo.GetTaskSession(ctx, selectedID)
	require.NoError(t, err)
	revision := snapshot.UpdatedAt
	snapshot.IsPrimary = false
	require.NoError(t, repo.SetSessionPrimary(ctx, previousID))
	changed, err := repo.UpdateTaskSessionIfCurrentSnapshot(ctx, snapshot, snapshot.State, revision, snapshot.Metadata)
	require.NoError(t, err)
	require.False(t, changed, "a superseded snapshot must not release the newer selection")
	selected, err := repo.GetPrimarySessionByTaskID(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, previousID, selected.ID)
}

func TestPrimaryPromotionAndStaleWriteSQLite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exercisePrimaryPromotionAndStaleWrite(t, repo, repo)
}

func TestPostgresPrimaryPromotionAndStaleWrite(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	second := openSecondPostgresConnection(t, dsn, db)
	promoter, err := NewWithDB(second, second, nil)
	require.NoError(t, err)
	exercisePrimaryPromotionAndStaleWrite(t, repo, promoter)
}
