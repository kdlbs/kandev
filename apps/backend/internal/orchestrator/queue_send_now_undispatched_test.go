package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/stretchr/testify/require"
)

func planCommentSendNowService(t *testing.T, promptErr error) (*Service, *mockMessageCreator) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-1", models.TaskSessionStateWaitingForInput, ""))
	agentMgr := &mockAgentManager{isAgentRunning: true, promptErr: promptErr, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))
	for _, id := range []string{"comment-a", "comment-b"} {
		_, err := svc.messageQueue.QueueMessageWithMetadata(ctx, "session-1", "task-1", "feedback "+id, "",
			messagequeue.QueuedByUser, false, nil, map[string]interface{}{
				plancomments.MetadataClientQueueID:      id,
				plancomments.MetadataRequestFingerprint: "fingerprint-" + id,
			})
		require.NoError(t, err)
	}
	return svc, messages
}

func runPlanCommentSendNow(t *testing.T, svc *Service) {
	t.Helper()
	ctx := context.Background()
	sources := svc.messageQueue.GetStatus(ctx, "session-1").Entries
	require.Len(t, sources, 2)
	claimed, err := svc.messageQueue.ClaimSendNow(ctx, "session-1", sources)
	require.NoError(t, err)
	svc.markQueuedDispatchInFlight("session-1", claimed.Dispatch.ID)
	// Keep restored entries pending so the test observes the restore itself.
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "session-1", false))
	svc.executeSendNowClaim(claimed)
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendNowPlanCommentsFailingBeforeTransportAreRestored(t *testing.T) {
	svc, messages := planCommentSendNowService(t,
		agentruntime.MarkPromptNotDispatched(errors.New("prompt admission persistence failed")))

	runPlanCommentSendNow(t, svc)

	entries := svc.messageQueue.GetStatus(context.Background(), "session-1").Entries
	require.Len(t, entries, 2, "undispatched Send Now sources must return to the queue")
	for _, entry := range entries {
		require.False(t, entry.IsDeliveryAttempted(), "entry %s kept the delivery marker", entry.ID)
		require.True(t, entry.IsDurablePlanComment(), "entry %s lost its replay identity", entry.ID)
	}
	require.Len(t, messages.userMessages, 1, "the combined transcript is written once")
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestSendNowPlanCommentsPossiblyDeliveredAreSettled(t *testing.T) {
	svc, _ := planCommentSendNowService(t, errors.New("agentctl transport closed mid-request"))

	runPlanCommentSendNow(t, svc)

	require.Empty(t, svc.messageQueue.GetStatus(context.Background(), "session-1").Entries,
		"a failure after possible receipt keeps at-most-once delivery")
}
