package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

func claimSettlementRuntime(t *testing.T, phase string, claim *pluginExecutorClaimState, resource *pluginsdk.ExecutorResourceDescriptor) (*PluginRemoteExecutor, *pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake) {
	t.Helper()
	record := pluginExecutorRecoveryRecord(t, phase, resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	inventory.NegotiatedCapabilities = []string{"destroy_checkpoints@1"}
	inventory.Claim = claim
	record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  record,
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	return runtime, operations, store
}

func destroyClaimEnvironment(runtime *PluginRemoteExecutor) error {
	return runtime.DestroyTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "environment-plugin-recovery", TaskID: "task-plugin-recovery", OwnershipGeneration: 7,
	})
}

func TestMissingClaimCheckpointIsNotAbsence(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, operations, store := claimSettlementRuntime(t, "ready", nil, resource)
	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("confirmed absence without a persisted checkpoint was settled")
	}
	if operations.destroyRequest == nil {
		t.Fatal("destroy was not issued")
	}
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || inventory.Phase != "cleanup_pending" || inventory.Resource == nil {
		t.Fatalf("retained inventory = %+v, err=%v; cleanup information must survive", inventory, err)
	}
	if store.deleted {
		t.Fatal("the inventory row was removed")
	}
}

func TestClaimCheckpointFromAnotherClaimIsNotAbsence(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	stale := &pluginExecutorClaimState{Kind: "cleanup", OperationID: "operation-plugin-recovery:recovery", Sequence: 4, Digest: "d"}
	runtime, _, store := claimSettlementRuntime(t, "ready", stale, resource)
	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("a checkpoint of an earlier claim settled this claim")
	}
	if store.deleted {
		t.Fatal("the inventory row was removed")
	}
}

func TestConfirmedAbsenceWithThePersistedCheckpointSettles(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	state := &pluginExecutorClaimState{Kind: "cleanup", OperationID: "operation-plugin-recovery:cleanup", Sequence: 2, Digest: "d"}
	runtime, _, store := claimSettlementRuntime(t, "cleanup_progress", state, resource)
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("settle: %v", err)
	}
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || inventory.Phase != "absent" || inventory.Resource != nil {
		t.Fatalf("settled inventory = %+v, err=%v, want absent", inventory, err)
	}
}

func TestDestroyIsReissuedFromTheLatestCheckpointWithoutAllocating(t *testing.T) {
	latest := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"half-destroyed"}`, Platform: "linux-amd64", StateVersion: 1}
	state := &pluginExecutorClaimState{Kind: "cleanup", OperationID: "operation-plugin-recovery:cleanup", Sequence: 3, Digest: "d"}
	runtime, operations, store := claimSettlementRuntime(t, "cleanup_progress", state, latest)
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("re-issued destroy: %v", err)
	}
	request := operations.destroyRequest
	if request == nil || !strings.Contains(request.GetResource().GetStateJson(), "half-destroyed") {
		t.Fatalf("destroy request = %#v, want the latest checkpointed state", request)
	}
	if request.GetCleanupClaim() != "operation-plugin-recovery:cleanup" || store.claimRequest.OperationID != "operation-plugin-recovery:cleanup" {
		t.Fatalf("the re-issued destroy must run under the same claim identity: %q / %q", request.GetCleanupClaim(), store.claimRequest.OperationID)
	}
	if operations.provisionRequest != nil || operations.recoverRequest != nil {
		t.Fatal("recovery allocated or re-ran a provider operation instead of destroying")
	}
}
