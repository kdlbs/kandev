package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kdlbs/kandev/pluginsdk"
)

func rollbackAfterFailure(t *testing.T, negotiated []string) (*pluginExecutorOperationsFake, []pluginExecutorInventory) {
	t.Helper()
	operations := &pluginExecutorOperationsFake{
		destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
	}
	runtime := NewPluginRemoteExecutor(operations, logger.Default())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	var checkpoints []pluginExecutorInventory
	request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		checkpoints = append(checkpoints, metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory))
		return nil
	}
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	profile := request.PluginExecutor.Profile
	inventory := pluginExecutorLaunchInventory(request, profile, request.InstanceID, "digest")
	inventory.NegotiatedCapabilities = negotiated
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-1"}
	operationContext := pluginExecutorRequestContext(context.Background(), request, profile, request.InstanceID, "digest")
	_ = runtime.cleanupAfterPluginExecutorFailure(context.Background(), request, operationContext, inventory, resource, "control_handshake", errors.New("boom"))
	return operations, checkpoints
}

func TestFailedLaunchRollbackSendsNotAttemptedDrainOutcome(t *testing.T) {
	operations, checkpoints := rollbackAfterFailure(t, []string{pluginExecutorCapabilityDrainReceipt + "@1"})
	if operations.destroyRequest == nil {
		t.Fatal("rollback sent no destroy request")
	}
	if got := operations.destroyRequest.GetDrainOutcome(); got != pluginDrainOutcomeNotAttempted {
		t.Fatalf("rollback drain_outcome = %q, want %q", got, pluginDrainOutcomeNotAttempted)
	}
	if operations.destroyRequest.GetDrainReceipt() != nil {
		t.Fatal("rollback fabricated a drain receipt")
	}
	if len(checkpoints) == 0 || checkpoints[0].DrainOutcome != pluginDrainOutcomeNotAttempted ||
		checkpoints[0].DrainClaim != operations.destroyRequest.GetCleanupClaim() {
		t.Fatalf("outcome was not persisted before destroy under the cleanup claim: %+v", checkpoints)
	}
}

func TestFailedLaunchRollbackOmitsDrainOutcomeWithoutTheCapability(t *testing.T) {
	operations, _ := rollbackAfterFailure(t, nil)
	if operations.destroyRequest == nil || operations.destroyRequest.GetDrainOutcome() != "" {
		t.Fatalf("a provider without drain_receipt received a drain outcome: %#v", operations.destroyRequest)
	}
}
