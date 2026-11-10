package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/engine"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestProcessOnChildrenCompleted_QueuesAutoStartWhileParentIsRunning pins the
// busy-parent case of on_children_completed -> auto_start_agent: when the last
// child finishes while the parent is still in its own turn, the step prompt is
// queued (QueueIfBusy) and delivered exactly once after that turn, even though
// the completion is delivered twice (task state change and terminal step move).
func TestProcessOnChildrenCompleted_QueuesAutoStartWhileParentIsRunning(t *testing.T) {
	ctx := context.Background()
	repo, svc, agentMgr := setupBusyParentChildrenCompleted(t)

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for delivery := 0; delivery < 2; delivery++ {
		require.False(t, svc.processOnChildrenCompleted(callCtx, busyParentID),
			"auto_start_agent on the same step is not a transition")
	}
	require.Empty(t, agentMgr.capturedPrompts, "a busy parent must not be prompted mid-turn")
	requireBusyParentCompletionApplied(t, repo, svc)

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one wake must be queued for the parent")
	require.Contains(t, entries[0].Content, busyParentStepPrompt)

	// The parent's turn ends: the ordinary queue drain delivers the wake once.
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }
	require.True(t, svc.drainQueuedMessageForPromptableSessionLocked(ctx, busyParentSessionID))
	waitForBusyParentWake(t, done)
	requireSingleBusyParentWake(t, agentMgr)
}

// TestProcessOnChildrenCompleted_DeliversWakeWhenTurnEndsWhileQueueing covers
// a parent turn that ends after the auto-start read the session as running but
// before the prompt reached the queue: that turn's drain found nothing, so the
// callback must deliver the queued prompt itself.
func TestProcessOnChildrenCompleted_DeliversWakeWhenTurnEndsWhileQueueing(t *testing.T) {
	ctx := context.Background()
	repo, svc, agentMgr := setupBusyParentChildrenCompleted(t)
	svc.afterBusyAutoStartSessionRead = func() {
		require.NoError(t, repo.UpdateTaskSessionState(
			ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	}
	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	require.False(t, svc.processOnChildrenCompleted(callCtx, busyParentID))
	waitForBusyParentWake(t, done)
	requireSingleBusyParentWake(t, agentMgr)
	requireBusyParentCompletionApplied(t, repo, svc)

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Empty(t, entries, "the delivered wake must leave the queue")
}

// TestAutoStartAgentCallback_RetriesBusyRejectionAfterTurnEnds covers a
// launch refused as busy whose turn ends before the callback re-reads the
// session: the prompt never reached the agent, so the callback launches again
// instead of reporting the rejection.
func TestAutoStartAgentCallback_RetriesBusyRejectionAfterTurnEnds(t *testing.T) {
	ctx := context.Background()
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))

	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(context.Context, engine.ActionInput) error {
		launches++
		if launches == 1 {
			return fmt.Errorf("failed to prompt session: %w", ErrAgentPromptInProgress)
		}
		return nil
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	require.Equal(t, 2, launches, "a busy rejection on an idle session must be retried")
}

// TestAutoStartAgentCallback_QueuesBusyRejectionWhenTurnStarted covers a
// launch refused as busy because a turn started after the first state read:
// the retry finds the session mid-turn and queues the prompt.
func TestAutoStartAgentCallback_QueuesBusyRejectionWhenTurnStarted(t *testing.T) {
	ctx := context.Background()
	repo, svc, agentMgr := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))

	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(context.Context, engine.ActionInput) error {
		launches++
		require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateRunning, ""))
		return fmt.Errorf("failed to prompt session: %w", ErrAgentPromptInProgress)
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	require.Equal(t, 1, launches)
	require.Empty(t, agentMgr.capturedPrompts)
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Content, busyParentStepPrompt)
}

// TestAutoStartAgentCallback_QueuesAfterFinalBusyRejection covers a turn that
// starts during the last allowed launch: the prompt is queued rather than the
// rejection being returned.
func TestAutoStartAgentCallback_QueuesAfterFinalBusyRejection(t *testing.T) {
	ctx := context.Background()
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))

	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(context.Context, engine.ActionInput) error {
		launches++
		if launches == maxBusyAutoStartAttempts {
			require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateRunning, ""))
		}
		return fmt.Errorf("failed to prompt session: %w", ErrAgentPromptInProgress)
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	require.Equal(t, maxBusyAutoStartAttempts, launches)
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

// TestAutoStartAgentCallback_StopsAfterBoundedBusyRejections keeps an idle
// session that keeps refusing prompts as busy from retrying without bound.
func TestAutoStartAgentCallback_StopsAfterBoundedBusyRejections(t *testing.T) {
	ctx := context.Background()
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))

	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(context.Context, engine.ActionInput) error {
		launches++
		return fmt.Errorf("failed to prompt session: %w", ErrAgentPromptInProgress)
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.ErrorIs(t, err, ErrAgentPromptInProgress)
	require.Equal(t, maxBusyAutoStartAttempts, launches)
}

// TestAutoStartAgentCallback_KeepsHandoffClaimedByRefusedLaunch covers a
// launch that claims the step handoff and is then refused as busy: the queued
// prompt that replaces it still carries that handoff.
func TestAutoStartAgentCallback_KeepsHandoffClaimedByRefusedLaunch(t *testing.T) {
	ctx := context.Background()
	const handoff = "notes from the previous step"
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))
	seedHandoffCarryToken(t, repo, busyParentID, "step_coordinate", handoff, "stamp-busy-retry")

	cb := &autoStartAgentCallback{svc: svc, launch: func(launchCtx context.Context, in engine.ActionInput) error {
		require.Equal(t, handoff, svc.claimStepHandoffTextForLaunch(launchCtx, in.State.TaskID, in.Step.ID))
		require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateRunning, ""))
		return fmt.Errorf("failed to prompt session: %w", ErrAgentPromptInProgress)
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, handoff, stepHandoffFromQueuedMetadata(entries[0].Metadata),
		"the handoff claimed by the refused launch must ride on the queued prompt")
}

// TestAutoStartAgentCallback_ReturnsNonBusyLaunchError keeps a launch failure
// that may follow prompt acceptance from being retried or queued.
func TestAutoStartAgentCallback_ReturnsNonBusyLaunchError(t *testing.T) {
	ctx := context.Background()
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateWaitingForInput, ""))

	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(context.Context, engine.ActionInput) error {
		launches++
		require.NoError(t, repo.UpdateTaskSessionState(ctx, busyParentSessionID, models.TaskSessionStateRunning, ""))
		return errors.New("agent stream disconnected: EOF")
	}}
	_, err := cb.Execute(ctx, busyParentAutoStartInput())
	require.Error(t, err)
	require.Equal(t, 1, launches)
	entries, _, err := svc.messageQueue.SnapshotSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	require.Empty(t, entries, "a possibly delivered prompt must not be queued again")
}

func busyParentAutoStartInput() engine.ActionInput {
	return engine.ActionInput{
		Trigger: engine.TriggerOnChildrenCompleted,
		State:   engine.MachineState{TaskID: busyParentID, SessionID: busyParentSessionID, CurrentStepID: "step_coordinate"},
		Step:    engine.StepSpec{ID: "step_coordinate", WorkflowID: "wf1", Name: "Coordinate"},
		Action: engine.Action{
			Kind:           engine.ActionAutoStartAgent,
			AutoStartAgent: &engine.AutoStartAgentAction{QueueIfBusy: true},
		},
	}
}

const (
	busyParentID         = "parent"
	busyParentSessionID  = "parent-session"
	busyParentStepPrompt = "Review the finished subtasks."
)

// setupBusyParentChildrenCompleted seeds a parent whose session is running a
// turn, on a step whose on_children_completed starts the agent, and one
// completed child.
func setupBusyParentChildrenCompleted(t *testing.T) (*sqliterepo.Repository, *Service, *mockAgentManager) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, busyParentID, busyParentSessionID, "step_coordinate")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_coordinate"] = &wfmodels.WorkflowStep{
		ID: "step_coordinate", WorkflowID: "wf1", Name: "Coordinate", Position: 0, Prompt: busyParentStepPrompt,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionAutoStartAgent}},
		},
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	seedExecutorRunning(t, repo, busyParentSessionID, busyParentID, "exec-parent")
	svc.messageCreator = &mockMessageCreator{}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child", WorkflowID: "wf1", Title: "Child",
		State: v1.TaskStateCompleted, ParentID: busyParentID, CreatedAt: now, UpdatedAt: now,
	})
	return repo, svc, agentMgr
}

func requireBusyParentCompletionApplied(t *testing.T, repo *sqliterepo.Repository, svc *Service) {
	t.Helper()
	ctx := context.Background()
	rows, err := repo.ListChildCompletionRows(ctx, busyParentID)
	require.NoError(t, err)
	applied, err := svc.workflowStore.IsOperationApplied(ctx, childCompletionOperationID(busyParentID, rows))
	require.NoError(t, err)
	require.True(t, applied, "the queued wake must settle the completion operation")
}

func waitForBusyParentWake(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the queued wake to dispatch")
	}
}

func requireSingleBusyParentWake(t *testing.T, agentMgr *mockAgentManager) {
	t.Helper()
	require.Len(t, agentMgr.capturedPrompts, 1)
	require.True(t, strings.Contains(agentMgr.capturedPrompts[0], busyParentStepPrompt),
		"prompt = %q", agentMgr.capturedPrompts[0])
}
