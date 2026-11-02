package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// Signal only after the real cancel guard has queued a waiter. Earlier SQL
// calls also inspect Done, and must not be mistaken for guard contention.
type settlementGuardWaitContext struct {
	context.Context
	guard   *cancelInFlightMutex
	waiting chan struct{}
	once    sync.Once
}

func (c *settlementGuardWaitContext) Done() <-chan struct{} {
	c.guard.stateMu.Lock()
	waiting := len(c.guard.waiters) != 0
	c.guard.stateMu.Unlock()
	if waiting {
		c.once.Do(func() { close(c.waiting) })
	}
	return c.Context.Done()
}

func TestStopSession_ExecutionLessSettlementYieldsToInverseOrderLaunch(t *testing.T) {
	svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	guard, releaseRef := svc.acquireCancelInFlightGuard(sessionID)
	defer releaseRef()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopCtx := &settlementGuardWaitContext{Context: ctx, guard: guard, waiting: make(chan struct{})}
	guardHeld, allowLaunch, launchRead := make(chan struct{}), make(chan struct{}), make(chan struct{})
	stopFinished, launchFinished := make(chan struct{}), make(chan struct{})
	stopResult, launchResult := make(chan error, 1), make(chan error, 1)
	boundaryErr := errors.New("launch reached repository under lifecycle ownership")
	svc.repo = &coordinatorStopRepoHooks{repoStore: repo, getSessionFunc: func(context.Context, string) (*models.TaskSession, error) {
		close(launchRead)
		return nil, boundaryErr
	}}
	var allowOnce sync.Once
	allow := func() { allowOnce.Do(func() { close(allowLaunch) }) }
	t.Cleanup(func() {
		cancel()
		allow()
		coordinatorStopAwaitSignal(t, stopFinished, "stop worker cleanup")
		coordinatorStopAwaitSignal(t, launchFinished, "launch worker cleanup")
	})
	go func() {
		defer close(launchFinished)
		guard.Lock()
		defer guard.Unlock()
		close(guardHeld)
		<-allowLaunch
		// Match the existing dynamic-successor/auto-start guard -> lifecycle order,
		// entering the actual StartCreatedSession consumer before external launch.
		_, err := svc.StartCreatedSession(withCancelInFlightGuardHeld(t.Context()), "task-execution-less-stop", sessionID, "profile", "prompt", true, false, true, nil, nil)
		launchResult <- err
	}()
	coordinatorStopAwaitSignal(t, guardHeld, "inverse-order launch guard")
	go func() {
		defer close(stopFinished)
		stopResult <- svc.StopSession(stopCtx, sessionID, "stop", false)
	}()
	select {
	case <-stopFinished:
		allow()
	case <-stopCtx.waiting:
		// The old settlement owns lifecycle while waiting for the launch's guard.
		release, acquired := svc.tryAcquireSessionLifecycleLock(sessionID)
		if acquired {
			release()
		}
		require.False(t, acquired, "settlement must be the lifecycle owner at the cycle boundary")
		allow()
		cancel() // Break the proven cycle so RED does not leak either worker.
	}
	coordinatorStopAwaitSignal(t, launchRead, "real launch crossed lifecycle boundary")
	coordinatorStopAwaitSignal(t, stopFinished, "stop result")
	coordinatorStopAwaitSignal(t, launchFinished, "launch result")
	require.ErrorIs(t, <-stopResult, lifecycle.ErrNoExecutionForSession, "settlement must yield without requiring cancellation to break a lock cycle")
	require.ErrorIs(t, <-launchResult, boundaryErr)
	active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, turnID, active.ID)
	releaseRef()
	svc.cancelInFlightMu.Lock()
	_, retained := svc.cancelInFlight[sessionID]
	svc.cancelInFlightMu.Unlock()
	require.False(t, retained, "contended settlement must release its guard reference")
}

func TestStopSession_ExecutionLessSettlementPreservesCancelledContext(t *testing.T) {
	svc, repo, _, sessionID, turnID := newExecutionLessStopService(t, models.TaskSessionStateWaitingForInput)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	settled, err := svc.settleExecutionLessSessionStop(ctx, sessionID)
	require.False(t, settled)
	require.ErrorIs(t, err, context.Canceled)
	active, err := repo.GetActiveTurnBySessionID(t.Context(), sessionID)
	require.NoError(t, err)
	require.Equal(t, turnID, active.ID)
}
