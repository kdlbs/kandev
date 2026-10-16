package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSchedulingOrphanPreparedDestinationKeepsAcceptedTransition(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	const taskID, selectedID, preparedID = "orphan-prepared", "selected-source", "prepared-destination"
	f.seedTask(t, taskID, orphanHoldStep, nil)
	seedOrphanResumableExecution(t, f, taskID, selectedID)
	f.seedSession(t, taskID, preparedID, models.TaskSessionStateIdle)
	task := orphanTaskWithEntry(t, f, taskID)
	task.WorkflowStepID = orphanAutoStep
	require.NoError(t, f.repo.UpdateTask(t.Context(), task))
	task = orphanTaskWithEntry(t, f, taskID)
	acceptedEntryID := task.WorkflowStepTransitionID
	require.Positive(t, acceptedEntryID)
	steps := f.svc.workflowStepGetter.(*mockStepGetter)
	step := steps.steps[orphanAutoStep]
	route := workflowProfileSessionRoute(taskID, &models.TaskSession{ID: selectedID}, step,
		testAgentProfileID, workflowEntryIdentityForID(acceptedEntryID), f.svc.resolveStepProfileSessionStartPolicy(step))
	route.DestinationID = preparedID
	require.NoError(t, f.repo.SetTaskMetadataKey(t.Context(), taskID, models.MetaKeyWorkflowSessionRoute, route))
	task = orphanTaskWithEntry(t, f, taskID)
	selected, err := f.svc.schedulingOrphanRecoverableSession(t.Context(), taskID)
	require.NoError(t, err)
	require.Equal(t, selectedID, selected.ID)
	entryID, source := f.svc.schedulingOrphanEntryContext(t.Context(), task, selected)
	require.Equal(t, acceptedEntryID, entryID)
	require.NotNil(t, source)
	require.Equal(t, orphanHoldStep, source.ID)
	require.True(t, f.svc.workflowEntryDispatchIsCurrentForSession(t.Context(), taskID, selectedID, step, entryID))
	require.False(t, f.svc.workflowEntryDispatchIsCurrentForSession(t.Context(), taskID, selectedID, step, entryID-1))

	f.runStartupSweep(t)
	awaitOrphanEntryPrompt(t, f, selectedID)
	reloaded, err := f.repo.GetTask(t.Context(), taskID)
	require.NoError(t, err)
	committed, present := models.LoadWorkflowSessionRoute(reloaded.Metadata)
	require.True(t, present)
	require.Equal(t, workflowSessionRouteCommitted, committed.Phase)
	require.Equal(t, workflowEntryIdentityForID(acceptedEntryID), committed.EntryIdentity)
	require.Equal(t, selectedID, committed.DestinationID)
	require.Equal(t, 2, sessionCount(t, f.repo, taskID))
	require.Equal(t, 1, orphanPromptCount(f))
}

func TestSchedulingOrphanIdleSessionReceivesEntryInPlace(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	const taskID, sessionID = "orphan-idle-real", "orphan-idle-real-session"
	f.seedTask(t, taskID, orphanAutoStep, nil)
	f.seedSession(t, taskID, sessionID, models.TaskSessionStateIdle)
	// IDLE recovery retains runtime identity, just like WAITING_FOR_INPUT.
	// A missing record means the existing launch owner has not registered yet.
	now := time.Now().UTC()
	require.NoError(t, f.repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
		ID: sessionID + "-execution", TaskID: taskID, SessionID: sessionID,
		AgentExecutionID: "exec-before-restart", ResumeToken: "resume-idle",
		Resumable: true, CreatedAt: now, UpdatedAt: now,
	}))
	f.agentMgr.isAgentReadyFn = func(context.Context, string) bool { return true }

	f.runStartupSweep(t)
	require.True(t, f.launches.awaitLaunch(0), "an IDLE conversation was not resumed")
	require.Eventually(t, func() bool {
		session, err := f.repo.GetTaskSession(t.Context(), sessionID)
		return err == nil && session.State == models.TaskSessionStateStarting
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), sessionID, models.TaskSessionStateWaitingForInput, ""))
	require.Eventually(t, func() bool { return orphanPromptCount(f) == 1 }, 5*time.Second, 10*time.Millisecond)
	f.svc.orphanReentryWorkers.Wait()
	require.Equal(t, 1, sessionCount(t, f.repo, taskID))
}

func TestSchedulingOrphanCommittedRecipientNeedsNoSourceTransition(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	f.seedTask(t, "orphan-committed", orphanAutoStep, nil)
	f.seedSession(t, "orphan-committed", "committed-recipient", models.TaskSessionStateWaitingForInput)
	task := orphanTaskWithEntry(t, f, "orphan-committed")
	route := models.WorkflowSessionRoute{
		OperationID: "committed-entry", DestinationStepID: orphanAutoStep,
		DestinationID: "committed-recipient", TargetKind: "profile", Phase: workflowSessionRouteCommitted,
		EntryIdentity: workflowEntryIdentityForID(task.WorkflowStepTransitionID),
	}
	require.NoError(t, f.repo.SetTaskMetadataKey(t.Context(), task.ID, models.MetaKeyWorkflowSessionRoute, route))
	task = orphanTaskWithEntry(t, f, task.ID)
	session, err := f.repo.GetTaskSession(t.Context(), route.DestinationID)
	require.NoError(t, err)
	entryID, source := f.svc.schedulingOrphanEntryContext(t.Context(), task, session)
	require.Zero(t, entryID)
	require.Nil(t, source)
}

func TestSchedulingOrphanIdleWithoutRuntimeRecordDoesNotInventExecution(t *testing.T) {
	f := newOrphanInterruptionFixture(t)
	const taskID, sessionID = "orphan-idle-awaiting", "orphan-idle-awaiting-session"
	f.seedTask(t, taskID, orphanAutoStep, nil)
	f.seedSession(t, taskID, sessionID, models.TaskSessionStateIdle)
	f.runStartupSweep(t)
	f.svc.orphanReentryWorkers.Wait()
	require.Equal(t, 0, f.launches.count())
	require.Equal(t, 0, orphanPromptCount(f))
	require.Equal(t, 1, sessionCount(t, f.repo, taskID))
}
