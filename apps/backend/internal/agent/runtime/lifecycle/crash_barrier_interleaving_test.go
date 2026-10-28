package lifecycle

import (
	"context"
	"fmt"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// crashSentinel is what a terminate-mode barrier panics with in these tests:
// it abandons the call at the barrier point exactly as a killed process would,
// leaving only what was already persisted.
type crashSentinel struct{ code int }

func installCrashingBarriers(t *testing.T) *crashBarrierSet {
	t.Helper()
	previous := defaultCrashBarriers
	set := newCrashBarrierSet(func() bool { return true }, func(code int) { panic(crashSentinel{code}) })
	defaultCrashBarriers = set
	t.Cleanup(func() { defaultCrashBarriers = previous })
	return set
}

// crashAt runs call and reports whether it was abandoned at a barrier.
func crashAt(call func()) (crashed bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(crashSentinel); !ok {
				panic(recovered)
			}
			crashed = true
		}
	}()
	call()
	return false
}

func TestCrashAfterDrainBeforeDestroyRecoversByReusingTheReceipt(t *testing.T) {
	set := installCrashingBarriers(t)
	runtime, operations, store, drainer := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	require.NoError(t, set.arm(BarrierAfterDrain, CrashBarrierModeTerminate))

	require.True(t, crashAt(func() { _ = destroyClaimEnvironment(runtime) }), "the barrier did not fire")
	require.Nil(t, operations.destroyRequest, "destroy ran after the crash point")
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	require.NoError(t, err)
	require.Equal(t, "drained", persisted.DrainOutcome, "the drain outcome must be durable before the barrier")

	require.NoError(t, destroyClaimEnvironment(runtime))
	require.Len(t, drainer.calls, 1, "recovery re-drained instead of reusing the receipt")
	require.NotNil(t, operations.destroyRequest)
	require.Equal(t, "drained", operations.destroyRequest.GetDrainOutcome())
}

func TestCrashAfterDestroyConfirmationRecoversByDestroyingAgain(t *testing.T) {
	set := installCrashingBarriers(t)
	runtime, operations, store, _ := drainRuntimeFixture(t, nil, nil)
	require.NoError(t, set.arm(BarrierAfterDestroyConfirm, CrashBarrierModeTerminate))

	require.True(t, crashAt(func() { _ = destroyClaimEnvironment(runtime) }), "the barrier did not fire")
	require.NotNil(t, operations.destroyRequest)
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	require.NoError(t, err)
	require.NotEqual(t, pluginExecutorPhaseAbsent, persisted.Phase, "absence must not be recorded before the barrier")

	operations.destroyRequest = nil
	require.NoError(t, destroyClaimEnvironment(runtime))
	require.NotNil(t, operations.destroyRequest, "recovery must destroy again; destroy is idempotent")
}

func TestCrashBeforeReadyPublicationRecoversAndPublishesOnce(t *testing.T) {
	set := installCrashingBarriers(t)
	fixture := newDurableRecoveryFixture(t, "provisioned")
	require.NoError(t, set.arm(BarrierBeforePublish, CrashBarrierModeTerminate))

	require.True(t, crashAt(func() { _, _ = fixture.recover() }), "the barrier did not fire")
	require.Empty(t, fixture.store.published, "ready was published past the crash point")

	restarted, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	require.Equal(t, "ready", restarted.Phase)
	require.True(t, restarted.PublishPending, "the crash must leave the publication marked pending")
	fixture.operations.attachResponse = &pluginsdk.AttachExecutorEnvironmentResponse{Resource: restarted.Resource}
	fixture.operations.inspectResponses = []*pluginsdk.InspectExecutorEnvironmentResponse{{State: "running"}, {State: "running"}}
	fixture.runtime.operations = confirmingOperations{fixture.operations}
	fixture.runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	defer instances[0].Client.Close()
	require.Len(t, fixture.store.published, 1)
	require.Nil(t, fixture.operations.provisionRequest, "recovery allocated a second environment")
}

func TestCrashAfterHandshakeBeforeAuthorizationRunsBootstrapConsumedCleanup(t *testing.T) {
	set := installCrashingBarriers(t)
	fixture := newDurableRecoveryFixture(t, "provisioned")
	require.NoError(t, set.arm(BarrierAfterHandshake, CrashBarrierModeTerminate))

	require.True(t, crashAt(func() { _, _ = fixture.recover() }), "the barrier did not fire")
	require.Empty(t, fixture.store.published)

	// The restarted backend finds the nonce already spent.
	fixture.handshake = func() error { return fmt.Errorf("%w: spent", agentctl.ErrHandshakeRejected) }
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Empty(t, instances)
	require.NotNil(t, fixture.operations.destroyRequest)
	require.Equal(t, "bootstrap_consumed", fixture.operations.destroyRequest.GetCleanupReason())
	require.Nil(t, fixture.operations.provisionRequest, "a duplicate environment was allocated")
	require.Empty(t, fixture.store.published, "ready was published for a spent nonce")
}

func TestCrashAfterProviderSideEffectLeavesTheAllocationIntentForRecovery(t *testing.T) {
	set := installCrashingBarriers(t)
	harness := newWorkspaceLaunchHarness(t, capabilityLaunchProvider(), workspaceResource(testBackendOnlyWorkspace, "worker-7"))
	require.NoError(t, set.arm(BarrierAfterSideEffect, CrashBarrierModeTerminate))

	require.True(t, crashAt(func() { _, _ = harness.runtime.CreateInstance(context.Background(), harness.request) }), "the barrier did not fire")
	require.NotNil(t, harness.operations.provisionRequest, "the provider side effect must precede the barrier")
	require.NotEmpty(t, harness.checkpoints)
	for _, envelope := range harness.checkpoints {
		require.Equal(t, pluginExecutorPhaseAllocating, envelope.Phase, "nothing past the allocation intent may be recorded before the barrier")
	}
	require.Empty(t, harness.instance.ID, "no agentctl instance may exist past the crash point")
}
