package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func exerciseTurnEntryStampAcrossRevisit(t *testing.T, repo *Repository, receipt bool) {
	t.Helper()
	const taskID, sessionID = "entry-stamp-task", "entry-stamp-session"
	seedTurnStepStampSession(t, repo, taskID, "step-a", sessionID)
	task, err := repo.GetTask(t.Context(), taskID)
	require.NoError(t, err)
	_, _, firstEntryID, err := repo.GetTaskWorkflowStepEntry(t.Context(), taskID)
	require.NoError(t, err)
	require.Positive(t, firstEntryID)
	create := repo.CreateTurnWithStepStamp
	if receipt {
		create = func(ctx context.Context, turn *models.Turn) (bool, error) {
			stamped, result, err := repo.CreateTurnWithStepStampConversationReceipt(ctx, turn)
			require.NoError(t, err)
			require.NotNil(t, result)
			return stamped, err
		}
	}
	// A timestamp captured before waiting for the task lock cannot identify
	// the entry ultimately observed by the transactional step stamp.
	capturedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldTurn := &models.Turn{ID: "entry-stamp-old", TaskID: taskID, TaskSessionID: sessionID, StartedAt: capturedAt}
	_, err = create(t.Context(), oldTurn)
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, repo.UpdateTask(t.Context(), task))
	task.WorkflowStepID = "step-a"
	require.NoError(t, repo.UpdateTask(t.Context(), task))
	task, err = repo.GetTask(t.Context(), taskID)
	require.NoError(t, err)
	require.Equal(t, "step-a", task.WorkflowStepID)
	_, _, currentEntryID, err := repo.GetTaskWorkflowStepEntry(t.Context(), taskID)
	require.NoError(t, err)
	require.Greater(t, currentEntryID, firstEntryID)
	currentTurn := &models.Turn{ID: "entry-stamp-current", TaskID: taskID, TaskSessionID: sessionID, StartedAt: capturedAt}
	_, err = create(t.Context(), currentTurn)
	require.NoError(t, err)

	for id, entryID := range map[string]int64{oldTurn.ID: firstEntryID, currentTurn.ID: currentEntryID} {
		stored, err := repo.GetTurn(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, "step-a", stored.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
		require.Equal(t, float64(entryID), stored.Metadata["workflow_step_transition_id_at_start"])
	}
}

func TestTurnEntryStampAcrossRevisitSQLite(t *testing.T) {
	exerciseTurnEntryStampAcrossRevisit(t, newTurnStepStampTestRepo(t), false)
}

func TestPostgresTurnEntryStampAcrossRevisit(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	exerciseTurnEntryStampAcrossRevisit(t, repo, false)
}

func TestTurnEntryReceiptStampAcrossRevisitSQLite(t *testing.T) {
	exerciseTurnEntryStampAcrossRevisit(t, newTurnStepStampTestRepo(t), true)
}

func TestPostgresTurnEntryReceiptStampAcrossRevisit(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	exerciseTurnEntryStampAcrossRevisit(t, repo, true)
}
