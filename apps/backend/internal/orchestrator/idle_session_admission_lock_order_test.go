package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const lockOrderReturnBudget = 2 * time.Second

// requireSessionAdmissionFree proves no holder occupies the session gate.
func requireSessionAdmissionFree(t *testing.T, svc *Service, sessionID string) {
	t.Helper()
	probeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := svc.messageQueue.WithSessionAdmission(probeCtx, sessionID, func(context.Context) error { return nil })
	require.NoError(t, err, "session admission must stay acquirable while a lifecycle operation runs")
}

func runWithinBudget(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(lockOrderReturnBudget):
		t.Fatalf("%s blocked on the session lifecycle lock while holding admission", name)
	}
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
func TestFocusRefreshSkipsBusyLifecycleWithoutHoldingAdmission(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.isAgentReadyFn = func(context.Context, string) bool { return true }

	releaseLifecycle := svc.acquireSessionLifecycleLock(row.SessionID)
	lifecycleReleased := false
	defer func() {
		if !lifecycleReleased {
			releaseLifecycle()
		}
	}()
	runWithinBudget(t, "focusTaskSession", func() {
		_, _, resumed, err := svc.focusTaskSession(ctx, row.TaskID, row.SessionID)
		require.NoError(t, err)
		require.False(t, resumed)
	})
	requireSessionAdmissionFree(t, svc, row.SessionID)
	require.True(t, svc.idleParkingFocusAt[row.SessionID].IsZero(),
		"focus must not refresh the idle interval while a lifecycle operation owns the session")

	releaseLifecycle()
	lifecycleReleased = true
	_, _, _, err := svc.focusTaskSession(ctx, row.TaskID, row.SessionID)
	require.NoError(t, err)
	require.False(t, svc.idleParkingFocusAt[row.SessionID].IsZero(),
		"positive control: focus refreshes the idle interval once the lifecycle lock is free")
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
func TestIdleSuspensionRevalidationSkipsBusyLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")

	releaseLifecycle := svc.acquireSessionLifecycleLock(row.SessionID)
	lifecycleReleased := false
	defer func() {
		if !lifecycleReleased {
			releaseLifecycle()
		}
	}()
	runWithinBudget(t, "idle suspension revalidation", func() {
		svc.suspendWorkspaceIdleSessionsOnce(ctx)
	})
	requireSessionAdmissionFree(t, svc, row.SessionID)
	require.Empty(t, agent.suspensionCalls(), "a session with a lifecycle operation in progress must not be suspended")

	releaseLifecycle()
	lifecycleReleased = true
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Len(t, agent.suspensionCalls(), 1, "positive control: the skipped candidate is suspended on a later scan")
}
