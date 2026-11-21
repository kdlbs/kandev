package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/stretchr/testify/require"
)

func dispatchReservedPlanComment(t *testing.T, svc *Service, ctx context.Context) {
	t.Helper()
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "t1", "s1")
	require.NoError(t, err)
	reserved, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, ok)
	reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(ctx, identity, reserved.ID, reserved)
	svc.executeQueuedMessageWithReservation("s1", reserved, reservation)
}

func queuePlanCommentForDelivery(t *testing.T, svc *Service, messages *mockMessageCreator, origin string) string {
	t.Helper()
	ctx := t.Context()
	const feedback = "Review the preview feedback before continuing."
	if origin == "message" {
		require.NoError(t, messages.CreateUserMessageIdempotent(ctx, "feedback-message", "t1", feedback, "s1", "", nil))
	}
	_, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "s1", "t1", feedback, "", messagequeue.QueuedByUser, false, nil, planCommentDeliveryMetadata(origin),
	)
	require.NoError(t, err)
	return feedback
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestQueuedPlanCommentFailingBeforeTransportStaysQueued(t *testing.T) {
	for _, origin := range []string{"message", "queue"} {
		t.Run(origin, func(t *testing.T) {
			svc, manager, messages := newPlanCommentDeliveryService(t, false)
			ctx := t.Context()
			feedback := queuePlanCommentForDelivery(t, svc, messages, origin)
			manager.promptErr = agentruntime.MarkPromptNotDispatched(errors.New("prompt admission persistence failed"))

			dispatchReservedPlanComment(t, svc, ctx)

			entries, err := svc.messageQueue.ListEntries(ctx, "s1")
			require.NoError(t, err)
			require.Len(t, entries, 1, "an undispatched comment message must stay queued")
			require.False(t, entries[0].IsDeliveryAttempted(), "the at-most-once marker must be revoked")
			require.False(t, entries[0].IsReservedInFlight(), "the entry must be visible for the next drain")
			require.True(t, entries[0].IsDurablePlanComment(), "the replay identity must survive the requeue")
			require.Len(t, messages.userMessages, 1)

			manager.promptErr = nil
			dispatchReservedPlanComment(t, svc, ctx)

			require.Zero(t, svc.messageQueue.GetStatus(ctx, "s1").Count)
			require.Len(t, messages.userMessages, 1, "the retry must not write a second transcript")
			require.NotEmpty(t, manager.capturedPromptCalls)
			require.Contains(t, manager.capturedPromptCalls[len(manager.capturedPromptCalls)-1].Prompt, feedback)
		})
	}
}

// @covers AC-TASKS-PLAN-COMMENTS-002.9
func TestQueuedPlanCommentPossiblyDeliveredFailureIsNotRetried(t *testing.T) {
	svc, manager, messages := newPlanCommentDeliveryService(t, false)
	ctx := t.Context()
	queuePlanCommentForDelivery(t, svc, messages, "queue")
	manager.promptErr = errors.New("agentctl transport closed mid-request")

	dispatchReservedPlanComment(t, svc, ctx)

	require.Zero(t, svc.messageQueue.GetStatus(ctx, "s1").Count,
		"a failure after possible receipt keeps at-most-once delivery")
}
