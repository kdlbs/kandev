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
