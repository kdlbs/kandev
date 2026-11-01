package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
)

const leakedProfileSecret = "launch-secret"

func secretBearingResource() *pluginsdk.ExecutorResourceDescriptor {
	resource := workspaceResource("", "")
	resource.StateJson = `{"resource_id":"` + leakedProfileSecret + `"}`
	return resource
}

// failResourceCheckpoints refuses every checkpoint that carries a resource, as
// a store that lost the environment would, so the launch has to hand off.
func failResourceCheckpoints(harness *workspaceLaunchHarness) *pluginExecutorInventory {
	handed := &pluginExecutorInventory{}
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Resource != nil {
			return models.ErrExecutionRotated
		}
		return nil
	}
	harness.request.RecordUnrecordedRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		*handed, _ = metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory)
		return nil
	}
	return handed
}

func requireNoSecretInHandoff(t *testing.T, handed pluginExecutorInventory) {
	t.Helper()
	if handed.Resource.GetResourceHandle() != "opaque-resource" {
		t.Fatalf("handed-off resource = %v, want the provisioned handle to survive", handed.Resource)
	}
	encoded, err := json.Marshal(handed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), leakedProfileSecret) {
		t.Fatalf("the unrecorded handoff carries a revealed profile secret: %s", encoded)
	}
}

func TestSecretInStateNeverReachesTheHandoffWhenTheClaimIsFenced(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), secretBearingResource())
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)})
	handed := failResourceCheckpoints(harness)

	if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
		t.Fatal("a launch whose state carries a secret succeeded")
	}
	requireNoSecretInHandoff(t, *handed)
}

func TestSecretInStateNeverReachesTheHandoffAfterTheDrainReceipt(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{{Name: pluginExecutorCapabilityDrainReceipt, Version: 1}}
	harness := newWorkspaceLaunchHarness(t, provider, secretBearingResource())
	harness.runtime.drainRuntime = (&pluginDrainerFake{}).drain
	handed := failResourceCheckpoints(harness)

	if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
		t.Fatal("a launch whose state carries a secret succeeded")
	}
	requireNoSecretInHandoff(t, *handed)
}

// The handoff is the last writer before the table, so it must refuse a secret
// on its own account whatever the caller passed.
func TestHandoffSanitizesAResourceWhoseStateCarriesASecret(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), secretBearingResource())
	handed := failResourceCheckpoints(harness)
	inventory := pluginExecutorInventory{EnvironmentID: "environment-plugin-1"}

	_ = harness.runtime.handOffUnrecordedPluginExecutorResource(context.Background(), harness.request, inventory, secretBearingResource(), "test", nil)
	requireNoSecretInHandoff(t, *handed)
}

// An adopted handoff written by an older build may already carry a secret; it
// must not be re-checkpointed into executors_running.
func TestAdoptingAHandoffDropsASecretFromItsState(t *testing.T) {
	runtime, _, store := claimSettlementRuntime(t, "ready", nil, nil)
	store.record = nil
	running := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseCleanupPending, secretBearingResource())
	item := &models.UnrecordedPluginExecutor{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: "environment-plugin-recovery", Running: running,
	}
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
		SecretValues: map[string]string{"credential": leakedProfileSecret},
	}}, store)
	var stored string
	store.onCheckpoint = func(record *models.ExecutorRunning) {
		encoded, _ := json.Marshal(record.Metadata)
		stored = string(encoded)
	}
	environment := &models.TaskEnvironment{ID: item.EnvironmentID, TaskID: item.TaskID}

	if err := runtime.adoptUnrecordedPluginExecutor(context.Background(), environment, item); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if stored == "" || strings.Contains(stored, leakedProfileSecret) {
		t.Fatalf("adopted inventory = %q, want a stored row without the secret", stored)
	}
}

// Whichever gate refuses the response first, a revealed secret in the state must
// not reach any inventory checkpoint. Here the descriptor is invalid too, so the
// launch fails before the secret gate would have run in its old position.
func TestSecretInStateNeverReachesACheckpointWhenTheDescriptorIsInvalid(t *testing.T) {
	resource := secretBearingResource()
	resource.StateVersion = 99
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), resource)

	if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
		t.Fatal("a launch with an invalid descriptor succeeded")
	}
	sawResource := false
	for _, checkpoint := range harness.checkpoints {
		encoded, err := json.Marshal(checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), leakedProfileSecret) {
			t.Fatalf("a checkpoint carries a revealed profile secret: %s", encoded)
		}
		sawResource = sawResource || checkpoint.Resource.GetResourceHandle() == "opaque-resource"
	}
	if !sawResource {
		t.Fatal("no checkpoint kept the resource handle for cleanup")
	}
}
