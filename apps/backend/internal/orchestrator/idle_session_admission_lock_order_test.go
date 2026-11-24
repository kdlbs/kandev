package orchestrator

import (
	"context"
	"sync"
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

// runWithinBudget runs fn on a worker and returns its error to the test
// goroutine, which owns every assertion.
func runWithinBudget(t *testing.T, name string, unblock func(), fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- fn()
	}()
	t.Cleanup(func() {
		if unblock != nil {
			unblock()
		}
		<-finished
	})
	select {
	case err := <-done:
		return err
	case <-time.After(lockOrderReturnBudget):
		t.Fatalf("%s blocked on the session lifecycle lock while holding admission", name)
		return nil
	}
}

// holdLifecycleLock holds the session lifecycle lock until the returned
// release runs; cleanup releases it on every exit path.
func holdLifecycleLock(t *testing.T, svc *Service, sessionID string) func() {
	t.Helper()
	var once sync.Once
	release := svc.acquireSessionLifecycleLock(sessionID)
	releaseOnce := func() { once.Do(release) }
	t.Cleanup(releaseOnce)
	return releaseOnce
}

func persistedFocusRecorded(t *testing.T, svc *Service, sessionID string) bool {
	t.Helper()
	session, err := svc.repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	_, ok := session.Metadata[idleParkingFocusMetadataKey]
	return ok
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.9
func TestFocusDuringBusyLifecycleKeepsAdmissionFree(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.isAgentReadyFn = func(context.Context, string) bool { return true }

	releaseLifecycle := holdLifecycleLock(t, svc, row.SessionID)
	var resumed bool
	err := runWithinBudget(t, "focusTaskSession", releaseLifecycle, func() error {
		var focusErr error
		_, _, resumed, focusErr = svc.focusTaskSession(ctx, row.TaskID, row.SessionID)
		return focusErr
	})
	require.NoError(t, err)
	require.False(t, resumed)
	requireSessionAdmissionFree(t, svc, row.SessionID)
	require.False(t, svc.idleParkingFocusAt[row.SessionID].IsZero(),
		"focus restarts the in-memory idle interval while a lifecycle operation owns the session")
	require.False(t, persistedFocusRecorded(t, svc, row.SessionID),
		"the persisted focus record waits for a settled execution")

	releaseLifecycle()
	_, _, _, err = svc.focusTaskSession(ctx, row.TaskID, row.SessionID)
	require.NoError(t, err)
	require.True(t, persistedFocusRecorded(t, svc, row.SessionID),
		"positive control: focus persists once the lifecycle lock is free")
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.9
func TestFocusDuringBusyLifecycleDefersIdleSuspension(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	agent.isAgentReadyFn = func(context.Context, string) bool { return true }

	// A reaper liveness probe holds the lifecycle lock when the user focuses.
	releaseLifecycle := holdLifecycleLock(t, svc, row.SessionID)
	_, _, _, err := svc.focusTaskSession(ctx, row.TaskID, row.SessionID)
	require.NoError(t, err)
	releaseLifecycle()

	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Empty(t, agent.suspensionCalls(), "a focused session must not be suspended on its old idle interval")
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.5
func TestIdleSuspensionRevalidationSkipsBusyLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")

	releaseLifecycle := holdLifecycleLock(t, svc, row.SessionID)
	require.NoError(t, runWithinBudget(t, "idle suspension revalidation", releaseLifecycle, func() error {
		svc.suspendWorkspaceIdleSessionsOnce(ctx)
		return nil
	}))
	requireSessionAdmissionFree(t, svc, row.SessionID)
	require.Empty(t, agent.suspensionCalls(), "a session with a lifecycle operation in progress must not be suspended")

	releaseLifecycle()
	svc.suspendWorkspaceIdleSessionsOnce(ctx)
	require.Len(t, agent.suspensionCalls(), 1, "positive control: the skipped candidate is suspended on a later scan")
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.9
func TestIdleSuspensionRevalidationHonorsFocusAfterCandidateSelection(t *testing.T) {
	ctx := context.Background()
	svc, agent, row := newIdleParkingFixture(t, true, 1, "claude-acp")
	current, err := svc.repo.GetExecutorRunningBySessionID(ctx, row.SessionID)
	require.NoError(t, err)
	now := time.Now().UTC()
	candidate, _, eligible := svc.workspaceIdleCandidate(ctx, current, now)
	require.True(t, eligible)
	require.True(t, svc.idleParkingCandidateDue(candidate.key, now, candidate.timeout))
	require.True(t, svc.claimIdleParkingCandidate(candidate.key))

	// Focus can reset the interval after selection but before admission.
	svc.resetIdleParkingCandidates(row.SessionID)
	summary := idleParkingScanSummary{skipped: make(map[idleParkingSkipReason]int)}
	svc.suspendWorkspaceIdleCandidate(ctx, current, candidate, agent, &summary)

	require.Empty(t, agent.suspensionCalls(), "revalidation must honor the refreshed focus interval")
}
