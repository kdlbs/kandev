package client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
)

type runtimeControlProbeFake struct {
	healthErr   error
	healthCalls *atomic.Int32
	identity    *IdentityInfo
	identityErr error
	details     *ServerDetails
	detailsErr  error
}

func (probe runtimeControlProbeFake) Health(context.Context) error {
	if probe.healthCalls != nil {
		probe.healthCalls.Add(1)
	}
	return probe.healthErr
}

func TestAdoptedRuntimeMonitorUsesConfiguredRetryBudget(t *testing.T) {
	var healthCalls atomic.Int32
	stored := processidentity.Identity{PID: 41, GroupID: 41, SessionID: 41, BirthToken: "test:boot:41"}
	observed := make(chan RuntimeObservation, 2)
	monitor := NewAdoptedRuntimeMonitor(runtimeControlProbeFake{
		healthErr:   errors.New("i/o timeout"),
		healthCalls: &healthCalls,
		identityErr: errors.New("i/o timeout"),
	}, "server-a", stored, time.Millisecond, 50*time.Millisecond, 2,
		func(processidentity.Identity) (processidentity.State, error) { return processidentity.StateAlive, nil },
		func(observation RuntimeObservation) { observed <- observation })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	monitor.Start(ctx)
	select {
	case observation := <-observed:
		if observation.Evidence != RuntimeEvidenceTransportFailure {
			t.Fatalf("evidence = %q, want transient transport failure", observation.Evidence)
		}
	case <-time.After(time.Second):
		t.Fatal("monitor did not report its exhausted retry budget")
	}
	monitor.Stop()
	if calls := healthCalls.Load(); calls != 3 {
		t.Fatalf("health calls = %d, want initial attempt plus two retries", calls)
	}
	select {
	case extra := <-observed:
		t.Fatalf("monitor reported more than one terminal observation: %+v", extra)
	default:
	}
}

func TestAdoptedRuntimeMonitorShutdownSuppressesRecovery(t *testing.T) {
	stored := processidentity.Identity{PID: 41, GroupID: 41, SessionID: 41, BirthToken: "test:boot:41"}
	called := make(chan struct{}, 1)
	monitor := NewAdoptedRuntimeMonitor(runtimeControlProbeFake{
		healthErr:   errors.New("i/o timeout"),
		identityErr: errors.New("i/o timeout"),
	}, "server-a", stored, time.Millisecond, time.Second, 1,
		func(processidentity.Identity) (processidentity.State, error) { return processidentity.StateAlive, nil },
		func(RuntimeObservation) { called <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	monitor.Start(ctx)
	cancel()
	monitor.Stop()
	select {
	case <-called:
		t.Fatal("shutdown emitted a runtime recovery observation")
	default:
	}
}

func (probe runtimeControlProbeFake) GetIdentity(context.Context) (*IdentityInfo, error) {
	return probe.identity, probe.identityErr
}

func (probe runtimeControlProbeFake) GetServerDetails(context.Context) (*ServerDetails, error) {
	return probe.details, probe.detailsErr
}

func TestAdoptedRuntimeDeathEvidence(t *testing.T) {
	stored := processidentity.Identity{PID: 41, GroupID: 41, SessionID: 41, BirthToken: "test:boot:41"}
	probeIdentity := &IdentityInfo{ServerIdentity: "server-a"}
	storedDetails := &ServerDetails{ProcessIdentity: &stored}
	inspectState := func(state processidentity.State) ProcessIdentityInspector {
		return func(identity processidentity.Identity) (processidentity.State, error) {
			if identity != stored {
				t.Fatalf("inspected identity = %+v, want stored identity %+v", identity, stored)
			}
			return state, nil
		}
	}

	tests := []struct {
		name    string
		probe   RuntimeControlProbe
		stored  processidentity.Identity
		inspect ProcessIdentityInspector
		want    RuntimeEvidence
	}{
		{
			name: "healthy authenticated owner",
			probe: runtimeControlProbeFake{
				identity: probeIdentity,
				details:  storedDetails,
			},
			stored:  stored,
			inspect: inspectState(processidentity.StateAlive),
			want:    RuntimeEvidenceHealthy,
		},
		{
			name: "process birth is gone",
			probe: runtimeControlProbeFake{
				healthErr:   errors.New("connection refused"),
				identityErr: errors.New("connection refused"),
			},
			stored:  stored,
			inspect: inspectState(processidentity.StateExited),
			want:    RuntimeEvidenceProcessExited,
		},
		{
			name: "process id was reused",
			probe: runtimeControlProbeFake{
				healthErr:   errors.New("connection refused"),
				identityErr: errors.New("connection refused"),
			},
			stored:  stored,
			inspect: inspectState(processidentity.StateReused),
			want:    RuntimeEvidenceProcessExited,
		},
		{
			name: "live process with transient transport failure",
			probe: runtimeControlProbeFake{
				healthErr:   errors.New("i/o timeout"),
				identityErr: errors.New("i/o timeout"),
			},
			stored:  stored,
			inspect: inspectState(processidentity.StateAlive),
			want:    RuntimeEvidenceTransportFailure,
		},
		{
			name: "stale credential",
			probe: runtimeControlProbeFake{
				identity:   probeIdentity,
				detailsErr: ErrControlCredentialsRejected,
			},
			stored: stored,
			want:   RuntimeEvidenceCredentialsRejected,
		},
		{
			name: "foreign server at endpoint",
			probe: runtimeControlProbeFake{
				identity: &IdentityInfo{ServerIdentity: "server-b"},
			},
			stored: stored,
			want:   RuntimeEvidenceForeignOwner,
		},
		{
			name: "missing identity does not prove death",
			probe: runtimeControlProbeFake{
				healthErr:   errors.New("connection reset"),
				identityErr: errors.New("connection reset"),
			},
			want: RuntimeEvidenceOwnershipUnverified,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := AssessAdoptedRuntime(context.Background(), test.probe, "server-a", test.stored, test.inspect)
			if observation.Evidence != test.want {
				t.Fatalf("evidence = %q, want %q", observation.Evidence, test.want)
			}
		})
	}
}
