package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const entryGateStepID = "entry-gate-step"

type entryPromptFixture struct {
	ctx     context.Context
	repo    *sqliterepo.Repository
	svc     *Service
	taskID  string
	session string
}

// newEntryPromptFixture seeds a task on a signal-gated step whose session is
// in sessionState and has already recorded a completion signal for that step.
func newEntryPromptFixture(t *testing.T, taskID, sessionID string, sessionState models.TaskSessionState) *entryPromptFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, sessionState)

	stepGetter := newMockStepGetter()
	stepGetter.steps[entryGateStepID] = &wfmodels.WorkflowStep{
		ID:                         entryGateStepID,
		WorkflowID:                 "wf1",
		Name:                       "Implementation",
		AutoAdvanceRequiresSignal:  true,
		CancelTriggersTurnComplete: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["entry-other-step"] = &wfmodels.WorkflowStep{ID: "entry-other-step", WorkflowID: "wf1", Name: "Other"}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	task.WorkflowID = "wf1"
	task.WorkflowStepID = entryGateStepID
	require.NoError(t, repo.UpdateTaskPreservingDeferredLaunch(ctx, task))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: signalEntryForSession(t, repo, sessionID),
		StepID:                   entryGateStepID,
		Source:                   models.StepCompletionSourceAgent,
	}))
	return &entryPromptFixture{ctx: ctx, repo: repo, svc: svc, taskID: taskID, session: sessionID}
}

func (f *entryPromptFixture) task(t *testing.T) *models.Task {
	t.Helper()
	task, err := f.repo.GetTask(f.ctx, f.taskID)
	require.NoError(t, err)
	return task
}

// boundEntryPrompt builds the capacity-deferred entry prompt that the current
// workflow entry queued for destinationSessionID.
func (f *entryPromptFixture) boundEntryPrompt(t *testing.T, destinationSessionID string) models.CeilingDeferral {
	t.Helper()
	entryIdentity := f.svc.workflowEntryIdentity(f.ctx, f.taskID)
	binding := models.CeilingWorkflowEntryBinding{
		WorkflowID:           "wf1",
		DestinationStepID:    entryGateStepID,
		RouteOperationID:     workflowSessionRouteID(f.taskID, entryGateStepID, entryIdentity, nil, models.WorkflowProfileSessionStartPolicyReuse),
		EntryIdentity:        entryIdentity,
		DestinationSessionID: destinationSessionID,
	}
	return models.CeilingDeferral{
		Kind: models.CeilingLaunchPromptEnsure,
		Payload: map[string]interface{}{
			metaKeySessionID:                    destinationSessionID,
			metaKeyPrompt:                       "implementation entry prompt",
			models.CeilingLaunchEntryBindingKey: ceilingEntryBindingValue(binding),
		},
	}
}

func (f *entryPromptFixture) queue(t *testing.T, deferral models.CeilingDeferral) {
	t.Helper()
	require.NoError(t, f.repo.SetTaskMetadataKey(f.ctx, f.taskID, models.MetaKeyDeferredLaunch, models.CeilingRecordKeys(deferral)))
}

func (f *entryPromptFixture) advances(t *testing.T, cause turnCompletionCause) bool {
	t.Helper()
	session, err := f.repo.GetTaskSession(f.ctx, f.session)
	require.NoError(t, err)
	_, advances := f.svc.prepareEngineTurnCompletion(f.ctx, f.taskID, f.task(t), session, cause)
	return advances
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-003.1: a bound entry prompt waiting for
// capacity has not run yet, so a running destination is a reason to retry,
// not proof that the prompt was superseded.
func TestValidateCeilingEntry_BoundEntryPromptOnRunningDestinationIsRetried(t *testing.T) {
	f := newEntryPromptFixture(t, "entry-running", "entry-running-session", models.TaskSessionStateRunning)

	disposition, _, err := f.svc.validateCeilingEntry(f.ctx, f.task(t), f.boundEntryPrompt(t, f.session))
	require.NoError(t, err)
	require.Equal(t, ceilingEntryUnavailable, disposition,
		"a bound entry prompt whose destination is running must be kept for a later pass")
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-003.2
func TestValidateCeilingEntry_UnboundPromptOnRunningDestinationKeepsSupersededDisposition(t *testing.T) {
	f := newEntryPromptFixture(t, "entry-legacy", "entry-legacy-session", models.TaskSessionStateRunning)
	deferral := models.CeilingDeferral{
		Kind: models.CeilingLaunchPromptEnsure,
		Payload: map[string]interface{}{
			metaKeySessionID: f.session,
			metaKeyPrompt:    "legacy prompt",
		},
	}

	disposition, _, err := f.svc.validateCeilingEntry(f.ctx, f.task(t), deferral)
	require.NoError(t, err)
	require.Equal(t, ceilingEntrySuperseded, disposition)
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-003.3, 003.4
func TestPrepareEngineTurnCompletion_HoldsAdvanceWhileBoundEntryPromptPending(t *testing.T) {
	f := newEntryPromptFixture(t, "entry-hold", "entry-hold-session", models.TaskSessionStateWaitingForInput)
	require.True(t, f.advances(t, turnCompletionCauseAgentTurn),
		"a signaled gate with no pending entry prompt advances")

	f.queue(t, f.boundEntryPrompt(t, f.session))

	require.False(t, f.advances(t, turnCompletionCauseAgentTurn),
		"a valid queued entry prompt for this step holds the signal-gated advance")
	session, err := f.repo.GetTaskSession(f.ctx, f.session)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.True(t, f.advances(t, turnCompletionCauseUserCancellation),
		"a user-cancelled turn keeps its existing completion path")
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-003.4: an entry prompt whose destination
// session has ended is superseded, so it does not hold the advance.
func TestPrepareEngineTurnCompletion_SupersededEntryPromptDoesNotHoldAdvance(t *testing.T) {
	f := newEntryPromptFixture(t, "entry-ended", "entry-ended-session", models.TaskSessionStateWaitingForInput)
	now := time.Now().UTC()
	require.NoError(t, f.repo.CreateTaskSession(f.ctx, &models.TaskSession{
		ID: "entry-ended-destination", TaskID: f.taskID, State: models.TaskSessionStateCompleted,
		StartedAt: now, UpdatedAt: now,
	}))
	f.queue(t, f.boundEntryPrompt(t, "entry-ended-destination"))

	require.True(t, f.advances(t, turnCompletionCauseAgentTurn))
}

// A deferred-launch read error leaves the entry-prompt state unknown, so the
// signal-gated advance must stay held rather than release the hold.
func TestWorkflowEntryPromptPendingForStep_HoldsAdvanceWhenDeferredLaunchReadFails(t *testing.T) {
	f := newEntryPromptFixture(t, "entry-read-error", "entry-read-error-session", models.TaskSessionStateWaitingForInput)
	f.queue(t, f.boundEntryPrompt(t, f.session))
	f.svc.repo = entryPromptReadErrorRepo{sessionExecutorStore: f.repo, err: errors.New("deferred launch storage unavailable")}

	require.True(t, f.svc.workflowEntryPromptPendingForStep(f.ctx, f.task(t), entryGateStepID),
		"an unreadable deferred-launch record must hold the signal-gated advance")
}

// entryPromptReadErrorRepo fails deferred-launch reads while delegating the
// rest of the store to the real repository.
type entryPromptReadErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r entryPromptReadErrorRepo) GetTaskDeferredLaunch(
	context.Context, string,
) (map[string]interface{}, interface{}, error) {
	return nil, nil, r.err
}

// A held entry prompt is released by existing events rather than by an age
// limit: the destination's turn ends, which makes the prompt deliverable on
// the next ceiling sweep, or the task leaves the step, which supersedes it.
func TestValidateCeilingEntry_HeldEntryPromptEndsWithTurnOrStepChange(t *testing.T) {
	f := newEntryPromptFixture(t, "entry-release", "entry-release-session", models.TaskSessionStateRunning)
	deferral := f.boundEntryPrompt(t, f.session)

	require.NoError(t, f.repo.UpdateTaskSessionState(f.ctx, f.session, models.TaskSessionStateWaitingForInput, ""))
	disposition, _, err := f.svc.validateCeilingEntry(f.ctx, f.task(t), deferral)
	require.NoError(t, err)
	require.Equal(t, ceilingEntryValid, disposition, "an idle destination can receive the held prompt")

	task := f.task(t)
	task.WorkflowStepID = "entry-other-step"
	require.NoError(t, f.repo.UpdateTaskPreservingDeferredLaunch(f.ctx, task))
	disposition, _, err = f.svc.validateCeilingEntry(f.ctx, f.task(t), deferral)
	require.NoError(t, err)
	require.Equal(t, ceilingEntrySuperseded, disposition, "leaving the step supersedes the held prompt")
}

func TestPrepareEngineTurnCompletion_HoldsAdvanceWhenEntryBindingUnreadable(t *testing.T) {
	for _, binding := range []interface{}{
		map[string]interface{}{"destination_step_id": entryGateStepID},
		"unreadable binding",
	} {
		f := newEntryPromptFixture(t, "entry-binding-error", "entry-binding-error-session", models.TaskSessionStateWaitingForInput)
		deferral := f.boundEntryPrompt(t, f.session)
		deferral.Payload[models.CeilingLaunchEntryBindingKey] = binding
		f.queue(t, deferral)
		disposition, _, err := f.svc.validateCeilingEntry(f.ctx, f.task(t), deferral)
		require.NoError(t, err)
		require.Equal(t, ceilingEntryPermanentlyUnavailable, disposition,
			"a malformed durable binding is terminal for replay")
		require.False(t, f.advances(t, turnCompletionCauseAgentTurn),
			"an unreadable bound entry prompt must hold the signal-gated advance")
	}
}
