package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPersistTaskSessionState_ReleasesOnlyWhenLeavingThePopulation pins AC-51's
// first persistence funnel: a transition out of the AC-1 population releases
// the reservation, and a transition that stays inside (or enters) it does not.
func TestPersistTaskSessionState_ReleasesOnlyWhenLeavingThePopulation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "streaming-task-leave", "streaming-session-leave", models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, "streaming-task-stay", "streaming-session-stay", models.TaskSessionStateStarting)

	svc := &Service{repo: repo, logger: testLogger(), sessionCeiling: newSessionCeilingController(unlimitedSessionCeiling, nil, nil), ceilingSweeper: newCeilingSweeper()}

	held := func(sessionID string) bool {
		svc.sessionCeiling.mu.Lock()
		defer svc.sessionCeiling.mu.Unlock()
		_, ok := svc.sessionCeiling.reservations[sessionID]
		return ok
	}
	reserve := func(sessionID string) {
		decision := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "t", sessionID: sessionID, origin: launchOriginAutomatic, seam: "test"})
		require.True(t, decision.admitted)
	}

	// RUNNING -> FAILED leaves the population.
	reserve("streaming-session-leave")
	leaveSession, err := repo.GetTaskSession(ctx, "streaming-session-leave")
	require.NoError(t, err)
	_, _, ok := svc.persistTaskSessionState(ctx, "streaming-session-leave", leaveSession, models.TaskSessionStateFailed, "boom")
	require.True(t, ok)
	require.False(t, held("streaming-session-leave"), "leaving the population must release the reservation")

	// STARTING -> RUNNING stays inside the population.
	reserve("streaming-session-stay")
	staySession, err := repo.GetTaskSession(ctx, "streaming-session-stay")
	require.NoError(t, err)
	_, _, ok = svc.persistTaskSessionState(ctx, "streaming-session-stay", staySession, models.TaskSessionStateRunning, "")
	require.True(t, ok)
	require.True(t, held("streaming-session-stay"), "staying inside the population must not release the reservation")
}

// TestPersistStrictTaskSessionState_ReleasesOnlyOnASuccessfulLeavingChange pins
// AC-51's second, independent persistence funnel used by transitionTaskSessionState.
func TestPersistStrictTaskSessionState_ReleasesOnlyOnASuccessfulLeavingChange(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "strict-task-leave", "strict-session-leave", models.TaskSessionStateRunning)

	svc := &Service{repo: repo, logger: testLogger(), sessionCeiling: newSessionCeilingController(unlimitedSessionCeiling, nil, nil), ceilingSweeper: newCeilingSweeper()}
	decision := svc.sessionCeiling.admit(ctx, admissionRequest{taskID: "t", sessionID: "strict-session-leave", origin: launchOriginAutomatic, seam: "test"})
	require.True(t, decision.admitted)

	session, err := repo.GetTaskSession(ctx, "strict-session-leave")
	require.NoError(t, err)
	changed, _, _, err := svc.persistStrictTaskSessionState(ctx, "strict-session-leave", session, models.TaskSessionStateCompleted, "")
	require.NoError(t, err)
	require.True(t, changed)

	svc.sessionCeiling.mu.Lock()
	_, held := svc.sessionCeiling.reservations["strict-session-leave"]
	svc.sessionCeiling.mu.Unlock()
	require.False(t, held, "a successful strict transition leaving the population must release the reservation")
}
