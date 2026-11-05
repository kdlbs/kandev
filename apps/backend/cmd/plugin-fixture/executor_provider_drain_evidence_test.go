package main

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func destroyWithEvidence(t *testing.T, mutate func(*pluginsdk.DestroyExecutorEnvironmentRequest)) *pluginsdk.DestroyExecutorEnvironmentResponse {
	t.Helper()
	plugin := newFixturePluginAt(t.TempDir())
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
	ctx, resource := provisionForCleanup(t, plugin)
	plugin.SetHost(&checkpointingHost{})
	request := &pluginsdk.DestroyExecutorEnvironmentRequest{Context: ctx, Resource: resource, CleanupClaim: "claim-1", CleanupReason: "task_cleanup"}
	mutate(request)
	response, err := plugin.DestroyExecutorEnvironment(context.Background(), request)
	require.NoError(t, err)
	return response
}

// A provider that requires drain_receipt accepts not_attempted only as the
// no-runtime proof of a failed launch, and a receipt only for its own claim.
func TestFixtureDestroyAppliesTheDrainEvidenceRules(t *testing.T) {
	receipt := func(claim string) *pluginsdk.ExecutorDrainReceipt {
		return &pluginsdk.ExecutorDrainReceipt{ReceiptId: "receipt-1", CleanupClaim: claim, RuntimeIdentity: "runtime-1"}
	}
	cases := []struct {
		name    string
		mutate  func(*pluginsdk.DestroyExecutorEnvironmentRequest)
		refused bool
	}{
		{"no outcome", func(*pluginsdk.DestroyExecutorEnvironmentRequest) {}, true},
		{"not attempted under task cleanup", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) { r.DrainOutcome = "not_attempted" }, true},
		{"not attempted under launch failure", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) {
			r.DrainOutcome, r.CleanupReason = "not_attempted", "launch_failed"
		}, false},
		{"drained without a receipt", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) { r.DrainOutcome = "drained" }, true},
		{"drained with another claim's receipt", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) {
			r.DrainOutcome, r.DrainReceipt = "drained", receipt("claim-0")
		}, true},
		{"drained with this claim's receipt", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) {
			r.DrainOutcome, r.DrainReceipt = "drained", receipt("claim-1")
		}, false},
		{"uncertain", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) { r.DrainOutcome = "uncertain" }, false},
		{"unreachable", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) { r.DrainOutcome = "unreachable" }, false},
		{"uncertain with a receipt", func(r *pluginsdk.DestroyExecutorEnvironmentRequest) {
			r.DrainOutcome, r.DrainReceipt = "uncertain", receipt("claim-1")
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := destroyWithEvidence(t, tc.mutate)
			if tc.refused {
				require.Equal(t, "drain_evidence_refused", response.GetError().GetCode())
				require.False(t, response.GetConfirmedAbsent())
				return
			}
			require.Nil(t, response.GetError())
			require.True(t, response.GetConfirmedAbsent())
		})
	}
}

// A provider configured without the capabilities neither checkpoints nor
// demands drain evidence.
func TestFixtureWithoutDeclaredCapabilitiesAcceptsABareDestroy(t *testing.T) {
	t.Setenv(fixtureCapabilitiesEnv, "")
	response := destroyWithEvidence(t, func(*pluginsdk.DestroyExecutorEnvironmentRequest) {})
	require.Nil(t, response.GetError())
	require.True(t, response.GetConfirmedAbsent())
}

// A provider declaring drain_receipt@2 accepts not_attempted under any cleanup
// reason, but never with a receipt; the version 1 declaration keeps the narrow
// rule exercised above.
func TestFixtureDrainReceiptVersionTwoAcceptsNotAttemptedForAnyReason(t *testing.T) {
	cases := []struct {
		name         string
		declaration  string
		reason       string
		withReceipt  bool
		wantRefusal  bool
		wantAbsences bool
	}{
		{"version 2, task cleanup", "destroy_checkpoints,drain_receipt@2", "task_cleanup", false, false, true},
		{"version 2, environment reset", "destroy_checkpoints,drain_receipt@2", "task_environment_reset", false, false, true},
		{"version 2, recovery cleanup", "destroy_checkpoints,drain_receipt@2", "recovery_cleanup", false, false, true},
		{"version 2, launch failure", "destroy_checkpoints,drain_receipt@2", "launch_failed", false, false, true},
		{"version 2 with a receipt", "destroy_checkpoints,drain_receipt@2", "task_cleanup", true, true, false},
		{"explicit version 1, task cleanup", "destroy_checkpoints,drain_receipt@1", "task_cleanup", false, true, false},
		{"unversioned means version 1", "destroy_checkpoints,drain_receipt", "task_cleanup", false, true, false},
		{"explicit version 1, launch failure", "destroy_checkpoints,drain_receipt@1", "launch_failed", false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(fixtureCapabilitiesEnv, tc.declaration)
			response := destroyWithEvidence(t, func(r *pluginsdk.DestroyExecutorEnvironmentRequest) {
				r.DrainOutcome, r.CleanupReason = "not_attempted", tc.reason
				if tc.withReceipt {
					r.DrainReceipt = &pluginsdk.ExecutorDrainReceipt{ReceiptId: "receipt-1", CleanupClaim: "claim-1", RuntimeIdentity: "runtime-1"}
				}
			})
			if tc.wantRefusal {
				require.Equal(t, "drain_evidence_refused", response.GetError().GetCode())
				return
			}
			require.Nil(t, response.GetError())
			require.Equal(t, tc.wantAbsences, response.GetConfirmedAbsent())
		})
	}
}
