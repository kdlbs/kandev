package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionCapacityLiveChangePreservesReservations(t *testing.T) {
	svc := &Service{
		sessionCeiling: newSessionCeilingController(1, nil, nil),
		ceilingSweeper: newCeilingSweeper(),
	}
	decision := svc.sessionCeiling.admit(context.Background(), admissionRequest{
		taskID: "capacity-task", sessionID: "capacity-session", origin: launchOriginAutomatic, seam: "test",
	})
	require.True(t, decision.admitted)
	require.Equal(t, "capacity-session", decision.reservationKey)

	svc.SetSessionCapacity(0)

	require.Equal(t, 0, svc.SessionCapacity())
	svc.sessionCeiling.mu.Lock()
	_, stillHeld := svc.sessionCeiling.reservations["capacity-session"]
	svc.sessionCeiling.mu.Unlock()
	require.True(t, stillHeld, "changing capacity must preserve the launch reservation")
	select {
	case <-svc.ceilingSweeper.signal:
	default:
		t.Fatal("disabling the ceiling must signal the retry sweep")
	}
}

func TestSessionCapacityChangeSignalsSweepForRefresh(t *testing.T) {
	svc := &Service{
		sessionCeiling: newSessionCeilingController(2, nil, nil),
		ceilingSweeper: newCeilingSweeper(),
	}

	svc.SetSessionCapacity(1)
	select {
	case <-svc.ceilingSweeper.signal:
	default:
		t.Fatal("lowering capacity must request a status refresh sweep")
	}

	svc.SetSessionCapacity(3)
	select {
	case <-svc.ceilingSweeper.signal:
	default:
		t.Fatal("increasing capacity must signal the retry sweep")
	}
}
