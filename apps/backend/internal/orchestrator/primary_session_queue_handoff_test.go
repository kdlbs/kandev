package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// enableTerminalPrimaryQueueHandoff also starts the service-owned worker
// lifetime that production Start creates, and joins it when the test ends.
func enableTerminalPrimaryQueueHandoff(t *testing.T, svc *Service) *Service {
	t.Helper()
	svc.config.TerminalPrimaryQueueHandoff = true
	svc.resetParkedSamplingWorkers()
	t.Cleanup(svc.stopParkedSamplingLoops)
	return svc
}

func queuedContents(t *testing.T, svc *Service, sessionID string) []string {
	t.Helper()
	entries, _, err := svc.messageQueue.SnapshotSession(context.Background(), sessionID)
	require.NoError(t, err)
	contents := make([]string, 0, len(entries))
	for _, entry := range entries {
		contents = append(contents, entry.Content)
	}
	return contents
}

// requireQueueMoved waits for the asynchronous hand-off.
func requireQueueMoved(t *testing.T, svc *Service, from, to string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return len(queuedContents(t, svc, from)) == 0 && len(queuedContents(t, svc, to)) == 1
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, []string{"follow-up"}, queuedContents(t, svc, to))
}

// A message queued for a primary that then fails would otherwise stay queued
// forever: the terminal session never becomes promptable again.
func TestPrimaryPromotionHandsOffTerminalSessionQueue(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-live", State: models.TaskSessionStateRunning},
	)
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateFailed)

	require.Equal(t, "session-live", primarySessionID(t, repo))
	requireQueueMoved(t, svc, primaryStoppedID, "session-live")
}

// An explicit primary choice that moves the role off a terminal session
// carries its queue too, e.g. when no candidate existed at stop time.
func TestExplicitPrimaryChoiceHandsOffTerminalSessionQueue(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "")
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)
	stopPrimary(t, svc, repo)
	require.Equal(t, primaryStoppedID, primarySessionID(t, repo))

	// Starting is not promptable yet, so the moved message stays visible.
	next := &models.TaskSession{ID: "session-next", TaskID: primaryTaskID, State: models.TaskSessionStateStarting}
	require.NoError(t, repo.CreateTaskSession(ctx, next))
	require.NoError(t, svc.SetPrimarySession(ctx, next.ID))

	require.Equal(t, "session-next", primarySessionID(t, repo))
	requireQueueMoved(t, svc, primaryStoppedID, "session-next")
}

// A live previous primary keeps its own queue.
func TestPrimaryChoiceKeepsQueueOfLiveSession(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "")
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateRunning, ""))
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)

	next := &models.TaskSession{ID: "session-next", TaskID: primaryTaskID, State: models.TaskSessionStateWaitingForInput}
	require.NoError(t, repo.CreateTaskSession(ctx, next))
	require.NoError(t, svc.SetPrimarySession(ctx, next.ID))

	require.Equal(t, []string{"follow-up"}, queuedContents(t, svc, primaryStoppedID))
	require.Empty(t, queuedContents(t, svc, "session-next"))
}

// With the flag off (every shipped profile), queued messages keep their
// explicit session target.
func TestPrimaryPromotionKeepsTerminalSessionQueueByDefault(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-live", State: models.TaskSessionStateRunning},
	)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateFailed)

	require.Equal(t, "session-live", primarySessionID(t, repo))
	require.Equal(t, []string{"follow-up"}, queuedContents(t, svc, primaryStoppedID))
	require.Empty(t, queuedContents(t, svc, "session-live"))
}

// Promotion runs inside the stopping session's queue admission. A receiver whose
// ID sorts before the stopping session used to deadlock the transfer, which
// locks both sessions in ID order.
func TestPrimaryPromotionInsideAdmissionHandsOffWithoutDeadlock(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-a-receiver", State: models.TaskSessionStateRunning},
	)
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	require.Less(t, "session-a-receiver", primaryStoppedID)
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateCancelled, ""))

	done := make(chan error, 1)
	go func() {
		done <- svc.messageQueue.WithSessionAdmission(ctx, primaryStoppedID, func(admitted context.Context) error {
			svc.maybePromotePrimary(admitted, primaryTaskID, primaryStoppedID, models.TaskSessionStateCancelled)
			return nil
		})
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("promotion inside the stopping session's admission did not return")
	}

	require.Equal(t, "session-a-receiver", primarySessionID(t, repo))
	requireQueueMoved(t, svc, primaryStoppedID, "session-a-receiver")
}

// The transfer combines both sessions' Auto-run settings. A paused previous
// primary must not pause the receiver, or the moved messages never drain.
func TestTerminalPrimaryQueueHandoffKeepsReceiverAutoRun(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-live", State: models.TaskSessionStateRunning},
	)
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, primaryStoppedID, false))
	require.True(t, svc.messageQueue.GetStatus(ctx, "session-live").AutoRun)

	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateFailed)

	requireQueueMoved(t, svc, primaryStoppedID, "session-live")
	require.True(t, svc.messageQueue.GetStatus(ctx, "session-live").AutoRun,
		"the hand-off must keep the receiver's own Auto-run setting")
}

// A fails and B takes over, then B fails and C takes over before either
// hand-off runs. B's hand-off can run first and find B's queue empty; A's
// hand-off must then deliver to C, not to the already-failed B.
func TestSuccessiveTerminalPromotionsDeliverQueueToFinalPrimary(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-b", State: models.TaskSessionStateRunning},
		&models.TaskSession{ID: "session-c", State: models.TaskSessionStateRunning},
	)
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)

	// Hold both hand-offs until both promotions have happened.
	svc.terminalQueueHandOffMu.Lock()
	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	require.NoError(t, svc.SetPrimarySession(ctx, "session-b"))
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-b", models.TaskSessionStateFailed, ""))
	require.NoError(t, svc.SetPrimarySession(ctx, "session-c"))
	svc.terminalQueueHandOffMu.Unlock()

	requireQueueMoved(t, svc, primaryStoppedID, "session-c")
	require.Empty(t, queuedContents(t, svc, "session-b"))
}

// After Stop, no detached hand-off starts; the queue keeps its target.
func TestTerminalPrimaryQueueHandoffDoesNotStartAfterStop(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "")
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)
	stopPrimary(t, svc, repo)
	svc.stopParkedSamplingLoops()

	next := &models.TaskSession{ID: "session-next", TaskID: primaryTaskID, State: models.TaskSessionStateStarting}
	require.NoError(t, repo.CreateTaskSession(ctx, next))
	require.NoError(t, svc.SetPrimarySession(ctx, next.ID))

	require.Equal(t, []string{"follow-up"}, queuedContents(t, svc, primaryStoppedID))
	require.Empty(t, queuedContents(t, svc, "session-next"))
}

// The hand-off resolves the live primary B, then an explicit promotion moves
// the role to C before the transfer commits. B is live, so it schedules no
// hand-off of its own; the messages must still reach C, the session that holds
// the primary role when they move.
func TestTerminalPrimaryQueueHandoffFollowsPromotionBeforeTransfer(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-b", State: models.TaskSessionStateRunning},
		&models.TaskSession{ID: "session-c", State: models.TaskSessionStateRunning},
	)
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)
	promoted := false
	svc.beforeTerminalQueueTransfer = func(toSessionID string) {
		if promoted {
			return
		}
		promoted = true
		assert.Equal(t, "session-b", toSessionID)
		assert.NoError(t, svc.SetPrimarySession(ctx, "session-c"))
	}

	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	require.NoError(t, svc.SetPrimarySession(ctx, "session-b"))

	requireQueueMoved(t, svc, primaryStoppedID, "session-c")
	require.Empty(t, queuedContents(t, svc, "session-b"))
	require.Equal(t, "session-c", primarySessionID(t, repo))
}

// A user who pauses the receiver's Auto-run while the hand-off is in flight
// keeps that pause; the hand-off must not restore an earlier value and drain.
func TestTerminalPrimaryQueueHandoffKeepsReceiverPauseDuringTransfer(t *testing.T) {
	svc, repo := newPrimaryPromotionFixture(t, "",
		&models.TaskSession{ID: "session-live", State: models.TaskSessionStateWaitingForInput},
	)
	enableTerminalPrimaryQueueHandoff(t, svc)
	ctx := context.Background()
	_, err := svc.messageQueue.QueueMessage(ctx, primaryStoppedID, primaryTaskID, "follow-up", "", "user", false, nil)
	require.NoError(t, err)
	require.True(t, svc.messageQueue.GetStatus(ctx, "session-live").AutoRun)
	svc.beforeTerminalQueueTransfer = func(toSessionID string) {
		assert.NoError(t, svc.messageQueue.SetAutoRun(ctx, toSessionID, false))
	}

	require.NoError(t, repo.UpdateTaskSessionState(ctx, primaryStoppedID, models.TaskSessionStateFailed, ""))
	svc.maybePromotePrimary(ctx, primaryTaskID, primaryStoppedID, models.TaskSessionStateFailed)

	requireQueueMoved(t, svc, primaryStoppedID, "session-live")
	svc.stopParkedSamplingLoops() // join the hand-off worker
	require.False(t, svc.messageQueue.GetStatus(ctx, "session-live").AutoRun,
		"the hand-off must keep the receiver's Auto-run pause made during the transfer")
	require.Equal(t, []string{"follow-up"}, queuedContents(t, svc, "session-live"))
}
