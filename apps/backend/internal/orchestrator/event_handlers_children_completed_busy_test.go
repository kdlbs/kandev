package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
