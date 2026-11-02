package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresStepCompletionEntry(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	exerciseStepCompletionEntry(t, repo)
}

func TestPostgresStepCompletionEntryConcurrentMove(t *testing.T) {
	for _, stage := range []string{"claim", "transition"} {
		t.Run(stage, func(t *testing.T) { exercisePostgresCompletionMove(t, stage) })
	}
}

func exercisePostgresCompletionMove(t *testing.T, stage string) {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	_, _, oldEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	mover := openSecondPostgresConnection(t, dsn, db)
	observer := openSecondPostgresConnection(t, dsn, db)
	tx, err := mover.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, _, _, err = repo.readTaskStepInTx(t.Context(), tx, "task")
	require.NoError(t, err)
	var pid int
	require.NoError(t, db.Get(&pid, `SELECT pg_backend_pid()`))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	done := make(chan struct{})
	var claimErr error
	var applied bool
	go func() {
		defer close(done)
		if stage == "transition" {
			guarded := models.WithWorkflowStepTransitionGuard(ctx, "task", "step-a", oldEntry)
			applied, claimErr = repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(guarded, task, "step-a", "step-b", 0)
		} else {
			_, _, claimErr = repo.ClaimStepCompletionSignal(ctx, "task", "session", models.PendingStepCompletionSignal{StepID: "step-a", WorkflowStepTransitionID: &oldEntry, Summary: "stale"})
		}
	}()
	defer func() { cancel(); _ = tx.Rollback(); <-done }()
	require.NoError(t, waitForPostgresLock(ctx, observer, pid, done))
	from := "step-a"
	for _, step := range []string{"step-b", "step-a"} {
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET workflow_step_id=$1,updated_at=$2 WHERE id='task'`, step, now)
		require.NoError(t, err)
		_, err = repo.recordStepTransition(ctx, tx, stepTransitionInput{taskID: "task", fromWorkflowID: "wf-turn-stamp", fromWorkflowStepID: from, toWorkflowID: "wf-turn-stamp", toWorkflowStepID: step, occurredAt: now})
		require.NoError(t, err)
		from = step
	}
	require.NoError(t, tx.Commit())
	<-done
	if stage == "transition" {
		require.NoError(t, claimErr)
		require.False(t, applied, "a same-step revisit must invalidate the final move")
	} else {
		require.ErrorIs(t, claimErr, ErrStepCompletionStale)
	}
	current, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step-a", current.WorkflowStepID)
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	_, has := models.LoadPendingStepSignal(session.Metadata)
	require.False(t, has)
}

func TestPostgresStepCompletionEntryHandoffRebase(t *testing.T) {
	for _, completion := range []bool{false, true} {
		for _, handoff := range []string{"new context", ""} {
			t.Run(fmt.Sprintf("completion=%t/handoff=%q", completion, handoff), func(t *testing.T) {
				db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
				repo, err := NewWithDB(db, db, nil)
				require.NoError(t, err)
				exerciseStepCompletionEntryHandoffRebase(t, repo, completion, handoff)
			})
		}
	}
}
