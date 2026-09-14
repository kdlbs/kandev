package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHandleAgentProcessStarted_ConfirmsCeilingReservationUnconditionally pins
// AC-52/AC-56a: the acceptance edge must confirm before profileExecutionResolver's
// own nil/sessionID guard, so an instance without dynamic routing configured still
// confirms a reservation on every ordinary launch.
func TestHandleAgentProcessStarted_ConfirmsCeilingReservationUnconditionally(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.ceilingSweeper = newCeilingSweeper()
	require.Nil(t, svc.profileExecutionResolver)

	decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
		taskID: "t", sessionID: "session-started", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted)

	svc.handleAgentProcessStarted(context.Background(), "t", "session-started", "exec-1")

	svc.sessionCeiling.mu.Lock()
	_, held := svc.sessionCeiling.reservations["session-started"]
	svc.sessionCeiling.mu.Unlock()
	require.False(t, held, "the acceptance edge must confirm (release) the reservation")

	select {
	case <-svc.ceilingSweeper.signal:
		t.Fatal("confirming on acceptance must not signal the retry sweep")
	default:
	}
}

// TestHandleAgentProcessStartFailed_ReleasesCeilingReservationUnconditionally
// pins AC-52's failure edge, mirroring the acceptance-edge test above.
func TestHandleAgentProcessStartFailed_ReleasesCeilingReservationUnconditionally(t *testing.T) {
	svc, _ := newServiceWithRealRepo(t)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	svc.ceilingSweeper = newCeilingSweeper()
	require.Nil(t, svc.profileExecutionResolver)

	decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
		taskID: "t", sessionID: "session-failed", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted)

	svc.handleAgentProcessStartFailed(context.Background(), "t", "session-failed", "exec-1", context.DeadlineExceeded)

	svc.sessionCeiling.mu.Lock()
	_, held := svc.sessionCeiling.reservations["session-failed"]
	svc.sessionCeiling.mu.Unlock()
	require.False(t, held, "the failure edge must release the reservation")

	select {
	case <-svc.ceilingSweeper.signal:
	default:
		t.Fatal("releasing on a start failure must signal the retry sweep")
	}
}
