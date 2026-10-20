package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
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
	const (
		parentID   = "parent"
		sessionID  = "parent-session"
		stepPrompt = "Review the finished subtasks."
	)
	repo := setupTestRepo(t)
	seedSession(t, repo, parentID, sessionID, "step_coordinate")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_coordinate"] = &wfmodels.WorkflowStep{
		ID: "step_coordinate", WorkflowID: "wf1", Name: "Coordinate", Position: 0, Prompt: stepPrompt,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionAutoStartAgent}},
		},
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	seedExecutorRunning(t, repo, sessionID, parentID, "exec-parent")
	svc.messageCreator = &mockMessageCreator{}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child", WorkflowID: "wf1", Title: "Child",
		State: v1.TaskStateCompleted, ParentID: parentID, CreatedAt: now, UpdatedAt: now,
	})

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for delivery := 0; delivery < 2; delivery++ {
		require.False(t, svc.processOnChildrenCompleted(callCtx, parentID),
			"auto_start_agent on the same step is not a transition")
	}
	require.Empty(t, agentMgr.capturedPrompts, "a busy parent must not be prompted mid-turn")

	rows, err := repo.ListChildCompletionRows(ctx, parentID)
	require.NoError(t, err)
	applied, err := svc.workflowStore.IsOperationApplied(ctx, childCompletionOperationID(parentID, rows))
	require.NoError(t, err)
	require.True(t, applied, "the queued wake must settle the completion operation")

	entries, _, err := svc.messageQueue.SnapshotSession(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one wake must be queued for the parent")
	require.Contains(t, entries[0].Content, stepPrompt)

	// The parent's turn ends: the ordinary queue drain delivers the wake once.
	require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""))
	done := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(done) }
	require.True(t, svc.drainQueuedMessageForPromptableSessionLocked(ctx, sessionID))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the queued wake to dispatch")
	}
	require.Len(t, agentMgr.capturedPrompts, 1)
	require.True(t, strings.Contains(agentMgr.capturedPrompts[0], stepPrompt),
		"prompt = %q", agentMgr.capturedPrompts[0])
}
