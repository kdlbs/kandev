package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type orphanSQLTaskStates struct {
	*mockTaskRepo
	repo *sqliterepo.Repository
}

func (r orphanSQLTaskStates) UpdateTaskState(ctx context.Context, id string, state v1.TaskState) error {
	if err := r.repo.UpdateTaskState(ctx, id, state); err != nil {
		return err
	}
	return r.mockTaskRepo.UpdateTaskState(ctx, id, state)
}

func (r orphanSQLTaskStates) UpdateTaskStateIfCurrentIn(ctx context.Context, id string, state v1.TaskState, expected []v1.TaskState) (bool, error) {
	_, changed, err := r.repo.UpdateTaskStateIfCurrentIn(ctx, id, state, expected)
	return changed, err
}

func newOrphanInterruptionFixture(t *testing.T) *schedulingOrphanFixture {
	t.Helper()
	f := newSchedulingOrphanFixture(t)
	f.svc.taskRepo = orphanSQLTaskStates{mockTaskRepo: f.taskRepo, repo: f.repo}
	t.Cleanup(f.svc.stopOrphanReentryWorkers)
	return f
}

func orphanTaskWithEntry(t *testing.T, f *schedulingOrphanFixture, taskID string) *models.Task {
	t.Helper()
	tasks, err := f.repo.ListSchedulingTasks(t.Context())
	require.NoError(t, err)
	for _, task := range tasks {
		if task.ID == taskID {
			_, _, entryID, err := f.repo.GetTaskWorkflowStepEntry(t.Context(), taskID)
			require.NoError(t, err)
			require.Positive(t, entryID)
			require.Equal(t, entryID, task.WorkflowStepTransitionID)
			return task
		}
	}
	t.Fatalf("SCHEDULING task %s was not listed", taskID)
	return nil
}

func seedOrphanResumableExecution(t *testing.T, f *schedulingOrphanFixture, taskID, sessionID string) {
	t.Helper()
	f.seedSession(t, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	now := time.Now().UTC()
	require.NoError(t, f.repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
		ID: sessionID + "-execution", TaskID: taskID, SessionID: sessionID,
		AgentExecutionID: "exec-before-restart", ResumeToken: "resume-" + sessionID,
		Resumable: true, CreatedAt: now, UpdatedAt: now,
	}))
	f.agentMgr.isAgentReadyFn = func(context.Context, string) bool { return true }
}

func orphanPromptCount(f *schedulingOrphanFixture) int {
	f.agentMgr.mu.Lock()
	defer f.agentMgr.mu.Unlock()
	return len(f.agentMgr.capturedPrompts)
}

func awaitOrphanEntryPrompt(t *testing.T, f *schedulingOrphanFixture, sessionID string) {
	t.Helper()
	require.True(t, f.launches.awaitLaunch(0), "the accepted step entry was not resumed")
	require.Eventually(t, func() bool {
		session, err := f.repo.GetTaskSession(t.Context(), sessionID)
		return err == nil && session.State == models.TaskSessionStateStarting
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), sessionID, models.TaskSessionStateWaitingForInput, ""))
	require.Eventually(t, func() bool { return orphanPromptCount(f) == 1 }, 5*time.Second, 10*time.Millisecond)
	f.svc.orphanReentryWorkers.Wait()
}

func TestSchedulingOrphanEarlierVisitDoesNotProveCurrentEntryDelivered(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	const taskID, sessionID = "orphan-revisited", "orphan-revisited-session"
	f.seedTask(t, taskID, orphanAutoStep, func(task *models.Task) {
		task.CreatedAt = time.Now().UTC().Add(-2 * time.Hour)
	})
	seedOrphanResumableExecution(t, f, taskID, sessionID)
	_, err := f.repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "earlier-visit-turn", TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: time.Now().UTC().Add(-time.Hour),
	})
	require.NoError(t, err)
	task, err := f.repo.GetTask(t.Context(), taskID)
	require.NoError(t, err)
	task.WorkflowStepID = orphanHoldStep
	require.NoError(t, f.repo.UpdateTask(t.Context(), task))
	task.WorkflowStepID = orphanAutoStep
	require.NoError(t, f.repo.UpdateTask(t.Context(), task))
	require.NoError(t, f.repo.UpdateTaskState(t.Context(), taskID, v1.TaskStateScheduling))
	entries, err := f.repo.ListTaskStepTransitions(t.Context(), taskID, 0, 1)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	acceptedEntryID := entries[0].ID

	f.runStartupSweep(t)
	awaitOrphanEntryPrompt(t, f, sessionID)

	task, err = f.repo.GetTask(t.Context(), taskID)
	require.NoError(t, err)
	route, found := models.LoadWorkflowSessionRoute(task.Metadata)
	require.True(t, found)
	require.Equal(t, workflowEntryIdentityForID(acceptedEntryID), route.EntryIdentity)
	require.Equal(t, sessionID, route.DestinationID)
	require.Equal(t, 1, sessionCount(t, f.repo, taskID))
	require.Equal(t, 1, orphanPromptCount(f))
}

func TestSchedulingOrphanLifecycleTurnDoesNotConsumeAcceptedEntry(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	const taskID, sessionID = "orphan-lifecycle", "orphan-lifecycle-session"
	f.seedTask(t, taskID, orphanAutoStep, nil)
	seedOrphanResumableExecution(t, f, taskID, sessionID)
	_, err := f.repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "boot-only-turn", TaskID: taskID, TaskSessionID: sessionID,
		Metadata: map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	})
	require.NoError(t, err)

	f.runStartupSweep(t)
	awaitOrphanEntryPrompt(t, f, sessionID)
	require.Equal(t, 1, sessionCount(t, f.repo, taskID))
	require.Equal(t, 1, orphanPromptCount(f))
}

func TestSchedulingOrphanCurrentEntryTurnRepairsSQLStateWithoutPrompt(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	const taskID, sessionID = "orphan-current-turn", "orphan-current-turn-session"
	publication := &recordingTaskEventPublisher{}
	f.svc.SetTaskEventPublisher(publication)
	f.seedTask(t, taskID, orphanAutoStep, nil)
	seedOrphanResumableExecution(t, f, taskID, sessionID)
	_, err := f.repo.CreateTurnWithStepStamp(t.Context(), &models.Turn{
		ID: "accepted-current-turn", TaskID: taskID, TaskSessionID: sessionID,
	})
	require.NoError(t, err)

	f.runStartupSweep(t)
	f.runStartupSweep(t)
	task, err := f.repo.GetTask(t.Context(), taskID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateInProgress, task.State)
	require.Equal(t, 0, f.launches.count())
	require.Equal(t, 0, orphanPromptCount(f))
	require.Equal(t, 1, sessionCount(t, f.repo, taskID))
	require.Equal(t, []recordedStateChange{{taskID: taskID, oldState: v1.TaskStateScheduling, newState: v1.TaskStateInProgress}}, publication.stateChanges)
}

func TestSchedulingOrphanMalformedDeferredIntentKeepsItsOwner(t *testing.T) {
	for name, value := range map[string]interface{}{
		"null": nil, "empty object": map[string]interface{}{}, "empty string": "", "malformed": "not-a-record",
	} {
		t.Run(name, func(t *testing.T) {
			f := newOrphanInterruptionFixture(t)
			f.seedTask(t, "orphan-malformed", orphanAutoStep, nil)
			require.NoError(t, f.repo.SetTaskMetadataKey(t.Context(), "orphan-malformed", models.MetaKeyDeferredLaunch, value))
			f.runStartupSweep(t)
			task, err := f.repo.GetTask(t.Context(), "orphan-malformed")
			require.NoError(t, err)
			stored, present := task.Metadata[models.MetaKeyDeferredLaunch]
			require.True(t, present)
			require.Equal(t, value, stored)
			require.Equal(t, v1.TaskStateScheduling, task.State)
			require.Equal(t, 0, sessionCount(t, f.repo, task.ID))
			require.False(t, f.launches.awaitLaunch(0))
		})
	}
}

type orphanOwnershipReadFailure struct{ *sqliterepo.Repository }

func (r orphanOwnershipReadFailure) GetTask(context.Context, string) (*models.Task, error) {
	return nil, errors.New("Office ownership read unavailable")
}

func TestSchedulingOrphanOwnershipReadFailureKeepsAcceptedWork(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	f.seedTask(t, "orphan-ownership-read", orphanAutoStep, nil)
	task, err := f.repo.GetTask(t.Context(), "orphan-ownership-read")
	require.NoError(t, err)
	f.svc.repo = orphanOwnershipReadFailure{Repository: f.repo}
	require.False(t, f.svc.schedulingOrphanRecoverable(t.Context(), task), "an unreadable owner must not authorize recovery")
	stored, err := f.repo.GetTask(t.Context(), task.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateScheduling, stored.State)
	require.Equal(t, 0, sessionCount(t, f.repo, task.ID))
}

func TestSchedulingOrphanEntryStampSeparatesVisitsAtTheSameClockTime(t *testing.T) {
	for _, currentEntry := range []bool{false, true} {
		t.Run(map[bool]string{false: "previous entry", true: "current entry"}[currentEntry], func(t *testing.T) {
			f := newOrphanInterruptionFixture(t)
			f.seedTask(t, "same-clock-task", orphanAutoStep, nil)
			f.seedSession(t, "same-clock-task", "same-clock-session", models.TaskSessionStateWaitingForInput)
			task := orphanTaskWithEntry(t, f, "same-clock-task")
			previousEntryID := task.WorkflowStepTransitionID
			task.WorkflowStepID = orphanHoldStep
			require.NoError(t, f.repo.UpdateTask(t.Context(), task))
			task.WorkflowStepID = orphanAutoStep
			require.NoError(t, f.repo.UpdateTask(t.Context(), task))
			task = orphanTaskWithEntry(t, f, task.ID)
			entryID := previousEntryID
			if currentEntry {
				entryID = task.WorkflowStepTransitionID
			}
			require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{
				ID: "same-clock-turn", TaskID: task.ID, TaskSessionID: "same-clock-session",
				StartedAt: time.Now().UTC().Add(-time.Hour),
				Metadata: map[string]interface{}{
					models.TurnMetaKeyWorkflowStepIDAtStart: orphanAutoStep,
					"workflow_step_transition_id_at_start":  entryID,
				},
			}))
			accepted, err := f.svc.sessionHasTurnForStep(t.Context(), "same-clock-session", task)
			require.NoError(t, err)
			require.Equal(t, currentEntry, accepted)
		})
	}
}
