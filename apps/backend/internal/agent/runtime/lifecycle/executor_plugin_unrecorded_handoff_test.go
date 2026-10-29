package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// A launch that holds a provisioned resource it can neither claim nor record
// must not mutate the provider under a claim that does not exist. It records
// the handle durably for the environment cleanup, which runs under a real claim.
func TestUnrecordableLaunchResourceIsHandedToTheEnvironmentCleanup(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: environment moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)})
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Resource != nil {
			return models.ErrExecutionRotated
		}
		return nil
	}
	var handed pluginExecutorInventory
	harness.request.RecordUnrecordedRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		handed, _ = metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory)
		return nil
	}

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err == nil || !strings.Contains(err.Error(), "not recorded") {
		t.Fatalf("error = %v, want the unrecorded resource to be reported", err)
	}
	if harness.operations.destroyRequest != nil {
		t.Fatal("the launch destroyed the resource under a claim that was never persisted")
	}
	if handed.Resource.GetResourceHandle() != "opaque-resource" || handed.Phase != pluginExecutorPhaseCleanupPending {
		t.Fatalf("handed-off inventory = phase %q resource %v, want cleanup_pending with the provisioned handle", handed.Phase, handed.Resource)
	}
}

// Without a durable handoff the launch is the last holder of the handle and
// must say the resource was neither recorded nor removed.
func TestUnrecordableLaunchResourceWithoutAHandoffReportsALostResource(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: environment moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)})
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Resource != nil {
			return models.ErrExecutionRotated
		}
		return nil
	}

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err == nil || !strings.Contains(err.Error(), "not recorded and was not removed") {
		t.Fatalf("error = %v, want the lost resource to be reported", err)
	}
}

// A handoff the cleanup cannot adopt stays recorded and fails the cleanup.
func TestEnvironmentCleanupFailsWhenAHandoffCannotBeAdopted(t *testing.T) {
	runtime, operations, store := claimSettlementRuntime(t, "ready", nil, nil)
	store.record = nil
	store.checkpointErr = errors.New("environment is held")
	running := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseCleanupPending, workspaceResource("", ""))
	store.unrecorded = []*models.UnrecordedPluginExecutor{{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: "environment-plugin-recovery", Running: running,
	}}

	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("cleanup reported success while a provisioned resource is still unaccounted for")
	}
	if operations.destroyRequest != nil {
		t.Fatal("a destroy was issued without an inventory record to hold the claim checkpoints")
	}
	if len(store.unrecorded) != 1 {
		t.Fatal("the handoff was dropped before the resource was destroyed")
	}
}
