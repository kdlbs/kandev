package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestUpsertPreservesBoundDeliveryBlock(t *testing.T) {
	testUpsertPreservesBoundDeliveryBlock(t, newRepoForSessionTests(t))
}

func TestPostgresUpsertPreservesBoundDeliveryBlock(t *testing.T) {
	testUpsertPreservesBoundDeliveryBlock(t, newPostgresRetentionTestRepo(t))
}

func testUpsertPreservesBoundDeliveryBlock(t *testing.T, repo *Repository) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
	ctx := context.Background()

	createSession := func(taskID, sessionID, incarnationID string) {
		t.Helper()
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}))
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
		}))
	}
	createSession("task-bound-delivery", "session-bound-delivery", "inc-bound-delivery")
	createSession("task-unbound-delivery", "session-unbound-delivery", "inc-unbound-delivery")

	bound := &models.SessionRecoveryBlock{
		ID: "block-bound-delivery", SessionID: "session-bound-delivery", IncarnationID: "inc-bound-delivery",
		ExpectedGeneration: 3, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-a", DeliveryStreamID: "stream-a",
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, bound))
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-interactive", SessionID: bound.SessionID, IncarnationID: bound.IncarnationID,
		ExpectedGeneration: bound.ExpectedGeneration, Reason: bound.Reason, State: models.RecoveryBlockOpen,
		ConsumerReference: "interactive",
	}))
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-delivery-b", SessionID: bound.SessionID, IncarnationID: bound.IncarnationID,
		ExpectedGeneration: bound.ExpectedGeneration, Reason: bound.Reason, State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-b",
	}))

	stored, err := repo.GetSessionRecoveryBlock(ctx, bound.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, stored.State)
	require.Equal(t, "agent_delivery", stored.ConsumerReference)
	require.Equal(t, "submission-a", stored.DeliverySubmissionID)
	require.Equal(t, "stream-a", stored.DeliveryStreamID)

	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET state = ? WHERE id = ?`),
		models.RecoveryBlockResolved, bound.ID)
	require.NoError(t, err)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	storedBinding, err := repo.findRecoveryBlockBindingTx(ctx, tx, "id = ?", bound.ID)
	require.NoError(t, err)
	conflictingResolved := &models.SessionRecoveryBlock{
		ID: bound.ID, SessionID: bound.SessionID, IncarnationID: bound.IncarnationID,
		ExpectedGeneration: bound.ExpectedGeneration, Reason: bound.Reason,
		State: models.RecoveryBlockResolved, ConsumerReference: "interactive", UpdatedAt: time.Now().UTC(),
	}
	updated, err := repo.updateResolvedSessionRecoveryBlockTx(ctx, tx, conflictingResolved, storedBinding)
	require.NoError(t, err)
	require.False(t, updated)
	require.NoError(t, tx.Commit())

	resolved, err := repo.GetSessionRecoveryBlock(ctx, bound.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, resolved.State)
	require.Equal(t, "agent_delivery", resolved.ConsumerReference)
	require.Equal(t, "submission-a", resolved.DeliverySubmissionID)

	unbound := &models.SessionRecoveryBlock{
		ID: "block-unbound-delivery", SessionID: "session-unbound-delivery", IncarnationID: "inc-unbound-delivery",
		ExpectedGeneration: 3, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, unbound))
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-bound-after-unbound", SessionID: unbound.SessionID, IncarnationID: unbound.IncarnationID,
		ExpectedGeneration: unbound.ExpectedGeneration, Reason: unbound.Reason, State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-control", DeliveryStreamID: "stream-control",
	}))

	control, err := repo.GetSessionRecoveryBlock(ctx, unbound.ID)
	require.NoError(t, err)
	require.Equal(t, "agent_delivery", control.ConsumerReference)
	require.Equal(t, "submission-control", control.DeliverySubmissionID)
	require.Equal(t, "stream-control", control.DeliveryStreamID)
}
