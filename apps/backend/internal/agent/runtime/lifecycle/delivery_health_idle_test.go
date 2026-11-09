package lifecycle

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func idleDeliveryPressureOwner(
	execution *AgentExecution,
	submission func() string,
	apply func(RuntimeStoragePressure),
	reconcile func(),
) deliveryHealthOwner {
	return deliveryHealthOwner{
		execution: execution,
		probe: func(context.Context) (*agentctl.StatusResponse, error) {
			return healthySilenceStatus(), nil
		},
		applyPressure: func(_ context.Context, pressure RuntimeStoragePressure) {
			if apply != nil {
				apply(pressure)
			}
		},
		current: func() bool { return true },
		reconcile: func(context.Context) DeliveryReconciliationResult {
			if reconcile != nil {
				reconcile()
			}
			return DeliveryReconciliationResult{}
		},
		submission:       submission,
		promptGeneration: execution.promptGenerationSnapshot,
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.15
// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
func TestDeliveryHealthSettledOwnerRecoversWithoutPrompt(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	execution.setDeliverySubmissionID("")
	sm := newDeliveryHealthTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probes, reconciles := 0, 0
	var pressures []RuntimeStoragePressure
	owner := idleDeliveryPressureOwner(execution, func() string { return "" },
		func(pressure RuntimeStoragePressure) { pressures = append(pressures, pressure) },
		func() { reconciles++ })
	probe := owner.probe
	owner.probe = func(ctx context.Context) (*agentctl.StatusResponse, error) {
		probes++
		if probes == 1 {
			status, _ := probe(ctx)
			status.DeliveryHealth.State = deliveryInterruptedHealthState
			return status, nil
		}
		return probe(ctx)
	}
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		if probes >= 4 {
			cancel()
		}
		return nil
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if probes < 2 || len(pressures) < 2 {
		t.Fatalf("probes=%d pressures=%d; idle observation did not run", probes, len(pressures))
	}
	if !pressures[0].WritePressure {
		t.Fatalf("first idle pressure = %+v, want stale blocked output", pressures[0])
	}
	if pressures[len(pressures)-1].WritePressure {
		t.Fatalf("last idle pressure = %+v, want confirmed recovery", pressures[len(pressures)-1])
	}
	if reconciles != 0 {
		t.Fatalf("reconciles=%d; idle observation must not settle or reconcile a submission", reconciles)
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
func TestDeliveryHealthIdleProbeRejectsSuccessor(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AgentExecution)
	}{
		{name: "new submission", mutate: func(execution *AgentExecution) {
			execution.setDeliverySubmissionID("submission-2")
		}},
		{name: "new harness generation", mutate: func(execution *AgentExecution) {
			execution.DeliveryHarnessGeneration++
		}},
		{name: "new prompt generation", mutate: func(execution *AgentExecution) {
			execution.promptGeneration++
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			execution := newDeliveryReconciliationTestExecution()
			execution.setDeliverySubmissionID("")
			sm := newDeliveryHealthTestManager()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			applied, reconciles := 0, 0
			owner := idleDeliveryPressureOwner(execution, func() string {
				return currentDeliverySubmissionID(execution)
			}, func(RuntimeStoragePressure) { applied++ }, func() { reconciles++ })
			owner.probe = func(context.Context) (*agentctl.StatusResponse, error) {
				test.mutate(execution)
				return healthySilenceStatus(), nil
			}
			sm.deliveryHealthWait = func(context.Context, time.Duration) error {
				cancel()
				return context.Canceled
			}
			sm.monitorDeliveryHealth(ctx, owner)
			if applied != 0 {
				t.Fatalf("applied %d stale idle observations", applied)
			}
			if reconciles != 0 {
				t.Fatalf("reconciles=%d; a late idle response reconciled a successor", reconciles)
			}
		})
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
func TestDeliveryHealthIdleUnsupportedStatusPreservesPressure(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	execution.setDeliverySubmissionID("")
	sm := newDeliveryHealthTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	applied := 0
	owner := idleDeliveryPressureOwner(execution, func() string { return "" },
		func(RuntimeStoragePressure) { applied++ }, nil)
	owner.probe = func(context.Context) (*agentctl.StatusResponse, error) {
		return &agentctl.StatusResponse{AgentStatus: "running"}, nil
	}
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if applied != 0 {
		t.Fatalf("unsupported status produced %d pressure observations", applied)
	}
}

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.15
func TestDeliveryHealthIdleObservationStopsWithOwner(t *testing.T) {
	execution := newDeliveryReconciliationTestExecution()
	execution.setDeliverySubmissionID("")
	sm := newDeliveryHealthTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	probes, applied := 0, 0
	owner := idleDeliveryPressureOwner(execution, func() string { return "" },
		func(RuntimeStoragePressure) { applied++ }, nil)
	owner.probe = func(context.Context) (*agentctl.StatusResponse, error) {
		probes++
		return healthySilenceStatus(), nil
	}
	owner.current = func() bool { return false }
	sm.deliveryHealthWait = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}
	sm.monitorDeliveryHealth(ctx, owner)
	if probes != 0 || applied != 0 {
		t.Fatalf("stale owner probed %d times and applied %d observations", probes, applied)
	}
}
