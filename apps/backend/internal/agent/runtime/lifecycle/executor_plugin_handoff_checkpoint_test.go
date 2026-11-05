package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
)

// claimStoreOverInventoryFake lets the host's real claim-checkpoint path write
// into the inventory store fake the cleanup reads.
type claimStoreOverInventoryFake struct {
	store *pluginExecutorInventoryStoreFake
}

func (a claimStoreOverInventoryFake) GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error) {
	return a.store.GetExecutorRunningBySessionID(ctx, sessionID)
}

func (a claimStoreOverInventoryFake) GetTaskEnvironmentRecoveryClaim(context.Context, string) (*models.TaskEnvironmentRecoveryClaim, error) {
	return a.store.claim, nil
}

func (a claimStoreOverInventoryFake) CheckpointPluginExecutorClaimInventory(_ context.Context, record *models.ExecutorRunning, cp models.PluginExecutorClaimCheckpoint) (bool, error) {
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return false, err
	}
	inventory.Claim = &pluginExecutorClaimState{
		Kind: cp.Kind, OperationID: cp.Claim.OperationID, AcquiredAt: pluginExecutorClaimAcquiredAt(cp.Claim), Sequence: cp.Sequence, Digest: cp.Digest,
	}
	inventory.Revision = record.ExpectedPluginExecutorRevision + 1
	stored := *record
	stored.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	a.store.record = &stored
	return true, nil
}

// A resource handed off by a launch that failed before the provider bound a
// node has no workspace descriptor. The provider still checkpoints under the
// handoff destroy's claim before it destroys, and the host accepts that
// checkpoint, so the destroy settles and the handoff is forgotten.
func TestHandedOffResourceDestroyAcceptsTheProvidersClaimCheckpoint(t *testing.T) {
	early := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-early", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, operations, store := claimSettlementRuntime(t, pluginExecutorPhaseCleanupPending, nil, early)
	running := store.record
	inventory, err := decodePluginExecutorInventory(running.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.NegotiatedCapabilities = []string{pluginExecutorCapabilityDestroyCheckpoints, pluginExecutorCapabilityWorkspaceDescriptor}
	running.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	store.unrecorded = []*models.UnrecordedPluginExecutor{{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: inventory.EnvironmentID, Running: running,
	}}
	store.record = nil

	manager := &Manager{logger: newTestLogger(), pluginExecutorProfileLoader: runtime.profileLoader, pluginExecutorClaimStore: claimStoreOverInventoryFake{store}}
	var checkpointErr error
	operations.beforeDestroy = func() {
		request := operations.destroyRequest
		callback := proto.Clone(request.GetContext()).(*pluginsdk.ExecutorProviderRequestContext)
		callback.DispatchGeneration = 3
		checkpointErr = manager.checkpointPluginExecutorClaim(context.Background(), &pluginsdk.CheckpointExecutorResourceRequest{
			Context: callback, Resource: request.GetResource(), Phase: pluginExecutorPhaseCleanupProgress,
			Sequence: 1, Claim: request.GetCleanupClaim(),
		}, pluginExecutorPhaseCleanupProgress)
	}

	cleanupErr := destroyClaimEnvironment(runtime)
	if checkpointErr != nil {
		t.Fatalf("host refused the provider's cleanup_progress checkpoint: %v", checkpointErr)
	}
	if cleanupErr != nil {
		t.Fatalf("cleanup of the handed-off resource: %v", cleanupErr)
	}
	if len(store.unrecorded) != 0 {
		t.Fatal("the handoff was not settled after a confirmed destroy")
	}
}
