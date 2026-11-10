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

func TestPostgresStepCompletionOutcome_ReplacementWinsCommitRace(t *testing.T) {
	exerciseStepCompletionOutcomeReplacementWinsCommitRace(t, newStepCompletionOutcomePostgresRepo(t))
}

func TestPostgresStepCompletionOutcome_MoveAndReceiptRollback(t *testing.T) {
	exerciseStepCompletionOutcomeMoveAndReceiptRollback(t, newStepCompletionOutcomePostgresRepo(t))
}

func TestPostgresStepCompletionOutcome_ReentryRejectsOldSource(t *testing.T) {
	exerciseStepCompletionOutcomeReentryRejectsOldSource(t, newStepCompletionOutcomePostgresRepo(t))
}
