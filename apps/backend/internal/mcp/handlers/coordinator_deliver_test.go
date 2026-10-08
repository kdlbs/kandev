package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliverQueued_QueuesBehindRunningSession(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)

	got, err := h.DeliverQueued(ctx, target.ID, session.ID, "coordinator says hi")

	require.NoError(t, err)
	assert.Equal(t, session.ID, got)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, session.ID).Count)
	assert.Empty(t, orch.interruptCalls, "a coordinator message must never interrupt a turn")
}

func TestDeliverQueued_ReachesWaitingSessionWithoutInterrupt(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)
	h, orch := newMessageTaskHandler(t, svc, repo)

	got, err := h.DeliverQueued(ctx, target.ID, session.ID, "coordinator says hi")

	require.NoError(t, err)
	assert.Equal(t, session.ID, got)
	assert.Empty(t, orch.interruptCalls)
	orch.mu.Lock()
	defer orch.mu.Unlock()
	assert.Len(t, orch.promptCalls, 1, "an idle session receives the prompt directly")
}

func TestDeliverQueued_RefusesSessionOfAnotherTask(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, _, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)

	_, err := h.DeliverQueued(ctx, sender.ID, session.ID, "wrong task")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong")
	assert.Equal(t, 0, orch.queue.GetStatus(ctx, session.ID).Count)
}

func TestDeliverQueued_QueueFullMapsToCoordinatorError(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	_, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateRunning)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.queue.SetMaxPerSession(1)
	orch.queue.SetMergeEnabled(false)
	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	_, err = orch.queue.QueueMessageWithMetadataForSession(ctx, identity, "existing", "", messagequeue.QueuedByAgent, false, nil, nil)
	require.NoError(t, err)

	_, err = h.DeliverQueued(ctx, target.ID, session.ID, "one too many")

	require.ErrorIs(t, err, coordinator.ErrMessageQueueFull)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, session.ID).Count)
}

func TestDeliverQueued_NoTaskServiceFailsClosed(t *testing.T) {
	h := &Handlers{}
	_, err := h.DeliverQueued(context.Background(), "t", "s", "p")
	require.Error(t, err)
}
