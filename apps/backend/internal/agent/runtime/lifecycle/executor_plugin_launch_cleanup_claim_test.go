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

func TestLaunchCleanupThatCannotRetainRetryInformationSaysSo(t *testing.T) {
	_, operations, _, err := launchCleanupRuntime(t, 1<<20, errors.New("claim held elsewhere"))
	if operations.destroyRequest != nil {
		t.Fatal("destroy ran without the cleanup claim")
	}
	if err == nil || !strings.Contains(err.Error(), "retry information could not be retained") {
		t.Fatalf("error = %v, want the lost retry information to be reported", err)
	}
}
