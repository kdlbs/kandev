package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kdlbs/kandev/pluginsdk"
)

func launchCleanupRuntime(t *testing.T, busy int, checkpointErr error) (*PluginRemoteExecutor, *pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake, error) {
	t.Helper()
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	store := &pluginExecutorInventoryStoreFake{claimBusy: busy}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(nil, store)
	runtime.cleanupClaimWait, runtime.cleanupClaimRetry = 200*time.Millisecond, 5*time.Millisecond
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return checkpointErr }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	profile := request.PluginExecutor.Profile
	inventory := pluginExecutorLaunchInventory(request, profile, request.InstanceID, "digest")
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-1"}
	operationContext := pluginExecutorRequestContext(context.Background(), request, profile, request.InstanceID, "digest")
	err := runtime.cleanupAfterPluginExecutorFailure(context.Background(), request, operationContext, inventory, resource, "control_handshake", errors.New("boom"))
	return runtime, operations, store, err
}

// An archive can hold the cleanup claim while the launch is still failing; the
// launch must wait for the claim rather than leave the provider job running.
func TestLaunchCleanupWaitsForABusyClaimAndDestroysTheResource(t *testing.T) {
	_, operations, _, err := launchCleanupRuntime(t, 3, nil)
	if operations.destroyRequest == nil {
		t.Fatalf("launch cleanup never destroyed the resource: %v", err)
	}
}

// A claim that stays busy and a checkpoint that is rejected leave the launch
// as the only holder of the resource handle. It hands the handle to the
// environment cleanup instead of mutating the provider without a claim.
func TestLaunchCleanupThatCannotRetainRetryInformationHandsTheResourceOff(t *testing.T) {
	runtime, operations, _, err := launchCleanupRuntime(t, 1<<20, errors.New("claim held elsewhere"))
	if operations.destroyRequest != nil {
		t.Fatal("the launch destroyed a resource without holding a persisted claim")
	}
	if err == nil || !strings.Contains(err.Error(), "not recorded") {
		t.Fatalf("error = %v, want the unrecorded resource to be reported", err)
	}
	if _, ok := runtime.unrecorded.Load("execution-plugin-1"); !ok {
		t.Fatal("the resource handle was dropped instead of handed to the environment cleanup")
	}
}
