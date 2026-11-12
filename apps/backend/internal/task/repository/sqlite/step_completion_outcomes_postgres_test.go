package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func newStepCompletionOutcomePostgresRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	return repo
}

// TestPostgresStepCompletionOutcome_ReplacementWinsCommitRace covers the source-specific completion contract.
func TestPostgresStepCompletionOutcome_ReplacementWinsCommitRace(t *testing.T) {
	exerciseStepCompletionOutcomeReplacementWinsCommitRace(t, newStepCompletionOutcomePostgresRepo(t))
}

// TestPostgresStepCompletionOutcome_MoveAndReceiptRollback covers the source-specific completion contract.
func TestPostgresStepCompletionOutcome_MoveAndReceiptRollback(t *testing.T) {
	exerciseStepCompletionOutcomeMoveAndReceiptRollback(t, newStepCompletionOutcomePostgresRepo(t))
}

// TestPostgresStepCompletionOutcome_ReentryRejectsOldSource covers the source-specific completion contract.
func TestPostgresStepCompletionOutcome_ReentryRejectsOldSource(t *testing.T) {
	exerciseStepCompletionOutcomeReentryRejectsOldSource(t, newStepCompletionOutcomePostgresRepo(t))
}

// TestPostgresStepCompletionOutcome_Snapshot covers the source-specific completion contract.
func TestPostgresStepCompletionOutcome_Snapshot(t *testing.T) {
	repo := newStepCompletionOutcomePostgresRepo(t)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	require.NoError(t, repo.SetSessionPrimary(t.Context(), "session"))

	snapshot, err := repo.GetTaskCompletionSnapshot(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "wf-turn-stamp", snapshot.WorkflowID)
	require.Equal(t, "step-a", snapshot.StepID)
	require.Positive(t, snapshot.EntryID)
	require.Equal(t, "session", snapshot.PrimarySessionID)

	_, err = repo.DB().Exec(`UPDATE task_sessions SET is_primary=0 WHERE id='session'`)
	require.NoError(t, err)
	snapshot, err = repo.GetTaskCompletionSnapshot(t.Context(), "task")
	require.NoError(t, err)
	require.Empty(t, snapshot.PrimarySessionID)
}
