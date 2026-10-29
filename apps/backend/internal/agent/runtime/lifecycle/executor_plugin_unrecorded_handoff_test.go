package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
)

// claimRequiringOperations fails a destroy the way a provider that checkpoints
// cleanup_progress under its claim does: the claim named by the request has to
// be a persisted claim row, or nothing is mutated.
type claimRequiringOperations struct {
	pluginExecutorOperationsFake
	store *pluginExecutorInventoryStoreFake
}

func (f *claimRequiringOperations) DestroyExecutorEnvironment(ctx context.Context, req *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error) {
	if f.store.claim == nil || f.store.claim.OperationID != req.GetCleanupClaim() {
		return nil, errors.New("cleanup claim not found: checkpoint refused")
	}
	return f.pluginExecutorOperationsFake.DestroyExecutorEnvironment(ctx, req)
}

func handoffRuntime(record *models.ExecutorRunning) (*PluginRemoteExecutor, *claimRequiringOperations, *pluginExecutorInventoryStoreFake) {
	store := &pluginExecutorInventoryStoreFake{
		record:  record,
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	operations := &claimRequiringOperations{
		pluginExecutorOperationsFake: pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}},
		store:                        store,
	}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	return runtime, operations, store
}

func orphanedLaunchResource() *unrecordedPluginResource {
	return &unrecordedPluginResource{
		taskID: "task-plugin-recovery", environmentID: "environment-plugin-recovery", generation: 7,
		resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "opaque-resource", StateJson: `{"job":"one"}`, Platform: "linux-amd64", StateVersion: 1},
	}
}

// A launch that holds a provisioned resource it can neither claim nor record
// must not mutate the provider under a claim that does not exist. It keeps the
// handle for the environment cleanup, which runs under a real claim.
func TestUnrecordableLaunchResourceIsHandedToTheEnvironmentCleanup(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: environment moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)})
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Resource != nil {
			return models.ErrExecutionRotated
		}
		return nil
	}

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err == nil || !strings.Contains(err.Error(), "not recorded") {
		t.Fatalf("error = %v, want the unrecorded resource to be reported", err)
	}
	if harness.operations.destroyRequest != nil {
		t.Fatal("the launch destroyed the resource under a claim that was never persisted")
	}
	held, ok := harness.runtime.unrecorded.Load(harness.request.InstanceID)
	if !ok || held.(*unrecordedPluginResource).resource.GetResourceHandle() != "opaque-resource" {
		t.Fatalf("handed-off resource = %v, want the provisioned handle", held)
	}
}

func TestEnvironmentCleanupDestroysAHandedOffResourceUnderItsOwnClaim(t *testing.T) {
	record := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseAllocating, nil)
	runtime, operations, store := handoffRuntime(record)
	runtime.unrecorded.Store(record.AgentExecutionID, orphanedLaunchResource())

	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("DestroyTaskEnvironment() error = %v", err)
	}
	if operations.destroyRequest.GetResource().GetResourceHandle() != "opaque-resource" {
		t.Fatalf("destroyed resource = %v, want the handed-off handle", operations.destroyRequest.GetResource())
	}
	if store.claim == nil {
		t.Fatal("the destroy did not run under a persisted claim")
	}
	if _, ok := runtime.unrecorded.Load(record.AgentExecutionID); ok {
		t.Fatal("the handle was kept after the destroy was confirmed")
	}
}

func TestUnconfirmedDestroyKeepsTheHandedOffResource(t *testing.T) {
	record := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseAllocating, nil)
	runtime, operations, _ := handoffRuntime(record)
	operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{}
	runtime.unrecorded.Store(record.AgentExecutionID, orphanedLaunchResource())

	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("an unconfirmed destroy reported success")
	}
	if _, ok := runtime.unrecorded.Load(record.AgentExecutionID); !ok {
		t.Fatal("the handle was dropped before the destroy was confirmed")
	}
}

// A cleanup that finds no inventory row while a launch for the environment
// provisioned something is not a settled cleanup.
func TestEnvironmentCleanupWithoutInventoryButWithAHandedOffResourceFails(t *testing.T) {
	runtime, operations, _ := handoffRuntime(nil)
	runtime.unrecorded.Store("execution-plugin-recovery", orphanedLaunchResource())

	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("cleanup reported success while a provisioned resource is still unaccounted for")
	}
	if operations.destroyRequest != nil {
		t.Fatal("a destroy was issued without an inventory record to hold the claim checkpoints")
	}
}
