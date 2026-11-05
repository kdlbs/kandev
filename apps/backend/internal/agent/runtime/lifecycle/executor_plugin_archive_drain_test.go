package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const archiveAuthSecretID = "runtime-secret-auth"

// archiveDrainFixture builds a runtime whose stored record carries only the
// reference to the agentctl token, as the inventory store returns it.
func archiveDrainFixture(t *testing.T, mutate func(*pluginExecutorInventory)) (*PluginRemoteExecutor, *pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake, *pluginDrainerFake, *runtimeSecretsFake) {
	t.Helper()
	runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, func(inventory *pluginExecutorInventory, record *models.ExecutorRunning) {
		if mutate != nil {
			mutate(inventory)
		}
		record.TransientAuthToken = ""
	})
	store.record.Metadata[MetadataKeyAuthTokenSecret] = archiveAuthSecretID
	secretsFake := newRuntimeSecretsFake(&eventLog{})
	secretsFake.values[archiveAuthSecretID] = "agentctl-secret"
	runtime.SetRuntimeSecretStore(secretsFake)
	return runtime, operations, store, drainer, secretsFake
}

func archiveInstance(store *pluginExecutorInventoryStoreFake) *ExecutorInstance {
	return &ExecutorInstance{
		InstanceID: "execution-plugin-recovery", TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
		StopReason: StopReasonLaunchRollback, Metadata: store.record.Metadata,
	}
}

func TestRollbackLoadsTheStoredAgentctlTokenAndDrainsBeforeDestroy(t *testing.T) {
	runtime, operations, store, drainer, _ := archiveDrainFixture(t, nil)
	if err := runtime.StopInstance(context.Background(), archiveInstance(store), true); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if len(drainer.calls) != 1 {
		t.Fatalf("archive drained %d times, want once", len(drainer.calls))
	}
	if got := operations.destroyRequest.GetDrainOutcome(); got != pluginDrainOutcomeDrained {
		t.Fatalf("archive drain_outcome = %q, want %q", got, pluginDrainOutcomeDrained)
	}
	if operations.destroyRequest.GetDrainReceipt() == nil {
		t.Fatal("archive destroy carried no drain receipt")
	}
}

func TestRollbackUsesTheLiveInstanceTokenWithoutTheSecretStore(t *testing.T) {
	runtime, operations, store, drainer, _ := archiveDrainFixture(t, nil)
	runtime.SetRuntimeSecretStore(nil)
	instance := archiveInstance(store)
	instance.AuthToken = "agentctl-secret"
	if err := runtime.StopInstance(context.Background(), instance, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(drainer.calls) != 1 || operations.destroyRequest.GetDrainOutcome() != pluginDrainOutcomeDrained {
		t.Fatalf("drain calls = %d, outcome = %q", len(drainer.calls), operations.destroyRequest.GetDrainOutcome())
	}
}

func TestCascadeCleanupLoadsTheStoredAgentctlToken(t *testing.T) {
	runtime, operations, _, drainer, _ := archiveDrainFixture(t, nil)
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("cascade cleanup: %v", err)
	}
	if len(drainer.calls) != 1 || operations.destroyRequest.GetDrainOutcome() != pluginDrainOutcomeDrained {
		t.Fatalf("drain calls = %d, outcome = %q", len(drainer.calls), operations.destroyRequest.GetDrainOutcome())
	}
}

func TestCascadeWithoutAnyRecordedTokenStillDestroysWithUnreachable(t *testing.T) {
	runtime, operations, _, drainer, secretsFake := archiveDrainFixture(t, nil)
	delete(secretsFake.values, archiveAuthSecretID)
	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("cascade: %v", err)
	}
	if len(drainer.calls) != 0 || operations.destroyRequest.GetDrainOutcome() != pluginDrainOutcomeUnreachable {
		t.Fatalf("drain calls = %d, outcome = %q, want unreachable: a handshake happened and no credential is left", len(drainer.calls), operations.destroyRequest.GetDrainOutcome())
	}
}

// A retry carries the instance's launch-time snapshot, which is older than the
// stored inventory that the first attempt advanced to cleanup_pending.
func TestRollbackRetryBuildsItsInventoryFromTheStoredRecord(t *testing.T) {
	runtime, operations, store, _, _ := archiveDrainFixture(t, func(inventory *pluginExecutorInventory) {
		inventory.Phase = pluginExecutorPhaseCleanupPending
		inventory.Revision = 4
		inventory.Resource = &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-stored", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	})
	stale := archiveInstance(store)
	stale.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: pluginExecutorInventory{
		PluginID: "example", InstallationID: "install-1", ProviderKey: "remote", ProviderIdentity: "plugin:example:remote",
		ContractVersion: 1, EnvironmentID: "environment-plugin-recovery", ProfileID: "profile-plugin-recovery",
		Phase: pluginExecutorPhaseReady, EnvironmentGeneration: 7, OperationID: "operation-plugin-recovery", Revision: 2,
		Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-stale", StateJson: `{"resource":"old"}`, Platform: "linux-amd64", StateVersion: 1},
	}}
	phases := recordPhasesWritten(store)
	if err := runtime.StopInstance(context.Background(), stale, true); err != nil {
		t.Fatalf("archive retry: %v", err)
	}
	if got := operations.destroyRequest.GetResource().GetResourceHandle(); got != "resource-stored" {
		t.Fatalf("destroy used resource %q, want the stored record's", got)
	}
	for _, phase := range *phases {
		if phase == pluginExecutorPhaseReady {
			t.Fatalf("retry wrote the stale ready phase over cleanup_pending: %v", *phases)
		}
	}
}

func recordPhasesWritten(store *pluginExecutorInventoryStoreFake) *[]string {
	phases := []string{}
	store.onCheckpoint = func(record *models.ExecutorRunning) {
		if inventory, err := decodePluginExecutorInventory(record.Metadata); err == nil {
			phases = append(phases, inventory.Phase)
		}
	}
	return &phases
}
