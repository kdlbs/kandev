package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

type failOnceAdmissionRepo struct {
	*sqliterepo.Repository
	mu        sync.Mutex
	remaining int
	failures  int
}

func (r *failOnceAdmissionRepo) UpdateTaskWithWorkflowStepAdmissionIfAtStep(
	ctx context.Context, task *models.Task, expectedStepID, targetStepID string, limit int,
) (bool, error) {
	r.mu.Lock()
	if r.remaining > 0 {
		r.remaining--
		r.failures++
		r.mu.Unlock()
		return false, errors.New("one-shot completion proof failure")
	}
	r.mu.Unlock()
	return r.Repository.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, expectedStepID, targetStepID, limit)
}

func TestStepCompletionOutcome_LegacyAndEngine(t *testing.T) {
	for _, path := range []string{"legacy", "engine"} {
		for _, queued := range []bool{false, true} {
			name := path + "/" + map[bool]string{false: "admitted", true: "queued"}[queued]
			t.Run(name, func(t *testing.T) {
				repo := setupTestRepo(t)
				seedSession(t, repo, "task", "session", "step1")
				if queued {
					require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "occupant", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step2", Title: "Occupant", WIPAdmitted: true}))
				}
				entry := signalEntryForSession(t, repo, "session")
				stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{
					WorkflowStepTransitionID: entry, StepID: "step1", Summary: "done", Handoff: "review this",
				})
				require.NoError(t, err)
				require.True(t, claimed)
				require.NotEmpty(t, stored.SignalID)

				getter := newMockStepGetter()
				getter.steps["step1"] = &wfmodels.WorkflowStep{
					ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{
						OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}},
					},
				}
				getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1, WIPLimit: 1}
				svc := createTestService(repo, getter, newMockTaskRepo())
				session, err := repo.GetTaskSession(t.Context(), "session")
				require.NoError(t, err)
				task, err := repo.GetTask(t.Context(), "task")
				require.NoError(t, err)
				if path == "engine" {
					svc.initWorkflowEngine()
					require.True(t, svc.processOnTurnCompleteViaEngine(t.Context(), task.ID, session))
				} else {
					require.True(t, svc.processOnTurnComplete(t.Context(), task, session))
				}

				task, err = repo.GetTask(t.Context(), "task")
				require.NoError(t, err)
				require.Equal(t, "step2", task.WorkflowStepID)
				session, err = repo.GetTaskSession(t.Context(), "session")
				require.NoError(t, err)
				receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
				require.True(t, ok)
				require.Equal(t, stored.SignalID, receipt.SignalID)
				require.NotNil(t, receipt.Transition)
				_, currentStep, currentEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
				require.NoError(t, err)
				require.Equal(t, currentEntry, receipt.Transition.LedgerID)
				require.Equal(t, currentStep, receipt.Transition.StepID)
				require.Equal(t, "step2", receipt.Transition.StepID)
				require.Equal(t, queued, receipt.Transition.Queued)
				if queued {
					require.Equal(t, models.StepCompletionOutcomeDestinationQueued, receipt.Outcome)
				} else {
					require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
				}
			})
		}
	}
}

func TestStepCompletionOutcome_SupersededConsumerReconcilesLatest(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	now := time.Now().UTC()
	_, err := repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "turn-1", TaskSessionID: "session", TaskID: "task", StartedAt: now.Add(-2 * time.Second), CreatedAt: now.Add(-2 * time.Second),
	})
	require.NoError(t, err)
	entry := signalEntryForSession(t, repo, "session")
	first, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step1", SourceTurnID: "turn-1", Summary: "old handoff", Handoff: "old handoff",
	})
	require.NoError(t, err)
	require.True(t, claimed)

	// A later current turn replaces the unconsumed source.
	_, err = repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "turn-2", TaskSessionID: "session", TaskID: "task", StartedAt: now, CreatedAt: now,
	})
	require.NoError(t, err)
	second, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step1", SourceTurnID: "turn-2", Summary: "latest handoff", Handoff: "latest handoff",
	})
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotEqual(t, first.SignalID, second.SignalID)

	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	staleCtx := models.WithWorkflowStepCompletionSignalGuard(t.Context(), "task", "step1", *entry, "session", first.SignalID)
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(staleCtx, task, "step1", "step2", 0)
	require.NoError(t, err)
	require.False(t, applied, "a superseded consumer must not move the task")

	getter := newMockStepGetter()
	getter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}},
		},
	}
	getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}
	svc := createTestService(repo, getter, newMockTaskRepo())
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), "session", models.TaskSessionStateWaitingForInput, ""))
	retry := svc.onStepCompletionSignaled(t.Context(), bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{
		"task_id": "task", "session_id": "session", "step_id": "step1", "signal_id": first.SignalID,
	}))
	require.Nil(t, retry)

	task, err = repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step2", task.WorkflowStepID, "the current source must reconcile after a superseded consumer")
	carry, present := carryToken(t, repo, "task")
	require.True(t, present)
	require.Equal(t, "latest handoff", carry.Handoff)
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, second.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
}

func TestStepCompletionOutcome_NativeVerificationKeepsAcceptedSource(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	now := time.Now().UTC()
	_, err := repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "turn-1", TaskSessionID: "session", TaskID: "task", StartedAt: now.Add(-2 * time.Second), CreatedAt: now.Add(-2 * time.Second),
	})
	require.NoError(t, err)
	entry := signalEntryForSession(t, repo, "session")
	stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step1", SourceTurnID: "turn-1", Summary: "accepted", Handoff: "keep this handoff",
	})
	require.NoError(t, err)
	require.True(t, claimed)

	// Native verification output creates a newer turn without another call.
	_, err = repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "turn-2", TaskSessionID: "session", TaskID: "task", StartedAt: now, CreatedAt: now,
	})
	require.NoError(t, err)

	getter := newMockStepGetter()
	getter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}},
		},
	}
	getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}
	svc := createTestService(repo, getter, newMockTaskRepo())
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.True(t, svc.processOnTurnComplete(t.Context(), task, session))

	task, err = repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step2", task.WorkflowStepID)
	carry, present := carryToken(t, repo, "task")
	require.True(t, present)
	require.Equal(t, "keep this handoff", carry.Handoff)
	session, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, stored.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
}

func TestStepCompletionOutcome_ProofFailureRetriesSameSignal(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), "session", models.TaskSessionStateWaitingForInput, ""))
	entry := signalEntryForSession(t, repo, "session")
	stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step1", Summary: "done", Handoff: "retry handoff",
	})
	require.NoError(t, err)
	require.True(t, claimed)

	getter := newMockStepGetter()
	getter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}},
		},
	}
	getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}
	_, _, beforeEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	failing := &failOnceAdmissionRepo{Repository: repo, remaining: 1}
	svc := createTestService(repo, getter, newMockTaskRepo())
	svc.repo = failing
	svc.initWorkflowEngine()

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	observedUpdatedAt := session.UpdatedAt
	require.False(t, svc.processOnTurnCompleteViaEngine(t.Context(), "task", session))
	require.Equal(t, 1, failing.failures)

	storedTask, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step1", storedTask.WorkflowStepID)
	session, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, observedUpdatedAt, session.UpdatedAt, "a waiting session settlement must not masquerade as a new generation")
	pending, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, stored.SignalID, pending.SignalID)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomePending, receipt.Outcome)
	require.Nil(t, receipt.Transition)

	require.True(t, svc.processOnTurnCompleteViaEngine(t.Context(), "task", session),
		"the same durable signal must remain eligible after a transient proof failure")

	storedTask, err = repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step2", storedTask.WorkflowStepID)
	session, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok = models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, stored.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, receipt.Outcome)
	carry, present := carryToken(t, repo, "task")
	require.True(t, present)
	require.Equal(t, "retry handoff", carry.Handoff)
	_, _, afterEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, beforeEntry+1, afterEntry, "the retry must commit exactly one move")
}

func TestStepCompletionOutcome_PreflightRefusalRecordsBlocked(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	entry := signalEntryForSession(t, repo, "session")
	stored, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step1", Summary: "done",
	})
	require.NoError(t, err)
	require.True(t, claimed)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := models.WithWorkflowStepCompletionSignalGuard(t.Context(), "task", "step1", *entry, "session", stored.SignalID)
	svc.markStepCompletionBlocked(ctx, "task", "step1", "session", models.StepCompletionReasonDestinationUnavailable)

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeBlocked, receipt.Outcome)
	require.Equal(t, models.StepCompletionReasonDestinationUnavailable, receipt.Reason)
	require.Nil(t, receipt.Transition, "a blocked refusal must not claim a transition")
	pending, hasPending := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, hasPending, "the source route stays retryable")
	require.Equal(t, stored.SignalID, pending.SignalID)

	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	require.Equal(t, "step1", task.WorkflowStepID)
}
