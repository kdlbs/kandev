package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

type rollbackHarness struct {
	runtime    *PluginRemoteExecutor
	operations *pluginExecutorOperationsFake
	store      *pluginExecutorInventoryStoreFake
	drainer    *pluginDrainerFake
	request    *ExecutorCreateRequest
	inventory  pluginExecutorInventory
	resource   *pluginsdk.ExecutorResourceDescriptor
	phases     []string
}

func newRollbackHarness(t *testing.T, handshake bool, token string) *rollbackHarness {
	t.Helper()
	h := &rollbackHarness{
		operations: &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}},
		store:      &pluginExecutorInventoryStoreFake{},
		drainer:    &pluginDrainerFake{},
		resource:   &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "allocated", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1},
	}
	h.runtime = NewPluginRemoteExecutor(h.operations, newTestLogger())
	h.runtime.SetRecoveryDependencies(nil, h.store)
	h.runtime.drainRuntime = h.drainer.drain
	h.request = pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	h.request.rollbackToken = token
	h.request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	h.request.CheckpointRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		if envelope, ok := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory); ok {
			h.phases = append(h.phases, envelope.Phase)
		}
		return nil
	}
	h.inventory = pluginExecutorInventory{
		PluginID: "example", InstallationID: "install-1", ProviderKey: "remote", ContractVersion: 1,
		EnvironmentID: "environment-plugin-1", EnvironmentGeneration: 1, OperationID: "operation-plugin-1",
		NegotiatedCapabilities: []string{"drain_receipt@1"}, HandshakeStarted: handshake,
	}
	return h
}

func (h *rollbackHarness) rollback(stage string) error {
	opContext := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "example", InstallationId: "install-1", ProviderKey: "remote", ContractVersion: 1,
		EnvironmentId: "environment-plugin-1", OperationId: "operation-plugin-1",
	}
	return h.runtime.cleanupAfterPluginExecutorFailure(context.Background(), h.request, opContext, h.inventory, h.resource, stage, errors.New("boom"))
}

// The rollback sends the drain evidence the destroy table allows for the
// stage that failed.
func TestLaunchRollbackDrainEvidenceFollowsTheStageThatFailed(t *testing.T) {
	cases := []struct {
		name      string
		stage     string
		handshake bool
		token     string
		want      string
	}{
		{"before any handshake", "provision_checkpoint", false, "", pluginDrainOutcomeNotAttempted},
		{"handshake started without a credential", "control_handshake", true, "", pluginDrainOutcomeUnreachable},
		{"after the handshake with a credential", "instance_create", true, "token-1", pluginDrainOutcomeDrained},
		{"host api probe under its own reason", cleanupReasonAPIUnreachable, true, "token-1", pluginDrainOutcomeDrained},
		{"repository inventory mismatch without a credential", cleanupStageRepositoryInventory, true, "", pluginDrainOutcomeUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRollbackHarness(t, tc.handshake, tc.token)
			_ = h.rollback(tc.stage)
			request := h.operations.destroyRequest
			if request == nil {
				t.Fatal("the rollback did not destroy the resource")
			}
			if got := request.GetDrainOutcome(); got != tc.want {
				t.Fatalf("drain_outcome = %q, want %q (reason %q)", got, tc.want, request.GetCleanupReason())
			}
			if tc.want == pluginDrainOutcomeNotAttempted && request.GetCleanupReason() != pluginExecutorCleanupReasonLaunch {
				t.Fatalf("not_attempted under reason %q", request.GetCleanupReason())
			}
			if (tc.want == pluginDrainOutcomeDrained) != (request.GetDrainReceipt() != nil) {
				t.Fatalf("receipt %#v does not match outcome %q", request.GetDrainReceipt(), tc.want)
			}
			if request.GetDrainReceipt() != nil && request.GetDrainReceipt().GetCleanupClaim() != request.GetCleanupClaim() {
				t.Fatalf("receipt claim %q is not the destroy claim %q", request.GetDrainReceipt().GetCleanupClaim(), request.GetCleanupClaim())
			}
		})
	}
}

// Confirmed absence from a provider that declared destroy_checkpoints settles
// only when the claim's checkpoint was persisted; otherwise cleanup stays
// pending, as in the cleanup job's destroy.
func TestLaunchRollbackDoesNotSettleAbsentWithoutTheClaimCheckpoint(t *testing.T) {
	h := newRollbackHarness(t, false, "")
	h.inventory.NegotiatedCapabilities = []string{"drain_receipt@1", "destroy_checkpoints@1"}
	var released bool
	h.request.ReleaseRuntimeInventory = func(context.Context) error { released = true; return nil }
	_ = h.rollback("provision_checkpoint")
	if released {
		t.Fatal("the rollback released the inventory on confirmed absence the provider never checkpointed")
	}
	if last := h.phases[len(h.phases)-1]; last != pluginExecutorPhaseCleanupPending {
		t.Fatalf("final phase = %q, want cleanup_pending", last)
	}
}

func (h *rollbackHarness) recoverFailedProvision(ctx context.Context) error {
	opContext := &pluginsdk.ExecutorProviderRequestContext{
		PluginId: "example", InstallationId: "install-1", ProviderKey: "remote", ContractVersion: 1,
		EnvironmentId: "environment-plugin-1", OperationId: "operation-plugin-1",
	}
	return h.runtime.recoverOrRetainFailedProvision(ctx, h.request, opContext, h.request.PluginExecutor.Profile, h.inventory, errors.New("provision failed"))
}

// A failed retain checkpoint with a known handle is never dropped: the handle
// goes to the durable hand-off, including when Recover's descriptor is rejected.
func TestFailedProvisionRecoveryHandsOffAKnownHandleWhenTheRetainCheckpointFails(t *testing.T) {
	h := newRollbackHarness(t, false, "")
	h.request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return errors.New("store unavailable") }
	var handedOff []map[string]interface{}
	h.request.RecordUnrecordedRuntimeInventory = func(_ context.Context, metadata map[string]interface{}) error {
		handedOff = append(handedOff, metadata)
		return nil
	}
	h.operations.recoverResponse = &pluginsdk.RecoverExecutorOperationResponse{
		Outcome:  "found",
		Resource: &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "handle-kept", StateJson: `{"unexpected_key":"x"}`, StateVersion: 1},
	}
	_ = h.recoverFailedProvision(context.Background())
	if len(handedOff) != 1 {
		t.Fatalf("hand-offs = %d, want the rejected descriptor's handle handed off", len(handedOff))
	}
	envelope, _ := handedOff[0][MetadataKeyPluginExecutor].(pluginExecutorInventory)
	if envelope.Resource.GetResourceHandle() != "handle-kept" || envelope.Phase != pluginExecutorPhaseCleanupPending {
		t.Fatalf("handed-off envelope = phase %q resource %#v", envelope.Phase, envelope.Resource)
	}
}

// An archive cancels the launch context; the recovery that decides whether a
// resource exists still runs.
func TestFailedProvisionRecoveryRunsUnderACanceledLaunchContext(t *testing.T) {
	h := newRollbackHarness(t, false, "")
	h.operations.recoverResponse = &pluginsdk.RecoverExecutorOperationResponse{Outcome: "unknown"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = h.recoverFailedProvision(ctx)
	if h.operations.recoverRequest == nil || h.operations.recoverCtxErr != nil {
		t.Fatalf("recover ran = %v under context error %v, want it to run uncancelled", h.operations.recoverRequest != nil, h.operations.recoverCtxErr)
	}
}
