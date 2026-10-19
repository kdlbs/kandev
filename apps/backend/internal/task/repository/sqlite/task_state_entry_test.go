package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func schedulingEntryTestTask(t *testing.T, repo *Repository, id string) *models.Task {
	t.Helper()
	tasks, err := repo.ListSchedulingTasks(t.Context())
	require.NoError(t, err)
	for _, task := range tasks {
		if task.ID == id {
			_, _, entryID, err := repo.GetTaskWorkflowStepEntry(t.Context(), id)
			require.NoError(t, err)
			require.Positive(t, entryID)
			require.Equal(t, entryID, task.WorkflowStepTransitionID)
			return task
		}
	}
	t.Fatalf("SCHEDULING task %s was not listed", id)
	return nil
}

func exerciseStateRepairRequiresCurrentEntry(t *testing.T, repo *Repository) {
	t.Helper()
	seedTurnStepStampSession(t, repo, "state-entry-task", "step-a", "state-entry-session")
	require.NoError(t, repo.UpdateTaskState(t.Context(), "state-entry-task", v1.TaskStateScheduling))
	observed := schedulingEntryTestTask(t, repo, "state-entry-task")
	unbound, err := repo.GetTask(t.Context(), observed.ID)
	require.NoError(t, err)
	changed, _, err := repo.UpdateTaskStateIfCurrentEntry(t.Context(), unbound, v1.TaskStateInProgress)
	require.NoError(t, err)
	require.False(t, changed, "an ordinary task read does not bind a workflow entry")
	previousClock := repo.clockNow
	repo.clockNow = func() time.Time { return time.Date(2026, 10, 15, 13, 0, 0, 123456789, time.UTC) }
	changed, committedAt, err := repo.UpdateTaskStateIfCurrentEntry(t.Context(), observed, v1.TaskStateInProgress)
	repo.clockNow = previousClock
	require.NoError(t, err)
	require.True(t, changed)
	stored, err := repo.GetTask(t.Context(), observed.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateInProgress, stored.State)
	require.True(t, committedAt.Equal(stored.UpdatedAt))

	require.NoError(t, repo.UpdateTaskState(t.Context(), observed.ID, v1.TaskStateScheduling))
	observed = schedulingEntryTestTask(t, repo, observed.ID)
	move := *observed
	move.WorkflowStepID = "step-b"
	require.NoError(t, repo.UpdateTask(t.Context(), &move))
	move.WorkflowStepID = "step-a"
	require.NoError(t, repo.UpdateTask(t.Context(), &move))
	// Keep the old timestamp deliberately: entry identity must fence a revisit
	// even on a clock that cannot distinguish two writes.
	_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), observed.UpdatedAt, observed.ID)
	require.NoError(t, err)
	changed, _, err = repo.UpdateTaskStateIfCurrentEntry(t.Context(), observed, v1.TaskStateInProgress)
	require.NoError(t, err)
	require.False(t, changed)
	stored, err = repo.GetTask(t.Context(), observed.ID)
	require.NoError(t, err)
	_, _, entryID, err := repo.GetTaskWorkflowStepEntry(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Greater(t, entryID, observed.WorkflowStepTransitionID)
	require.Equal(t, v1.TaskStateScheduling, stored.State)
	stored.WorkflowStepTransitionID = entryID

	_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`), time.Now().UTC(), stored.ID)
	require.NoError(t, err)
	changed, _, err = repo.UpdateTaskStateIfCurrentEntry(t.Context(), stored, v1.TaskStateInProgress)
	require.NoError(t, err)
	require.False(t, changed)
	archived, err := repo.GetTask(t.Context(), stored.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateScheduling, archived.State)
}

func TestStateRepairRequiresCurrentEntrySQLite(t *testing.T) {
	exerciseStateRepairRequiresCurrentEntry(t, newTurnStepStampTestRepo(t))
}

func TestPostgresStateRepairRequiresCurrentEntry(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	exerciseStateRepairRequiresCurrentEntry(t, repo)
}
