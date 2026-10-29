package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// An archive can rotate the execution between the provider returning a
// resource and the first durable write. The resource then has no inventory row
// to live in, so the launch must remove it itself rather than drop its handle.
func TestRotatedExecutionAfterProvisionDestroysTheResourceItHolds(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	store := &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: environment moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)}
	harness.runtime.SetRecoveryDependencies(nil, store)
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Resource != nil {
			return models.ErrExecutionRotated
		}
		return nil
	}

	if _, err := harness.runtime.CreateInstance(context.Background(), harness.request); err == nil {
		t.Fatal("CreateInstance() unexpectedly succeeded")
	}
	if harness.operations.destroyRequest == nil {
		t.Fatal("the provisioned resource was dropped: no durable record and no destroy call")
	}
	if harness.operations.destroyRequest.GetResource().GetResourceHandle() != "opaque-resource" {
		t.Fatalf("destroyed resource = %q, want the provisioned handle", harness.operations.destroyRequest.GetResource().GetResourceHandle())
	}
}

func TestRotatedExecutionAfterProvisionReportsAnUnremovableResource(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	harness.runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{claimErr: fmt.Errorf("%w: environment moved on", repoerrors.ErrTaskEnvironmentOwnershipChanged)})
	harness.operations.destroyResponse = nil
	harness.operations.destroyErr = errors.New("provider unavailable")
	rotated := false
	harness.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok && envelope.Resource != nil {
			rotated = true
		}
		if rotated {
			return models.ErrExecutionRotated
		}
		return nil
	}

	_, err := harness.runtime.CreateInstance(context.Background(), harness.request)
	if err == nil {
		t.Fatal("CreateInstance() unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "not recorded") || !strings.Contains(got, "not removed") {
		t.Fatalf("error = %q, want it to say the resource was neither recorded nor removed", got)
	}
}

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
