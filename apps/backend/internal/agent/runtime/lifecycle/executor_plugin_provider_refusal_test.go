package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func provisionRefusal(t *testing.T, outcome string) error {
	t.Helper()
	operations := &pluginExecutorOperationsFake{
		provisionResponse: &pluginsdk.ProvisionExecutorEnvironmentResponse{
			Error: &pluginsdk.ExecutorProviderError{Code: "node_contract", MessageId: "executor.node.identity_mismatch"},
		},
		recoverResponse: &pluginsdk.RecoverExecutorOperationResponse{Outcome: outcome},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	_, err := runtime.CreateInstance(context.Background(), request)
	if err == nil {
		t.Fatal("CreateInstance() unexpectedly succeeded")
	}
	return err
}

func TestProviderRefusalWithoutResourceKeepsItsCodeWhenOutcomeIsUnknown(t *testing.T) {
	err := provisionRefusal(t, "unknown")
	if !strings.Contains(err.Error(), "node_contract") || !strings.Contains(err.Error(), "executor.node.identity_mismatch") {
		t.Fatalf("error = %q, want the provider's code and message id", err)
	}
}

func TestProviderRefusalWithoutResourceKeepsItsCodeWhenNothingWasAllocated(t *testing.T) {
	err := provisionRefusal(t, "absent")
	if !strings.Contains(err.Error(), "node_contract") {
		t.Fatalf("error = %q, want the provider's code", err)
	}
}
