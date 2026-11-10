package sqlite

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func exerciseStepCompletionEntry(t *testing.T, repo *Repository) {
	t.Helper()
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	signal := models.PendingStepCompletionSignal{StepID: "step-a", WorkflowStepTransitionID: &entry, Summary: "first", SignaledAt: time.Now().UTC()}
	stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", signal)
	require.NoError(t, err)
	require.True(t, claimed)
	require.EqualValues(t, entry, *stored.WorkflowStepTransitionID)
	signal.Summary = "duplicate"
	stored, claimed, err = repo.ClaimStepCompletionSignal(t.Context(), "task", "session", signal)
	require.NoError(t, err)
	require.False(t, claimed)
	require.Equal(t, "first", stored.Summary)
	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	for _, step := range []string{"step-b", "step-a"} {
		task.WorkflowStepID = step
		require.NoError(t, repo.UpdateTask(t.Context(), task))
	}
	_, claimed, err = repo.ClaimStepCompletionSignal(t.Context(), "task", "session", signal)
	require.ErrorIs(t, err, ErrStepCompletionStale)
	require.False(t, claimed)
	signal.WorkflowStepTransitionID = nil
	stored, claimed, err = repo.ClaimStepCompletionSignal(t.Context(), "task", "session", signal)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Greater(t, *stored.WorkflowStepTransitionID, entry)
	// Legacy same-step signals cannot mask a fresh, proven signal.
	legacy := models.PendingStepCompletionSignal{StepID: "step-a", Summary: "unproven"}
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, legacy))
	_, claimed, err = repo.ClaimStepCompletionSignal(t.Context(), "task", "session", stored)
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestStepCompletionEntry(t *testing.T) {
	exerciseStepCompletionEntry(t, newTurnStepStampTestRepo(t))
}

func TestStepCompletionEntryConcurrentClaims(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	var wg sync.WaitGroup
	results := make(chan bool, 4)
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			_, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step-a", Summary: "concurrent", SignaledAt: time.Now().UTC()})
			results <- claimed
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	count := 0
	for claimed := range results {
		if claimed {
			count++
		}
	}
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, count)
}

func TestStepCompletionEntryEmptyMetadata(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	_, err := repo.DB().Exec(`UPDATE task_sessions SET metadata='' WHERE id='session'`)
	require.NoError(t, err)
	_, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step-a", Summary: "done"})
	require.NoError(t, err)
	require.True(t, claimed)
}

func TestStepCompletionEntryUnledgeredTask(t *testing.T) {
	repo := newTurnStepStampTestRepo(t)
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	_, err := repo.DB().Exec(`DELETE FROM task_step_transitions WHERE task_id='task'`)
	require.NoError(t, err)
	zero := int64(0)
	signal, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step-a", WorkflowStepTransitionID: &zero, Summary: "legacy task"})
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, signal.WorkflowStepTransitionID)
	require.Zero(t, *signal.WorkflowStepTransitionID)
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	signal, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.NotNil(t, signal.WorkflowStepTransitionID)
	require.Zero(t, *signal.WorkflowStepTransitionID)
}

func exerciseStepCompletionEntryHandoffRebase(t *testing.T, repo *Repository, completion bool, handoff string) {
	t.Helper()
	seedTurnStepStampSession(t, repo, "task", "step-a", "session")
	require.NoError(t, repo.SetTaskMetadataKey(t.Context(), "task", models.MetaKeyStepHandoffCarry, "original"))
	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	require.NoError(t, repo.SetTaskMetadataKey(t.Context(), "task", "concurrent", "keep current"))
	require.NoError(t, repo.SetTaskMetadataKey(t.Context(), "task", models.MetaKeyStepHandoffCarry, "current"))
	if handoff == "" {
		delete(task.Metadata, models.MetaKeyStepHandoffCarry)
	} else {
		task.Metadata[models.MetaKeyStepHandoffCarry] = handoff
	}
	ctx := models.WithWorkflowStepTransitionGuard(t.Context(), "task", "step-a", entry)
	if completion {
		ctx = models.WithWorkflowStepCompletionGuard(t.Context(), "task", "step-a", entry)
	}
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "step-a", "step-b", 0)
	require.NoError(t, err)
	require.True(t, applied)
	stored, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "keep current", stored.Metadata["concurrent"])
	carry, present := stored.Metadata[models.MetaKeyStepHandoffCarry]
	switch {
	case !completion:
		require.Equal(t, "current", carry)
	case handoff == "":
		require.False(t, present)
	default:
		require.Equal(t, handoff, carry)
	}
}

func TestStepCompletionEntryHandoffRebase(t *testing.T) {
	for _, completion := range []bool{false, true} {
		for _, handoff := range []string{"new context", ""} {
			t.Run(fmt.Sprintf("completion=%t/handoff=%q", completion, handoff), func(t *testing.T) {
				exerciseStepCompletionEntryHandoffRebase(t, newTurnStepStampTestRepo(t), completion, handoff)
			})
		}
	}
}
