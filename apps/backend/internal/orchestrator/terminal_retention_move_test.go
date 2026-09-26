package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestSilentRetainedTerminalMoveDoesNotCreateSessionOrReopenTask(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	source := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a"}
	done := &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Done", Position: 1, Prompt: "cleanup prompt",
		AgentProfileID: "profile-b", ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
		CompleteTaskOnEnter: true,
		Events:              wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	fixture.stepGetter.steps[source.ID] = source
	fixture.stepGetter.steps[done.ID] = done
	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = done.ID
	task.State = v1.TaskStateCompleted
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))
	setTerminalRetentionForOrchestratorTest(t, ctx, fixture.repo, task)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))
	before, err := fixture.repo.ListTaskSessions(ctx, task.ID)
	require.NoError(t, err)

	entry := workflowmove.OverlayStep(done, &workflowmove.EntryOptions{SkipStepPrompt: true})
	require.True(t, fixture.svc.silentRetainedTerminalEntry(ctx, task.ID, entry))
	require.False(t, fixture.svc.silentRetainedTerminalEntry(ctx, task.ID, done))
	require.False(t, fixture.svc.silentRetainedTerminalEntry(ctx, task.ID,
		workflowmove.OverlayStep(done, &workflowmove.EntryOptions{SkipStepPrompt: true, ResetContext: true})))
	require.NoError(t, fixture.svc.processStepExitAndEnterWithSteps(
		ctx, task.ID, session, source, entry, source.ID, done.ID, task.Description, false, 0, nil,
	))
	// A manual move guarded by a route effect reaches a separate preparation path.
	require.NoError(t, fixture.svc.processExitEnterForClaimedEffect(
		ctx, noOpRouteEffectClaim(), task.ID, session, source, entry,
		source.ID, done.ID, task.Description, 0,
	))
	// A duplicate delivery after the card is already Done must remain a no-op.
	require.NoError(t, fixture.svc.processStepExitAndEnterWithSteps(
		ctx, task.ID, session, source, entry, source.ID, done.ID, task.Description, false, 0, nil,
	))

	after, err := fixture.repo.ListTaskSessions(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, after, len(before), "silent Done entry must not prepare a profile session")
	current, err := fixture.repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.True(t, current.IsPrimary)
	require.Equal(t, models.TaskSessionStateWaitingForInput, current.State)
	require.Equal(t, "env-1", current.TaskEnvironmentID)
	stored, err := fixture.repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, stored.State)
	require.True(t, models.IsTerminalRetentionHeld(stored.Metadata))
}

// TestFinalizeStepEnter_SilentRetainedTerminalEntryLeavesSessionUntouched
// covers the finalize-only entry path used after queue promotion. A retained
// terminal card with no entry work must not clear a source session's review
// state before processOnEnter recognizes that the entry is suppressed.
func TestFinalizeStepEnter_SilentRetainedTerminalEntryLeavesSessionUntouched(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	source := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 0, AgentProfileID: "profile-a"}
	done := &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Done", Position: 1,
		AgentProfileID: "profile-b", ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
		CompleteTaskOnEnter: true,
	}
	fixture.stepGetter.steps[source.ID] = source
	fixture.stepGetter.steps[done.ID] = done

	task, err := fixture.repo.GetTask(ctx, "t1")
	require.NoError(t, err)
	task.WorkflowStepID = done.ID
	task.State = v1.TaskStateCompleted
	require.NoError(t, fixture.repo.UpdateTask(ctx, task))
	setTerminalRetentionForOrchestratorTest(t, ctx, fixture.repo, task)

	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	session.ReviewStatus = "approved"
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))

	require.NoError(t, fixture.svc.finalizeStepEnter(
		ctx, task.ID, session.ID, done, task.Description, true, source,
	))

	stored, err := fixture.repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.ReviewStatus("approved"), stored.ReviewStatus)
}

func TestDeferredSilentRetainedTerminalMovePreservesRunningSession(t *testing.T) {
	sc := buildPendingMoveScenarioWithQueue(t, true)
	done := sc.stepGetter.steps[stepReviewedID]
	done.Name = "Done"
	done.Prompt = "terminal prompt"
	done.AgentProfileID = profileImpl
	done.ProfileSessionStartPolicy = models.WorkflowProfileSessionStartPolicyNew
	done.CompleteTaskOnEnter = true
	done.Events.OnEnter = []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	require.NoError(t, err)
	setTerminalRetentionForOrchestratorTest(t, sc.ctx, sc.repo, task)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, session.State)
	require.True(t, session.IsPrimary)
	before, err := sc.repo.ListTaskSessions(sc.ctx, task.ID)
	require.NoError(t, err)
	move := &messagequeue.PendingMove{
		SessionIncarnationID: session.QueueIncarnationID,
		TaskID:               task.ID, WorkflowID: task.WorkflowID, WorkflowStepID: done.ID,
		EntryOptions: &workflowmove.EntryOptions{SkipStepPrompt: true},
	}
	require.NoError(t, sc.svc.messageQueue.SetPendingMove(sc.ctx, session.ID, move))
	move, found, err := sc.svc.messageQueue.GetPendingMoveWithError(sc.ctx, session.ID)
	require.NoError(t, err)
	require.True(t, found)
	sc.svc.applyPendingMove(sc.ctx, task.ID, session.ID, session, move)

	require.Eventually(t, func() bool {
		stored, loadErr := sc.repo.GetTask(sc.ctx, task.ID)
		return loadErr == nil && stored.WorkflowStepID == done.ID && stored.State == v1.TaskStateCompleted
	}, time.Second, 10*time.Millisecond)
	require.Never(t, func() bool {
		sessions, listErr := sc.repo.ListTaskSessions(sc.ctx, task.ID)
		if listErr != nil || len(sessions) != len(before) {
			return true
		}
		current, loadErr := sc.repo.GetTaskSession(sc.ctx, session.ID)
		return loadErr != nil || !current.IsPrimary || current.TaskEnvironmentID != session.TaskEnvironmentID
	}, 200*time.Millisecond, 10*time.Millisecond)
	stored, err := sc.repo.GetTask(sc.ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, stored.State)
	require.True(t, models.IsTerminalRetentionHeld(stored.Metadata))
}

func setTerminalRetentionForOrchestratorTest(
	t *testing.T,
	ctx context.Context,
	repo interface {
		UpdateTaskTerminalRetentionIfParent(context.Context, string, string, string, bool) (bool, error)
	},
	task *models.Task,
) {
	t.Helper()
	changed, err := repo.UpdateTaskTerminalRetentionIfParent(ctx, task.ID, task.ParentID, task.WorkspaceID, true)
	require.NoError(t, err)
	require.True(t, changed)
}
