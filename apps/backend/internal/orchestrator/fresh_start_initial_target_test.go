package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.13
func TestFreshStartInitialTargetWithoutRoute(t *testing.T) {
	for _, action := range []string{"fresh_start", "runtime_retry"} {
		action := action
		t.Run(action+" retries the original session and retains its prompt", func(t *testing.T) {
			const receipt = "Continue with the requested change."
			fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
				withReceipt:    true,
				receiptContent: receipt,
				stepPrompt:     "Step instructions: {{task_prompt}}",
			})
			configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)

			observation := fixture.recover(t, action)

			require.NoError(t, observation.recoveryErr)
			require.Equal(t, 1, observation.launchCount)
			require.Len(t, observation.capturedPrompts, 1)
			require.Contains(t, observation.capturedPrompts[0], "Step instructions:")
			require.Contains(t, observation.capturedPrompts[0], receipt)
			require.NotNil(t, observation.receipt)
			require.Equal(t, models.InitialPromptSubmissionAccepted, observation.receipt.State)
			require.Equal(t, receipt, observation.receipt.Content)
		})
	}

	t.Run("a different snapshot session is superseded", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: "Recover the original prompt.",
		})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, "another-session")

		observation := fixture.recover(t, "fresh_start")

		require.ErrorContains(t, observation.recoveryErr, "workflow destination session changed")
		require.Zero(t, observation.launchCount)
	})

	t.Run("an earlier-step target still requires a committed route", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{
			withReceipt:    true,
			receiptContent: "Recover the original prompt.",
		})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetStep, fixture.sessionID)

		observation := fixture.recover(t, "fresh_start")

		require.Error(t, observation.recoveryErr)
		require.ErrorContains(t, observation.recoveryErr, "workflow entry route is unavailable")
		require.Zero(t, observation.launchCount)
	})
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.13
func TestValidateUnroutedInitialTarget(t *testing.T) {
	t.Run("changed task step is superseded", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, err)
		task.WorkflowStepID = "later-step"
		require.NoError(t, fixture.repo.UpdateTask(fixture.ctx, task))

		disposition, _, err := validateUnroutedInitialTarget(t, fixture, task)

		require.NoError(t, err)
		require.Equal(t, ceilingEntrySuperseded, disposition)
	})

	t.Run("changed entry identity is superseded", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, err)
		binding := initialTargetBinding(t, fixture)
		binding.EntryIdentity = "entry:stale"

		disposition, _, err := fixture.svc.validateUnroutedWorkflowEntry(
			fixture.ctx, task, models.CeilingDeferral{Payload: map[string]interface{}{}}, binding, false,
		)

		require.NoError(t, err)
		require.Equal(t, ceilingEntrySuperseded, disposition)
	})

	t.Run("malformed route metadata blocks a claimed resume", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, err)
		task.Metadata[models.MetaKeyWorkflowSessionRoute] = map[string]interface{}{"invalid": true}
		require.NoError(t, fixture.repo.UpdateTask(fixture.ctx, task))
		binding := initialTargetBinding(t, fixture)
		ctx := withCeilingEntryKind(fixture.ctx, models.CeilingLaunchResume)

		err = fixture.svc.validateClaimedCeilingBinding(ctx, fixture.taskID, &binding)

		require.ErrorContains(t, err, "deferred workflow entry ownership is unavailable")
	})

	t.Run("different bound destination session is superseded", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, err)
		binding := initialTargetBinding(t, fixture)
		binding.DestinationSessionID = "another-session"

		disposition, detail, err := fixture.svc.validateUnroutedWorkflowEntry(
			fixture.ctx, task, models.CeilingDeferral{Payload: map[string]interface{}{}}, binding, false,
		)

		require.NoError(t, err)
		require.Equal(t, ceilingEntrySuperseded, disposition)
		require.Equal(t, "workflow destination session changed", detail)
	})

	t.Run("matching payload destination session is valid", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, err)
		session, err := fixture.repo.GetTaskSession(fixture.ctx, fixture.sessionID)
		require.NoError(t, err)
		session.State = models.TaskSessionStateWaitingForInput
		require.NoError(t, fixture.repo.UpdateTaskSession(fixture.ctx, session))
		binding := initialTargetBinding(t, fixture)

		disposition, detail, err := fixture.svc.validateUnroutedWorkflowEntry(
			fixture.ctx,
			task,
			models.CeilingDeferral{Payload: map[string]interface{}{"session_id": fixture.sessionID}},
			binding,
			false,
		)

		require.NoError(t, err)
		require.Equal(t, ceilingEntryValid, disposition)
		require.Empty(t, detail)
	})

	t.Run("missing snapshot is permanently unavailable", func(t *testing.T) {
		fixture := newFreshStartWorkflowFixture(t, freshStartWorkflowOptions{})
		configureUnroutedWorkflowTarget(t, fixture, wfmodels.WorkflowSessionTargetInitial, fixture.sessionID)
		_, err := fixture.repo.RemoveTaskMetadataKey(
			fixture.ctx, fixture.taskID, models.MetaKeyWorkflowInitialSession,
		)
		require.NoError(t, err)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		require.NoError(t, err)

		disposition, _, err := validateUnroutedInitialTarget(t, fixture, task)

		require.NoError(t, err)
		require.Equal(t, ceilingEntryPermanentlyUnavailable, disposition)
	})
}

func configureUnroutedWorkflowTarget(
	t *testing.T,
	fixture *freshStartWorkflowFixture,
	kind wfmodels.WorkflowSessionTargetKind,
	snapshotSessionID string,
) {
	t.Helper()
	step := fixture.stepGetter.steps[fixture.stepID]
	step.SessionTarget = &wfmodels.WorkflowSessionTarget{Kind: kind}
	if kind == wfmodels.WorkflowSessionTargetStep {
		step.SessionTarget.StepID = "earlier-step"
	}
	require.NoError(t, fixture.repo.SetTaskMetadataKey(
		fixture.ctx,
		fixture.taskID,
		models.MetaKeyWorkflowInitialSession,
		models.WorkflowInitialSessionSnapshot{SessionID: snapshotSessionID},
	))
}

func validateUnroutedInitialTarget(
	t *testing.T,
	fixture *freshStartWorkflowFixture,
	task *models.Task,
) (ceilingEntryDisposition, string, error) {
	binding := initialTargetBinding(t, fixture)
	return fixture.svc.validateUnroutedWorkflowEntry(
		fixture.ctx, task, models.CeilingDeferral{Payload: map[string]interface{}{}}, binding, false,
	)
}

func initialTargetBinding(
	t *testing.T,
	fixture *freshStartWorkflowFixture,
) models.CeilingWorkflowEntryBinding {
	t.Helper()
	step := fixture.stepGetter.steps[fixture.stepID]
	entryIdentity, err := fixture.svc.readWorkflowEntryIdentity(fixture.ctx, fixture.taskID)
	require.NoError(t, err)
	return models.CeilingWorkflowEntryBinding{
		WorkflowID:        step.WorkflowID,
		DestinationStepID: step.ID,
		RouteOperationID: workflowSessionRouteID(
			fixture.taskID,
			step.ID,
			entryIdentity,
			step.SessionTarget,
			fixture.svc.resolveStepProfileSessionStartPolicy(step),
		),
		EntryIdentity: entryIdentity,
	}
}
