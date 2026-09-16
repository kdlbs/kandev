package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestIsAC1SessionState(t *testing.T) {
	require.True(t, isAC1SessionState(models.TaskSessionStateStarting))
	require.True(t, isAC1SessionState(models.TaskSessionStateRunning))
	require.False(t, isAC1SessionState(models.TaskSessionStateCreated))
	require.False(t, isAC1SessionState(models.TaskSessionStateCompleted))
	require.False(t, isAC1SessionState(models.TaskSessionStateFailed))
	require.False(t, isAC1SessionState(models.TaskSessionStateCancelled))
	require.False(t, isAC1SessionState(models.TaskSessionStateWaitingForInput))
}

func TestConfirmCeilingReservation_ReleasesWithoutSignalling(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.ceilingSweeper = newCeilingSweeper()

	decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
		taskID: "t", sessionID: "s-confirm", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted)

	svc.confirmCeilingReservation("s-confirm")

	svc.sessionCeiling.mu.Lock()
	_, held := svc.sessionCeiling.reservations["s-confirm"]
	svc.sessionCeiling.mu.Unlock()
	require.False(t, held, "confirming must release the reservation")

	select {
	case <-svc.ceilingSweeper.signal:
		t.Fatal("confirming a reservation must not signal the retry sweep")
	default:
	}
}

func TestReleaseCeilingReservation_ReleasesAndSignals(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.ceilingSweeper = newCeilingSweeper()

	decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
		taskID: "t", sessionID: "s-release", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted)

	svc.releaseCeilingReservation("s-release")

	svc.sessionCeiling.mu.Lock()
	_, held := svc.sessionCeiling.reservations["s-release"]
	svc.sessionCeiling.mu.Unlock()
	require.False(t, held)

	select {
	case <-svc.ceilingSweeper.signal:
	default:
		t.Fatal("releasing a reservation must signal the retry sweep")
	}
}

func TestReleaseCeilingReservation_UnknownSessionIsANoOp(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	require.NotPanics(t, func() { svc.releaseCeilingReservation("never-reserved") })
	require.NotPanics(t, func() { svc.releaseCeilingReservation("") })
	require.NotPanics(t, func() { svc.confirmCeilingReservation("") })
}

func TestReleaseCeilingReservation_NilCeilingIsANoOp(t *testing.T) {
	svc := &Service{}
	require.NotPanics(t, func() { svc.releaseCeilingReservation("x") })
	require.NotPanics(t, func() { svc.confirmCeilingReservation("x") })
	require.NotPanics(t, func() { svc.ReleaseCeilingReservation("x") })
}

func TestReleaseCeilingIfLeftPopulation(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	// Unlimited: this test exercises releaseCeilingIfLeftPopulation's own
	// predicate, not the ceiling's admission math, so each seed must succeed
	// independently regardless of how many sessions are already reserved.
	svc.sessionCeiling = newSessionCeilingController(unlimitedSessionCeiling, nil, nil)
	svc.ceilingSweeper = newCeilingSweeper()

	seed := func(sessionID string) {
		decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
			taskID: "t", sessionID: sessionID, origin: launchOriginAutomatic, seam: "test",
		})
		require.True(t, decision.admitted)
	}
	held := func(sessionID string) bool {
		svc.sessionCeiling.mu.Lock()
		defer svc.sessionCeiling.mu.Unlock()
		_, ok := svc.sessionCeiling.reservations[sessionID]
		return ok
	}

	// STARTING -> RUNNING stays inside the population: no release.
	seed("stays-in")
	svc.releaseCeilingIfLeftPopulation("stays-in", models.TaskSessionStateStarting, models.TaskSessionStateRunning)
	require.True(t, held("stays-in"))

	// CREATED -> STARTING enters the population: no release (AC-51c).
	seed("enters")
	svc.releaseCeilingIfLeftPopulation("enters", models.TaskSessionStateCreated, models.TaskSessionStateStarting)
	require.True(t, held("enters"))

	// RUNNING -> COMPLETED leaves the population: release.
	seed("leaves")
	svc.releaseCeilingIfLeftPopulation("leaves", models.TaskSessionStateRunning, models.TaskSessionStateCompleted)
	require.False(t, held("leaves"))

	// CREATED -> FAILED never entered the population: no-op release, no panic.
	require.NotPanics(t, func() {
		svc.releaseCeilingIfLeftPopulation("never-existed", models.TaskSessionStateCreated, models.TaskSessionStateFailed)
	})
}
