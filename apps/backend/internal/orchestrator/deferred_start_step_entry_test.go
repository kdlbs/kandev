package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	deferredStartHoldStep  = "step-deferred-hold"
	deferredStartAgentStep = "step-deferred-agent"
)

// newDeferredStartStepEntryFixture seeds a task whose deferred start is not a
// dependency chain, so only a step entry can consume it.
func newDeferredStartStepEntryFixture(t *testing.T) (*Service, *sqliterepo.Repository, *launchCounter, *mockStepGetter) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	delete(task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{}), models.DeferredLaunchStartWhenUnblockedKey)
	require.NoError(t, repo.UpdateTask(ctx, task))

	counter := newLaunchCounter()
	svc := newDeferredLaunchTestService(t, repo, counter)
	steps := newMockStepGetter()
	steps.steps[deferredStartHoldStep] = &wfmodels.WorkflowStep{ID: deferredStartHoldStep, WorkflowID: "wf1", Name: "Hold"}
	steps.steps[deferredStartAgentStep] = &wfmodels.WorkflowStep{
		ID: deferredStartAgentStep, WorkflowID: "wf1", Name: "Work",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	svc.workflowStepGetter = steps
	return svc, repo, counter, steps
}

func hasDeferredStart(t *testing.T, svc *Service) bool {
	t.Helper()
	task, err := svc.repo.GetTask(context.Background(), deferredChainTaskID)
	require.NoError(t, err)
	_, ok := task.Metadata[models.MetaKeyDeferredLaunch]
	return ok
}

// AC-TASKS-DEFERRED-START-STEP-ENTRY-001.1, 001.2: a move into an agentless
// step keeps the deferred start, and the next agent step consumes it once.
func TestTaskMovedIntoAgentlessStepKeepsDeferredStart(t *testing.T) {
	ctx := context.Background()
	svc, repo, counter, _ := newDeferredStartStepEntryFixture(t)

	svc.autoStartTaskForStep(ctx, deferredChainTaskID, deferredStartHoldStep, events.TaskMoved, 0, false)
	require.False(t, counter.awaitLaunch(0), "a move into an agentless step launched the deferred start")
	require.True(t, hasDeferredStart(t, svc), "a move into an agentless step consumed the deferred start")

	svc.autoStartTaskForStep(ctx, deferredChainTaskID, deferredStartAgentStep, events.TaskMoved, 0, false)
	require.True(t, counter.awaitLaunch(0), "the next agent step did not launch the kept deferred start")
	awaitLaunchedSession(t, repo, deferredChainTaskID)
	require.False(t, hasDeferredStart(t, svc), "the agent step did not consume the deferred start")
	require.Equal(t, 1, counter.count())
}

// AC-TASKS-DEFERRED-START-STEP-ENTRY-001.3: a step that cannot be read is no
// evidence of an agentless step, so the move still attempts the deferred start.
func TestTaskMovedIntoUnreadableStepStillAttemptsDeferredStart(t *testing.T) {
	ctx := context.Background()
	svc, repo, counter, steps := newDeferredStartStepEntryFixture(t)
	steps.getStepFunc = func(context.Context, string) (*wfmodels.WorkflowStep, error) {
		return nil, errors.New("step store unavailable")
	}

	svc.autoStartTaskForStep(ctx, deferredChainTaskID, deferredStartHoldStep, events.TaskMoved, 0, false)
	require.True(t, counter.awaitLaunch(0), "an unreadable step dropped the deferred start attempt")
	awaitLaunchedSession(t, repo, deferredChainTaskID)
}

// AC-TASKS-DEFERRED-START-STEP-ENTRY-001.1: handleTaskMoved starts a task
// without a session through the already loaded destination step. That path
// keeps the deferred start too.
func TestLoadedStepTaskMovedIntoAgentlessStepKeepsDeferredStart(t *testing.T) {
	ctx := context.Background()
	svc, repo, counter, steps := newDeferredStartStepEntryFixture(t)
	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)

	svc.autoStartTaskForLoadedStep(ctx, task, steps.steps[deferredStartHoldStep], events.TaskMoved, false, 0, false)

	require.False(t, counter.awaitLaunch(0), "a loaded-step move into an agentless step launched the deferred start")
	require.True(t, hasDeferredStart(t, svc), "a loaded-step move into an agentless step consumed the deferred start")
}
