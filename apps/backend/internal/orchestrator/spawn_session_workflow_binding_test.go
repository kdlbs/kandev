package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// spawnSourceStep registers the direct-profile source step targeted by the later
// workflow entry, and returns its binding key.
func spawnSourceStep(t *testing.T, fixture *profileSwitchFixture) (*wfmodels.WorkflowStep, string) {
	t.Helper()
	source := &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a",
	}
	fixture.stepGetter.steps[source.ID] = source
	return source, workflowSessionBindingTargetKey(source.ID)
}

func spawnLaunchRequest() *LaunchSessionRequest {
	return &LaunchSessionRequest{
		TaskID:                        "t1",
		Intent:                        IntentStart,
		AgentProfileID:                "profile-a",
		ProfileExplicit:               true,
		ExecutorID:                    "exec-local",
		ExecutorProfileID:             "ep1",
		Prompt:                        "prepare the helper work",
		PreserveWorkflowSourceBinding: true,
	}
}

// spawnBindingFixture is the shared precondition for the source-binding
// regressions. The saved binding carries the same workflow entry operation the
// launch resolves, and its timestamp is deliberately older than the task row so
// every real SQL scheduling write is deterministically newer. That makes the
// overwrite path reachable without a wall-clock sleep; the positive control in
// TestLaunchSpawnPreservesWorkflowSourceBinding proves the condition is real.
type spawnBindingFixture struct {
	*profileSwitchFixture
	source        *wfmodels.WorkflowStep
	targetKey     string
	bindingBefore *models.WorkflowSessionBinding
	entryIdentity string
	taskBefore    *models.Task
}

func newSpawnBindingFixture(t *testing.T) *spawnBindingFixture {
	t.Helper()
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	source, targetKey := spawnSourceStep(t, fixture)

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, task)
	entryIdentity := fixture.svc.workflowEntryIdentity(ctx, "t1")
	_, err = fixture.repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID:         "t1",
		TargetKey:      targetKey,
		WorkflowID:     source.WorkflowID,
		AgentProfileID: source.AgentProfileID,
		SessionID:      fixture.current.ID,
		OperationID:    workflowSourceBindingOperationID(entryIdentity),
		UpdatedAt:      task.UpdatedAt.Add(-time.Hour),
	})
	require.NoError(t, err)
	bindingBefore, err := fixture.repo.GetWorkflowSessionBinding(ctx, "t1", targetKey)
	require.NoError(t, err)
	require.NotNil(t, bindingBefore)
	require.True(t, task.UpdatedAt.After(bindingBefore.UpdatedAt))

	// beginTaskScheduling and finishTaskScheduling must reach the real SQL task
	// row, not only the in-memory mock, or the timestamp condition is unproven.
	fixture.svc.taskRepo = orphanSQLTaskStates{
		mockTaskRepo: fixture.svc.taskRepo.(*mockTaskRepo),
		repo:         fixture.repo,
	}
	return &spawnBindingFixture{
		profileSwitchFixture: fixture,
		source:               source,
		targetKey:            targetKey,
		bindingBefore:        bindingBefore,
		entryIdentity:        entryIdentity,
		taskBefore:           task,
	}
}

// requireLaunchPrecondition proves the launch or replay performed a real SQL
// task write that is newer than the saved binding while staying in the same
// workflow entry. Without it an equal-timestamp write could reject the rebind on
// its own and the preservation assertion would pass vacuously.
func (f *spawnBindingFixture) requireLaunchPrecondition(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	taskAfter, err := f.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	require.True(t, taskAfter.UpdatedAt.After(f.taskBefore.UpdatedAt),
		"the launch must perform a newer SQL task write")
	require.True(t, taskAfter.UpdatedAt.After(f.bindingBefore.UpdatedAt),
		"the task timestamp used by the launch must be newer than the saved binding")

	entryIdentityAfter := f.svc.workflowEntryIdentity(ctx, "t1")
	require.Equal(t, f.entryIdentity, entryIdentityAfter,
		"a preservation claim requires the same workflow entry")
	require.Equal(t, workflowSourceBindingOperationID(f.entryIdentity), f.bindingBefore.OperationID)
}

func (f *spawnBindingFixture) requireBindingUnchanged(t *testing.T) {
	t.Helper()
	after, err := f.repo.GetWorkflowSessionBinding(context.Background(), "t1", f.targetKey)
	require.NoError(t, err)
	require.NotNil(t, after)
	require.Equal(t, *f.bindingBefore, *after,
		"spawn must not rebind the source step to the helper session")
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4, AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
func TestLaunchSpawnPreservesWorkflowSourceBinding(t *testing.T) {
	ctx := context.Background()
	fixture := newSpawnBindingFixture(t)

	response, err := fixture.svc.LaunchSession(ctx, spawnLaunchRequest())
	require.NoError(t, err)
	require.NotEmpty(t, response.SessionID)
	require.NotEqual(t, fixture.current.ID, response.SessionID)

	fixture.requireLaunchPrecondition(t)
	fixture.requireBindingUnchanged(t)

	// Positive control: with the same timestamp and entry setup, an ordinary
	// start with the policy off must reach and replace the saved binding. If
	// the timestamp condition above ever stopped holding, this control would
	// fail instead of letting the preservation assertion pass vacuously.
	t.Run("control: ordinary start rebinds the source step", func(t *testing.T) {
		control := newSpawnBindingFixture(t)
		request := spawnLaunchRequest()
		request.PreserveWorkflowSourceBinding = false
		controlResponse, err := control.svc.LaunchSession(ctx, request)
		require.NoError(t, err)
		require.NotEmpty(t, controlResponse.SessionID)

		control.requireLaunchPrecondition(t)
		after, err := control.repo.GetWorkflowSessionBinding(ctx, "t1", control.targetKey)
		require.NoError(t, err)
		require.NotNil(t, after)
		require.Equal(t, controlResponse.SessionID, after.SessionID,
			"the control must prove the overwrite path is reachable under this timestamp setup")
		require.True(t, after.UpdatedAt.After(control.bindingBefore.UpdatedAt))
	})
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
func TestLaunchSpawnDoesNotCreateWorkflowSourceBinding(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	source, targetKey := spawnSourceStep(t, fixture)

	response, err := fixture.svc.LaunchSession(ctx, spawnLaunchRequest())
	require.NoError(t, err)
	require.NotEmpty(t, response.SessionID)

	binding, err := fixture.repo.GetWorkflowSessionBinding(ctx, "t1", targetKey)
	require.NoError(t, err)
	require.Nil(t, binding, "a helper session must not create a source-step binding")

	target := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Position: 1,
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: source.ID},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	}
	helper, err := fixture.repo.GetTaskSession(ctx, response.SessionID)
	require.NoError(t, err)
	fallback, _, err := fixture.svc.prepareExplicitWorkflowSession(ctx, "t1", helper, target, source)
	require.NoError(t, err)
	require.NotNil(t, fallback)
	require.NotEqual(t, helper.ID, fallback.ID,
		"an absent source binding must use a fresh fallback conversation, not the helper")
	require.NotEqual(t, fixture.current.ID, fallback.ID)
	require.Equal(t, "profile-a", fallback.AgentProfileID)
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4
func TestManualInitialStartRecordsWorkflowSourceBinding(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	_, targetKey := spawnSourceStep(t, fixture)

	request := spawnLaunchRequest()
	request.PreserveWorkflowSourceBinding = false
	response, err := fixture.svc.LaunchSession(ctx, request)
	require.NoError(t, err)
	require.NotEmpty(t, response.SessionID)

	binding, err := fixture.repo.GetWorkflowSessionBinding(ctx, "t1", targetKey)
	require.NoError(t, err)
	require.NotNil(t, binding)
	require.Equal(t, response.SessionID, binding.SessionID,
		"an ordinary start without a supplied step still records its recipient")
}

// @covers AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.4
func TestSpawnThenEnterSourceTargetUsesBoundSession(t *testing.T) {
	ctx := context.Background()
	fixture := newSpawnBindingFixture(t)
	source := fixture.source
	const marker = "later source target marker"
	target := &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Position: 1, AgentProfileID: "profile-a",
		Prompt:                    marker,
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: source.ID},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{
			Type: wfmodels.OnEnterAutoStartAgent,
		}}},
	}
	fixture.stepGetter.steps[target.ID] = target

	response, err := fixture.svc.LaunchSession(ctx, spawnLaunchRequest())
	require.NoError(t, err)
	require.NotEmpty(t, response.SessionID)
	helper, err := fixture.repo.GetTaskSession(ctx, response.SessionID)
	require.NoError(t, err)
	require.NotEqual(t, fixture.current.ID, helper.ID)
	seedExecutorRunning(t, fixture.repo, helper.ID, "t1", "execution-helper")

	fixture.requireLaunchPrecondition(t)
	fixture.requireBindingUnchanged(t)

	// Both same-profile conversations are idle when the later entry runs, so
	// the entry dispatches its prompt instead of queueing it.
	for _, session := range []*models.TaskSession{fixture.current, helper} {
		session.State = models.TaskSessionStateWaitingForInput
		require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))
	}

	delivered := make(chan promptCall, 1)
	fixture.agentMgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	fixture.agentMgr.isAgentRunning = true
	fixture.agentMgr.promptAgentFunc = func(
		_ context.Context, executionID, prompt string, _ []v1.MessageAttachment, _ bool,
	) (*executor.PromptResult, error) {
		if strings.Contains(prompt, marker) {
			select {
			case delivered <- promptCall{ExecutionID: executionID, Prompt: prompt}:
			default:
			}
		}
		return &executor.PromptResult{}, nil
	}

	resolution, err := fixture.svc.resolveWorkflowSessionTarget(ctx, "t1", target)
	require.NoError(t, err)
	require.NotNil(t, resolution.session)
	require.Equal(t, fixture.current.ID, resolution.session.ID,
		"the live same-profile helper must not replace the source conversation")

	fixture.svc.processOnEnter(ctx, "t1", fixture.current, target, "task description", 0, source)

	select {
	case call := <-delivered:
		require.Equal(t, "execution-a", call.ExecutionID,
			"the later explicit target must send its prompt to the original source conversation")
	case <-time.After(2 * time.Second):
		t.Fatal("later explicit source target did not deliver a prompt")
	}

	fixture.requireBindingUnchanged(t)
}
