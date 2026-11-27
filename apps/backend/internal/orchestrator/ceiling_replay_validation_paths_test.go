package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SESSION-CEILING-001.11
func TestCeilingSweepValidationFailureAcrossSessionLaunchPaths(t *testing.T) {
	for _, kind := range []models.CeilingLaunchKind{
		models.CeilingLaunchStartCreated, models.CeilingLaunchResume,
		models.CeilingLaunchPromptEnsure, models.CeilingLaunchWorkflowStepEnsure,
		models.CeilingLaunchQueueDrainEnsure, models.CeilingLaunchDynamicRelaunch,
	} {
		t.Run(string(kind), func(t *testing.T) {
			cause := &routingerr.BuiltCommandValidationError{Command: "continue", Cause: errors.New("agent command cannot be empty")}
			f := newSessionFailureReplayFixture(t, kind, fmt.Errorf("launch agent: %w", cause))
			f.svc.drainDeferredCeilingLaunches(context.Background())
			require.Equal(t, 1, f.launchCount(), "the injected failure must be reached through the real dispatch path")
			require.False(t, f.ceilingDeferred(t), "a typed command failure must terminate the replay")
			f.svc.drainDeferredCeilingLaunches(context.Background())
			require.Equal(t, 1, f.launchCount(), "the invalid command must not be dispatched again")
			entries := f.logs.FilterMessage("dropping a ceiling-deferred launch").All()
			require.NotEmpty(t, entries)
			require.Contains(t, entries[0].ContextMap()["detail"], cause.Error())
		})
	}
}

// @covers AC-AGENTS-SESSION-CEILING-001.5
func TestCeilingSweepEnsureRetainsTransientLaunchFailure(t *testing.T) {
	for _, kind := range []models.CeilingLaunchKind{
		models.CeilingLaunchPromptEnsure, models.CeilingLaunchWorkflowStepEnsure,
		models.CeilingLaunchQueueDrainEnsure, models.CeilingLaunchDynamicRelaunch,
	} {
		t.Run(string(kind), func(t *testing.T) {
			f := newSessionFailureReplayFixture(t, kind, errors.New("provider temporarily unavailable"))
			f.svc.drainDeferredCeilingLaunches(context.Background())
			require.Equal(t, 1, f.launchCount())
			require.True(t, f.ceilingDeferred(t))
			entries := f.logs.FilterMessage(ceilingReplayRetainedFailureLog).All()
			require.NotEmpty(t, entries)
			require.Contains(t, entries[0].ContextMap()["error"], "provider temporarily unavailable")
		})
	}
}

func TestDynamicCreatedRelaunchCancellationWinsOverLaunchFailure(t *testing.T) {
	f := newSessionFailureReplayFixture(t, models.CeilingLaunchDynamicRelaunch, errors.New("unused launch failure"))
	ctx := context.Background()
	sessionID := f.taskID + "-session"
	session, err := f.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	cause := &routingerr.BuiltCommandValidationError{Command: "agent", Cause: errors.New("agent command cannot be empty")}
	launches := 0
	launchStarted := make(chan struct{})
	releaseLaunch := make(chan struct{})
	f.svc.agentManager.(*mockAgentManager).launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches++
		close(launchStarted)
		<-releaseLaunch
		return nil, cause
	}
	type launchResult struct {
		execution *executor.TaskExecution
		err       error
	}
	result := make(chan launchResult, 1)
	go func() {
		execution, err := f.svc.startDynamicRelaunchCreatedSession(ctx, f.taskID, session, capturedPrompt{text: "retry the task"})
		result <- launchResult{execution: execution, err: err}
	}()
	select {
	case <-launchStarted:
	case <-time.After(5 * time.Second):
		close(releaseLaunch)
		t.Fatal("created dynamic launch did not reach the ownership barrier")
	}
	f.svc.invalidateResumeAttempt(sessionID)
	close(releaseLaunch)
	var completed launchResult
	select {
	case completed = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("invalidated created dynamic launch did not return")
	}
	require.Equal(t, 1, launches, "the competing invalidation must occur inside the real launch path")
	require.Nil(t, completed.execution)
	require.ErrorIs(t, completed.err, ErrResumeAttemptCancelled)
	require.False(t, routingerr.IsBuiltCommandValidationError(completed.err), "a cancelled owner must not classify a competing launch failure")
	registry := f.svc.resumeAttemptStore()
	registry.mu.Lock()
	active := registry.attempts[sessionID]
	registry.mu.Unlock()
	require.Nil(t, active, "failed startup must release its attempt")
}

func newSessionFailureReplayFixture(t *testing.T, kind models.CeilingLaunchKind, launchErr error) *launchFailureReplayFixture {
	t.Helper()
	f := newLaunchFailureReplayFixture(t, "session-replay-"+string(kind), launchErr)
	ctx := context.Background()
	sessionID := f.taskID + "-session"
	now := time.Now().UTC()
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: f.taskID, AgentProfileID: "profile-1",
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}))
	if kind == models.CeilingLaunchStartCreated {
		require.NoError(t, f.repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateCreated, ""))
	} else if kind != models.CeilingLaunchDynamicRelaunch {
		seedExecutorRunning(t, f.repo, sessionID, f.taskID, "old-execution")
	}
	// Force the persisted-row cold-resume path, rather than presenting the old
	// row as an in-memory prompt-ready execution.
	f.svc.agentManager.(*mockAgentManager).getExecutionIDForSessionFunc = func(context.Context, string) (string, error) {
		return "", errors.New("execution is no longer registered")
	}
	f.svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})
	payload := map[string]interface{}{
		metaKeySessionID: sessionID, metaKeyPrompt: "replay the task",
		metaKeyAgentProfileID: "profile-1", "execution_profile_id": "profile-1",
		metaKeyTaskID: f.taskID, "agent_execution_id": "old-execution",
	}
	if kind == models.CeilingLaunchWorkflowStepEnsure {
		payload[metaKeyWorkflowStepID] = "step-auto"
	}
	deferral := models.CeilingDeferral{Kind: kind, Payload: payload,
		Origin: string(launchOriginAutomatic), ReasonCode: ceilingReasonRefused, QueuedAt: now}
	raw, prior, err := f.repo.GetTaskDeferredLaunch(ctx, f.taskID)
	require.NoError(t, err)
	updated := stripCeilingRecordKeys(raw)
	for key, value := range models.CeilingRecordKeys(deferral) {
		updated[key] = value
	}
	stored, _, err := f.repo.SetTaskDeferredLaunchIfUnchanged(ctx, f.taskID, prior, updated)
	require.NoError(t, err)
	require.True(t, stored)
	return f
}

// @covers AC-AGENTS-SESSION-CEILING-001.12
func TestCeilingSweepBoundStartPreservesFailedStepRead(t *testing.T) {
	f := newLaunchFailureReplayFixture(t, "bound-step-read", errors.New("launch must not be reached"))
	ctx := context.Background()
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	raw, prior, err := f.repo.GetTaskDeferredLaunch(ctx, task.ID)
	require.NoError(t, err)
	deferral, err := models.ReadCeilingDeferral(raw)
	require.NoError(t, err)
	binding, ok, err := f.svc.deriveCeilingEntryBinding(ctx, task, deferral.Payload, "")
	require.NoError(t, err)
	require.True(t, ok)
	deferral.Payload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(binding)
	stored, _, err := f.repo.SetTaskDeferredLaunchIfUnchanged(ctx, task.ID, prior, models.CeilingRecordKeys(deferral))
	require.NoError(t, err)
	require.True(t, stored)
	steps := f.svc.workflowStepGetter.(*mockStepGetter)
	steps.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
		return nil, errors.New("workflow store temporarily unavailable")
	}
	f.svc.drainDeferredCeilingLaunches(ctx)
	require.True(t, f.ceilingDeferred(t), "a failed eligibility read must preserve an already bound start")
	require.Zero(t, f.launchCount())
}
