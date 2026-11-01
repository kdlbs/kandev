package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func foundWithoutVersion() *pluginsdk.RecoverExecutorOperationResponse {
	return &pluginsdk.RecoverExecutorOperationResponse{Outcome: "found", Resource: &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-found", StateJson: `{"resource":"one"}`, Platform: "linux-amd64",
	}}
}

// A recover result that carries a resource without its state_version is
// rejected by name; the host does not infer a version for the provider.
func TestRecoveredResourceWithoutStateVersionIsRejectedDuringLaunchRecovery(t *testing.T) {
	operations := &pluginExecutorOperationsFake{
		provisionErr:    context.DeadlineExceeded,
		recoverResponse: foundWithoutVersion(),
		destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	var phases []string
	request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		phases = append(phases, metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory).Phase)
		return nil
	}
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	_, err := runtime.CreateInstance(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "state version") {
		t.Fatalf("error = %v, want the missing state version named", err)
	}
	if operations.destroyRequest != nil {
		t.Fatal("destroy was sent for a recovered resource the host could not validate")
	}
	if len(phases) == 0 || phases[len(phases)-1] != pluginExecutorPhaseCleanupPending {
		t.Fatalf("checkpointed phases = %v, want cleanup retained as pending", phases)
	}
}

func TestRecoveredResourceWithoutStateVersionIsRejectedDuringCleanup(t *testing.T) {
	runtime, operations, store := claimSettlementRuntime(t, pluginExecutorPhaseAllocating, nil, nil)
	operations.recoverResponse = foundWithoutVersion()
	err := destroyClaimEnvironment(runtime)
	if err == nil || !strings.Contains(err.Error(), "state version") {
		t.Fatalf("error = %v, want the missing state version named", err)
	}
	if operations.destroyRequest != nil || store.deleted {
		t.Fatalf("cleanup acted on an unvalidated resource: destroy=%v deleted=%v", operations.destroyRequest, store.deleted)
	}
}
