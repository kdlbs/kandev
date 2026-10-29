package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

func claimSettlementRuntime(t *testing.T, phase string, claimFor func(*models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState, resource *pluginsdk.ExecutorResourceDescriptor) (*PluginRemoteExecutor, *pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake) {
	t.Helper()
	record := pluginExecutorRecoveryRecord(t, phase, resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	inventory.NegotiatedCapabilities = []string{"destroy_checkpoints@1"}
	record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  record,
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	if claimFor != nil {
		store.onAcquire = func(record *models.ExecutorRunning, claim *models.TaskEnvironmentRecoveryClaim) {
			current, _ := decodePluginExecutorInventory(record.Metadata)
			current.Claim = claimFor(claim)
			record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: current}
		}
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

// checkpointedUnder is the claim state a provider's accepted checkpoint leaves
// for exactly this acquisition.
func checkpointedUnder(sequence uint64) func(*models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState {
	return func(claim *models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState {
		return &pluginExecutorClaimState{
			Kind: "cleanup", OperationID: claim.OperationID, AcquiredAt: pluginExecutorClaimAcquiredAt(claim), Sequence: sequence, Digest: "d",
		}
	}
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

// A checkpoint recorded under an earlier acquisition of the same operation, or
// under the same id acquired at another time, is not this claim's checkpoint.
func TestClaimCheckpointFromAnotherAcquisitionIsNotAbsence(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	cases := map[string]func(*models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState{
		"an earlier acquisition of the same operation": func(claim *models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState {
			return &pluginExecutorClaimState{
				Kind: "cleanup", OperationID: pluginExecutorClaimPrefix("operation-plugin-recovery", "cleanup") + "epoch-old:nonce-old",
				AcquiredAt: pluginExecutorClaimAcquiredAt(claim), Sequence: 4, Digest: "d",
			}
		},
		"the same id acquired at another time": func(claim *models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState {
			return &pluginExecutorClaimState{
				Kind: "cleanup", OperationID: claim.OperationID, AcquiredAt: "2001-01-01T00:00:00Z", Sequence: 4, Digest: "d",
			}
		},
		"a recovery claim": func(claim *models.TaskEnvironmentRecoveryClaim) *pluginExecutorClaimState {
			return &pluginExecutorClaimState{
				Kind: "recovery", OperationID: pluginExecutorClaimPrefix("operation-plugin-recovery", "recovery") + "epoch-old:nonce-old",
				AcquiredAt: pluginExecutorClaimAcquiredAt(claim), Sequence: 4, Digest: "d",
			}
		},
	}
	for name, stale := range cases {
		t.Run(name, func(t *testing.T) {
			runtime, _, store := claimSettlementRuntime(t, "ready", stale, resource)
			if err := destroyClaimEnvironment(runtime); err == nil {
				t.Fatal("a checkpoint of another acquisition settled this claim")
			}
			if store.deleted {
				t.Fatal("the inventory row was removed")
			}
		})
	}
}

func TestConfirmedAbsenceWithThePersistedCheckpointSettles(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, _, store := claimSettlementRuntime(t, "cleanup_progress", checkpointedUnder(2), resource)
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
	runtime, operations, store := claimSettlementRuntime(t, "cleanup_progress", checkpointedUnder(3), latest)
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("re-issued destroy: %v", err)
	}
	request := operations.destroyRequest
	if request == nil || !strings.Contains(request.GetResource().GetStateJson(), "half-destroyed") {
		t.Fatalf("destroy request = %#v, want the latest checkpointed state", request)
	}
	if request.GetCleanupClaim() == "" || request.GetCleanupClaim() != store.claimRequest.OperationID ||
		!strings.HasPrefix(request.GetCleanupClaim(), pluginExecutorClaimPrefix("operation-plugin-recovery", "cleanup")) {
		t.Fatalf("the destroy must run under the acquired claim: %q / %q", request.GetCleanupClaim(), store.claimRequest.OperationID)
	}
	if operations.provisionRequest != nil || operations.recoverRequest != nil {
		t.Fatal("recovery allocated or re-ran a provider operation instead of destroying")
	}
}
