package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// A destroy that fails while the cleanup-pending record is also rejected
// leaves the handle nowhere; the error has to say so instead of reading as a
// retryable cleanup.
func TestFailedDestroyWithRejectedRetentionReportsTheLostResource(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	harness.operations.destroyResponse = nil
	harness.operations.destroyErr = errors.New("provider unavailable")
	rotated := false
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Phase == pluginExecutorPhaseCleanupPending {
			rotated = true
		}
		if rotated {
			return models.ErrExecutionRotated
		}
		return nil
	}
	harness.runtime.ready = func(context.Context, *agentctl.Client) error { return errors.New("agentctl never became ready") }

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err == nil || !strings.Contains(err.Error(), "not recorded and was not removed") {
		t.Fatalf("error = %v, want the lost resource to be reported", err)
	}
}
