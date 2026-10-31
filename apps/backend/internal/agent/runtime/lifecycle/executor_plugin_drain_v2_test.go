package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// drain_receipt@2 widens the meaning of not_attempted to "the host never held
// an agentctl credential for this runtime". These tests pin the rule for a
// record whose handshake never started and, beside each case, that version 1
// still behaves as before.

var (
	drainV1           = []string{"drain_receipt@1"}
	drainV2           = []string{"drain_receipt@2"}
	everyCleanupCause = []string{
		pluginExecutorCleanupReasonTask, "task_environment_reset", "recovery_cleanup",
		pluginExecutorCleanupReasonBootstrapConsumed, "incomplete_bootstrap", pluginExecutorCleanupReasonLaunch,
		cleanupReasonAPIUnreachable,
	}
)

func TestDrainEvidenceWithoutAHandshakeFollowsTheNegotiatedVersion(t *testing.T) {
	for _, reason := range everyCleanupCause {
		for _, tc := range []struct {
			name       string
			negotiated []string
			want       string
		}{
			{"version 2", drainV2, pluginDrainOutcomeNotAttempted},
			{"version 1", drainV1, map[bool]string{true: pluginDrainOutcomeNotAttempted, false: pluginDrainOutcomeUnreachable}[reason == pluginExecutorCleanupReasonLaunch]},
		} {
			t.Run(reason+"/"+tc.name, func(t *testing.T) {
				runtime, _, _, _ := drainRuntimeFixture(t, tc.negotiated, nil)
				decision := runtime.drainPluginRuntime(context.Background(), "", &pluginsdk.ExecutorProviderRequestContext{},
					pluginExecutorInventory{NegotiatedCapabilities: tc.negotiated, RuntimeIdentity: "runtime"}, nil, nil, "claim-1", reason)
				require.Equal(t, tc.want, decision.outcome)
				require.Nil(t, decision.receipt)
			})
		}
	}
}

// A known credential always drains, whatever the version; not_attempted never
// follows a credential.
func TestVersionTwoDrainsWhenACredentialIsKnown(t *testing.T) {
	runtime, _, _, drainer := drainRuntimeFixture(t, drainV2, nil)
	decision := runtime.drainPluginRuntime(context.Background(), "token", &pluginsdk.ExecutorProviderRequestContext{},
		pluginExecutorInventory{NegotiatedCapabilities: drainV2, HandshakeStarted: true}, nil, nil, "claim-1", "task_cleanup")
	require.Equal(t, pluginDrainOutcomeDrained, decision.outcome)
	require.Len(t, drainer.calls, 1)
}

func destroyProfile() *models.ExecutorProviderLaunchProfile {
	return &models.ExecutorProviderLaunchProfile{Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery"}
}

// destroyStoredRecord runs the environment cleanup's destroy for one reason on
// the stored record, with no credential held.
func destroyStoredRecord(t *testing.T, negotiated []string, reason string, mutate func(*pluginExecutorInventory, *models.ExecutorRunning)) (*pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake, *pluginDrainerFake) {
	t.Helper()
	runtime, operations, store, drainer := drainRuntimeFixture(t, negotiated, func(inventory *pluginExecutorInventory, record *models.ExecutorRunning) {
		record.TransientAuthToken = ""
		if mutate != nil {
			mutate(inventory, record)
		}
	})
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	require.NoError(t, err)
	require.NoError(t, runtime.destroyPluginExecutorRecord(context.Background(), store.record, inventory, inventory.Resource, reason, destroyProfile()))
	return operations, store, drainer
}

// The outcome is persisted with the claim before the destroy call, for every
// cleanup reason, and the agentctl drain is never attempted.
func TestVersionTwoNotAttemptedIsPersistedBeforeTheDestroyForEveryReason(t *testing.T) {
	for _, reason := range everyCleanupCause {
		t.Run(reason, func(t *testing.T) {
			runtime, operations, store, drainer := drainRuntimeFixture(t, drainV2, func(_ *pluginExecutorInventory, record *models.ExecutorRunning) { record.TransientAuthToken = "" })
			var persistedAtDestroy pluginExecutorInventory
			operations.beforeDestroy = func() {
				persistedAtDestroy, _ = decodePluginExecutorInventory(store.record.Metadata)
			}
			inventory, err := decodePluginExecutorInventory(store.record.Metadata)
			require.NoError(t, err)
			require.NoError(t, runtime.destroyPluginExecutorRecord(context.Background(), store.record, inventory, inventory.Resource, reason, destroyProfile()))

			require.Equal(t, pluginDrainOutcomeNotAttempted, operations.destroyRequest.GetDrainOutcome())
			require.Nil(t, operations.destroyRequest.GetDrainReceipt())
			require.Equal(t, pluginDrainOutcomeNotAttempted, persistedAtDestroy.DrainOutcome, "the outcome was not persisted before the destroy call")
			require.Equal(t, operations.destroyRequest.GetCleanupClaim(), persistedAtDestroy.DrainClaim)
			require.Empty(t, drainer.calls, "no credential, so no drain may be attempted")
		})
	}
}

func TestVersionOneKeepsTheStrictOutcomeForTheSameInputs(t *testing.T) {
	for _, reason := range []string{"task_cleanup", "task_environment_reset", "recovery_cleanup", "bootstrap_consumed", "incomplete_bootstrap"} {
		t.Run(reason, func(t *testing.T) {
			operations, _, _ := destroyStoredRecord(t, drainV1, reason, nil)
			require.Equal(t, pluginDrainOutcomeUnreachable, operations.destroyRequest.GetDrainOutcome())
		})
	}
}

// The launch rollback reads the same table: under @2 the stage that failed no
// longer decides, only whether the host ever held a credential.
func TestLaunchRollbackDrainEvidenceUnderVersionTwo(t *testing.T) {
	cases := []struct {
		name      string
		stage     string
		handshake bool
		token     string
		want      string
	}{
		{"before any handshake, a stage with its own reason", cleanupStageRepositoryInventory, false, "", pluginDrainOutcomeNotAttempted},
		{"before any handshake under api_unreachable", cleanupReasonAPIUnreachable, false, "", pluginDrainOutcomeNotAttempted},
		{"handshake started without a credential, no durable_bootstrap", "control_handshake", true, "", pluginDrainOutcomeUnreachable},
		{"after the handshake with a credential", "instance_create", true, "token-1", pluginDrainOutcomeDrained},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRollbackHarness(t, tc.handshake, tc.token)
			h.inventory.NegotiatedCapabilities = drainV2
			_ = h.rollback(tc.stage)
			require.NotNil(t, h.operations.destroyRequest)
			require.Equal(t, tc.want, h.operations.destroyRequest.GetDrainOutcome(), "reason %q", h.operations.destroyRequest.GetCleanupReason())
			require.Equal(t, tc.want == pluginDrainOutcomeDrained, h.operations.destroyRequest.GetDrainReceipt() != nil)
		})
	}
}
