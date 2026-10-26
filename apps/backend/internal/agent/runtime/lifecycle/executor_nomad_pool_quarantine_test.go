package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func inventoryOf(states ...nomad.EnvironmentState) func(context.Context) ([]nomad.EnvironmentState, error) {
	return func(context.Context) ([]nomad.EnvironmentState, error) { return states, nil }
}

func TestNomadPoolQuarantineIsScopedToItsNode(t *testing.T) {
	retained := nomad.EnvironmentState{
		EnvironmentID: "env-bad", ExecutionID: "x-bad", NodeID: "node-1",
		Phase: nomadPhaseCleanupPending, QuarantineState: nomad.QuarantineMarked,
	}
	req := &ExecutorCreateRequest{TaskEnvironmentID: "env-new", InstanceID: "x-new", ListNomadInventory: inventoryOf(retained)}

	if err := admitAgainstRetainedInventory(context.Background(), req, nomad.ProfileConfig{NodeID: "node-1"}); !errors.Is(err, ErrNomadWorkerQuarantined) {
		t.Fatalf("the quarantined node admitted a new environment: %v", err)
	}
	if err := admitAgainstRetainedInventory(context.Background(), req, nomad.ProfileConfig{NodeID: "node-2"}); err != nil {
		t.Fatalf("another node was refused because of node-1's quarantine: %v", err)
	}
}

func TestNomadPoolOtherEnvironmentsOnTheNodeDoNotBlockAdmission(t *testing.T) {
	healthy := nomad.EnvironmentState{EnvironmentID: "env-ok", ExecutionID: "x-ok", NodeID: "node-1", Phase: "ready"}
	req := &ExecutorCreateRequest{TaskEnvironmentID: "env-new", InstanceID: "x-new", ListNomadInventory: inventoryOf(healthy)}
	if err := admitAgainstRetainedInventory(context.Background(), req, nomad.ProfileConfig{NodeID: "node-1"}); err != nil {
		t.Fatalf("a healthy environment on the node blocked a second one: %v", err)
	}
}
