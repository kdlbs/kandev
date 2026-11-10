package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func createStepCompletionOutcomeTurn(t *testing.T, repo *Repository, turnID, taskID, sessionID string, startedAt time.Time) {
	t.Helper()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     startedAt,
		CreatedAt:     startedAt,
	}
	stamped, err := repo.CreateTurnWithStepStamp(t.Context(), turn)
	require.NoError(t, err)
	require.True(t, stamped, "turn must carry its workflow-step stamp")
}

func claimStepCompletionOutcomeSignal(t *testing.T, repo *Repository, taskID, sessionID, stepID, turnID, summary string) models.PendingStepCompletionSignal {
	t.Helper()
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), taskID)
	require.NoError(t, err)
	stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), taskID, sessionID, models.PendingStepCompletionSignal{
		SourceTurnID:             turnID,
		WorkflowStepTransitionID: &entry,
		StepID:                   stepID,
		Summary:                  summary,
		SignaledAt:               time.Now().UTC(),
	})
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEmpty(t, stored.SignalID)
	return stored
}

func exerciseStepCompletionOutcomeReplacementWinsCommitRace(t *testing.T, repo *Repository) {
	t.Helper()
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	createStepCompletionOutcomeTurn(t, repo, "turn-1", "task", "session", time.Now().UTC().Add(-2*time.Second))
	first := claimStepCompletionOutcomeSignal(t, repo, "task", "session", "step-a", "turn-1", "first")

	createStepCompletionOutcomeTurn(t, repo, "turn-2", "task", "session", time.Now().UTC())
	second := claimStepCompletionOutcomeSignal(t, repo, "task", "session", "step-a", "turn-2", "latest")
	require.NotEqual(t, first.SignalID, second.SignalID)

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	pending, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, second.SignalID, pending.SignalID)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, second.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomePending, receipt.Outcome)

	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	entry := *first.WorkflowStepTransitionID
	staleCtx := models.WithWorkflowStepCompletionSignalGuard(t.Context(), "task", "step-a", entry, "session", first.SignalID)
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(staleCtx, task, "step-a", "step-b", 0)
	require.NoError(t, err)
	require.False(t, applied, "a superseded source must not commit")

	session, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	pending, ok = models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, second.SignalID, pending.SignalID)
	require.Equal(t, "latest", pending.Summary)

	task, err = repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	currentCtx := models.WithWorkflowStepCompletionSignalGuard(t.Context(), "task", "step-a", entry, "session", second.SignalID)
	applied, err = repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(currentCtx, task, "step-a", "step-b", 0)
	require.NoError(t, err)
	require.True(t, applied)

	session, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok = models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, second.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
	require.NotNil(t, receipt.Transition)
	require.Equal(t, "step-b", receipt.Transition.StepID)
	require.NotZero(t, receipt.Transition.LedgerID)
	require.False(t, receipt.Transition.Queued)

	updated, err := repo.SetStepCompletionReceiptOutcome(t.Context(), "session", second.SignalID, models.StepCompletionOutcomeNotConfigured, models.StepCompletionReasonNoAutomaticMove)
	require.NoError(t, err)
	require.False(t, updated, "a committed receipt cannot be downgraded")
	session, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok = models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
}

func exerciseStepCompletionOutcomeMoveAndReceiptRollback(t *testing.T, repo *Repository) {
	t.Helper()
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	createStepCompletionOutcomeTurn(t, repo, "turn-1", "task", "session", time.Now().UTC())
	signal := claimStepCompletionOutcomeSignal(t, repo, "task", "session", "step-a", "turn-1", "done")
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyLastStepCompletionReceipt, nil))

	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	entry := *signal.WorkflowStepTransitionID
	ctx := models.WithWorkflowStepCompletionSignalGuard(t.Context(), "task", "step-a", entry, "session", signal.SignalID)
	_, err = repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "step-a", "step-b", 0)
	require.ErrorIs(t, err, ErrStepCompletionReceiptMissing)

	stored, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step-a", stored.WorkflowStepID, "a missing receipt must roll back the move")

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	pending, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, signal.SignalID, pending.SignalID, "the signal stays retryable")
}

func exerciseStepCompletionOutcomeReentryRejectsOldSource(t *testing.T, repo *Repository) {
	t.Helper()
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	createStepCompletionOutcomeTurn(t, repo, "turn-1", "task", "session", time.Now().UTC().Add(-2*time.Second))
	signal := claimStepCompletionOutcomeSignal(t, repo, "task", "session", "step-a", "turn-1", "done")

	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	for _, step := range []string{"step-b", "step-a"} {
		task.WorkflowStepID = step
		require.NoError(t, repo.UpdateTask(t.Context(), task))
	}
	_, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", signal)
	require.ErrorIs(t, err, ErrStepCompletionStale)
	require.False(t, claimed)

	task, err = repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	entry := *signal.WorkflowStepTransitionID
	ctx := models.WithWorkflowStepCompletionSignalGuard(t.Context(), "task", "step-a", entry, "session", signal.SignalID)
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "step-a", "step-b", 0)
	require.NoError(t, err)
	require.False(t, applied, "a prior-entry source must not authorize a re-entry move")
}

func TestStepCompletionOutcome_ReplacementWinsCommitRace(t *testing.T) {
	exerciseStepCompletionOutcomeReplacementWinsCommitRace(t, newTurnStepStampTestRepo(t))
}

func TestStepCompletionOutcome_MoveAndReceiptRollback(t *testing.T) {
	exerciseStepCompletionOutcomeMoveAndReceiptRollback(t, newTurnStepStampTestRepo(t))
}

func TestStepCompletionOutcome_ReentryRejectsOldSource(t *testing.T) {
	exerciseStepCompletionOutcomeReentryRejectsOldSource(t, newTurnStepStampTestRepo(t))
}

func TestStepCompletionOutcome_LegacySignalGetsUnprovenReceipt(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	legacy := models.PendingStepCompletionSignal{StepID: "step-a", WorkflowStepTransitionID: &entry, Summary: "legacy", SignaledAt: time.Now().UTC()}
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, legacy))
	stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step-a", WorkflowStepTransitionID: &entry, Summary: "duplicate"})
	require.NoError(t, err)
	require.False(t, claimed)
	require.Empty(t, stored.SignalID)

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	_, hasReceipt := models.LoadStepCompletionReceipt(session.Metadata)
	require.False(t, hasReceipt, "legacy signals must remain explicitly unproven")
}
