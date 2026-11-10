package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresCanonicalDeliveryTurnWorkflowStamp(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	mover := openSecondPostgresConnection(t, testutil.PostgresDSNFromEnv(t), db)
	holder, err := mover.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, _, _, err = repo.readTaskStepInTx(t.Context(), holder, "task")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	tx, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = repo.canonicalDeliveryTurnTx(ctx, tx, "session", "task", "", &models.AgentDeliveryEvent{StreamID: "stream", Sequence: 1})
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "fallback must not wait on a task while holding session/delivery locks")
	require.Equal(t, "55P03", pgErr.Code)
}

func TestPostgresCanonicalDeliveryTurnRetryAndCancellation(t *testing.T) {
	for _, cancelRetry := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "cancel"}[cancelRetry], func(t *testing.T) {
			dsn := testutil.PostgresDSNFromEnv(t)
			db := testutil.OpenIsolatedPostgres(t, dsn)
			repo, err := NewWithDB(db, db, nil)
			require.NoError(t, err)
			seedTurnStepStampSession(t, repo, "task", "step-a", "session")
			seedAgentDeliveryTestSession(t, repo, "session", "incarnation")
			event := canonicalStampEvent(t, repo, "text", 1)
			mover := openSecondPostgresConnection(t, dsn, db)
			gate := openSecondPostgresConnection(t, dsn, db)
			observer := openSecondPostgresConnection(t, dsn, db)
			holder, err := mover.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = holder.Rollback() }()
			_, _, _, err = repo.readTaskStepInTx(t.Context(), holder, "task")
			require.NoError(t, err)
			barrier, err := gate.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = barrier.Rollback() }()
			require.NoError(t, lockSessionTurnWrites(t.Context(), barrier, "pgx", "session"))
			var pid int
			require.NoError(t, db.Get(&pid, "SELECT pg_backend_pid()"))
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			done := make(chan struct{})
			var projectErr error
			go func() {
				defer close(done)
				_, projectErr = repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event))
			}()
			defer func() { cancel(); _ = holder.Rollback(); _ = barrier.Rollback(); <-done }()
			require.NoError(t, waitForPostgresLock(ctx, observer, pid, done))
			require.NoError(t, barrier.Commit())
			if cancelRetry {
				cancel()
				<-done
				require.ErrorIs(t, projectErr, context.Canceled)
			} else {
				// The task-first writer can take the session lock while projection retries.
				// A blocking task read under session locks would deadlock this acquisition.
				require.NoError(t, lockSessionTurnWrites(ctx, holder, "pgx", "session"))
				now := time.Now().UTC()
				_, err = holder.ExecContext(ctx, `UPDATE tasks SET workflow_step_id='step-b',updated_at=$1 WHERE id='task'`, now)
				require.NoError(t, err)
				_, err = repo.recordStepTransition(ctx, holder, stepTransitionInput{taskID: "task", fromWorkflowID: "wf-turn-stamp", fromWorkflowStepID: "step-a", toWorkflowID: "wf-turn-stamp", toWorkflowStepID: "step-b", occurredAt: now})
				require.NoError(t, err)
				require.NoError(t, holder.Commit())
				<-done
				require.NoError(t, projectErr)
				turn, err := repo.GetTurn(t.Context(), "agent-delivery-turn-stream-1")
				require.NoError(t, err)
				require.Equal(t, "step-b", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
				_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
				require.NoError(t, err)
				require.EqualValues(t, entry, turn.Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart])
			}
		})
	}
}

func TestPostgresCanonicalDeliveryTurnNoStep(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedTurnStepStampSession(t, repo, "task", "", "session")
	seedAgentDeliveryTestSession(t, repo, "session", "incarnation")
	event := canonicalStampEvent(t, repo, "text", 1)
	for range 2 {
		_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, completedRetentionEventEffect(event))
		require.NoError(t, err)
	}
	turn, err := repo.GetTurn(t.Context(), "agent-delivery-turn-stream-1")
	require.NoError(t, err)
	require.Empty(t, turn.Metadata)
}
