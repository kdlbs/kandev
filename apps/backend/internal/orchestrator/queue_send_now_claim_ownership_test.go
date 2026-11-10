package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestSendNowClaimOwnsDurableSourceDuringReadinessDrain covers the incident's
// failing interleaving: while the Send Now replacement prompt is still waiting,
// a readiness drain scans the FIFO head. The open claim owns its durable source,
// so the drain reserves nothing, the claim restores cleanly, and the restore's
// own drain delivers the entry exactly once.
func TestSendNowClaimOwnsDurableSourceDuringReadinessDrain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1", "incarnation-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "session-1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	var prompts atomic.Int32
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(
			context.Context, string, string, []v1.MessageAttachment, bool,
		) (*executor.PromptResult, error) {
			if prompts.Add(1) == 1 {
				return nil, errors.New("prompt superseded by a newer turn")
			}
			return &executor.PromptResult{}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = withTestCapabilities(executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}))
	workerDone := make(chan struct{})
	svc.onQueuedMessageExecutionComplete = func() { close(workerDone) }
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	svc.messageQueue = messagequeue.NewService(
		newAuthoritativeMemoryRepository(repo), messagequeue.DefaultMaxPerSession, testLogger(),
	)

	identity := messagequeue.QueueSessionIdentity{
		TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "incarnation-1",
	}
	source, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "session-1", "task-1", "durable queued while the turn runs", "",
		messagequeue.QueuedByUser, false, nil,
		map[string]interface{}{messagequeue.MetadataLifecycleDurable: true},
	)
	require.NoError(t, err)
	claim, err := svc.messageQueue.ClaimSendNowForSession(ctx, identity, []messagequeue.QueuedMessage{*source})
	require.NoError(t, err)
	svc.markQueuedDispatchInFlight("session-1", claim.Dispatch.ID)

	reserved, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.False(t, ok, "the readiness drain must not reserve a claimed source")
	require.Nil(t, reserved)
	require.EqualValues(t, 0, prompts.Load(), "the readiness drain must not start a competing prompt")

	svc.executeSendNowClaimWithContext(ctx, claim, nil)
	select {
	case <-workerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the restored entry's FIFO dispatch")
	}

	require.EqualValues(t, 2, prompts.Load(),
		"one failed Send Now prompt plus exactly one restored FIFO delivery")
	for _, message := range messages.userMessages {
		require.NotEqual(t, models.MessageDeliveryStatusNotDelivered,
			message.metadata[models.MessageMetaKeyDeliveryStatus],
			"a successful restore must not mark the message not delivered")
	}
	status := svc.messageQueue.GetStatus(ctx, "session-1")
	require.Zero(t, status.Count)
}
