package acp

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/opencodeplugin"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestOpenCodePluginClaimsRetainConfirmedLaunches(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-oc"
	adapter.nativeGeneration = 3
	adapter.mu.Unlock()
	call, update := openCodeLaunchFrames("call-tests", "shell-tests", "go test ./...")
	require.NotNil(t, adapter.convertToolCallUpdate("ses-oc", call))
	event := adapter.convertToolCallResultUpdate("ses-oc", update)
	require.NotNil(t, event)
	claims := adapter.OpenCodePluginClaims()
	require.Len(t, claims, 1)
	require.Equal(t, "shell-tests", claims[0].ShellID)
	require.Equal(t, "ses-oc", claims[0].NativeSessionID)
	require.Equal(t, uint64(3), claims[0].NativeGeneration)
	require.Equal(t, "shell-tests", claims[0].WorkID)
	require.Equal(t, "shell-tests", claims[0].RunID)
	require.Equal(t, "call-tests", claims[0].SourceCallID)
	require.Equal(t, "go test ./...", claims[0].Title)
}

func TestOpenCodePluginClaimsAreClearedOnSessionTransition(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-first"
	adapter.nativeGeneration = 1
	adapter.mu.Unlock()
	call, update := openCodeLaunchFrames("call-first", "shell-first", "make test")
	adapter.convertToolCallUpdate("ses-first", call)
	adapter.convertToolCallResultUpdate("ses-first", update)
	require.Len(t, adapter.OpenCodePluginClaims(), 1)

	adapter.mu.Lock()
	adapter.sessionID = "ses-second"
	adapter.advanceNativeGenerationLocked()
	adapter.mu.Unlock()
	require.Empty(t, adapter.OpenCodePluginClaims(), "a superseded native session must drop its claims")
}

func TestOpenCodePluginClaimsAreDisabledWithSharedBackgroundWork(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, false)
	adapter.mu.Lock()
	adapter.sessionID = "ses-oc"
	adapter.nativeGeneration = 1
	adapter.mu.Unlock()
	call, update := openCodeLaunchFrames("call", "shell", "make test")
	adapter.convertToolCallUpdate("ses-oc", call)
	adapter.convertToolCallResultUpdate("ses-oc", update)
	require.Empty(t, adapter.OpenCodePluginClaims())
}

func TestOpenCodePluginAdmissionFenceTracksNativeGeneration(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-fence"
	adapter.nativeGeneration = 4
	adapter.mu.Unlock()
	call, update := openCodeLaunchFrames("call-fence", "shell-fence", "make test")
	adapter.convertToolCallUpdate("ses-fence", call)
	adapter.convertToolCallResultUpdate("ses-fence", update)
	claims := adapter.OpenCodePluginClaims()
	require.Len(t, claims, 1)
	admission := opencodeplugin.Admission{
		BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
		NativeSessionID:  claims[0].NativeSessionID,
		NativeGeneration: claims[0].NativeGeneration,
		ClaimKey:         claims[0].ShellID,
		Observation:      streams.WorkloadRunObservation{WorkID: claims[0].WorkID},
	}
	require.True(t, adapter.ValidateOpenCodePluginAdmission(admission))

	adapter.mu.Lock()
	adapter.sessionID = "ses-successor"
	adapter.advanceNativeGenerationLocked()
	adapter.mu.Unlock()
	require.False(t, adapter.ValidateOpenCodePluginAdmission(admission),
		"a superseded native session must revoke admission synchronously")
}

func TestOpenCodePluginRetiresAcceptedClaimsWithoutReplacingThem(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-retire"
	adapter.nativeGeneration = 1
	adapter.mu.Unlock()
	call, update := openCodeLaunchFrames("call-retire", "shell-retire", "make test")
	adapter.convertToolCallUpdate("ses-retire", call)
	adapter.convertToolCallResultUpdate("ses-retire", update)
	claim := adapter.OpenCodePluginClaims()[0]
	admission := opencodeplugin.Admission{
		BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
		NativeSessionID:  claim.NativeSessionID,
		NativeGeneration: claim.NativeGeneration,
		ClaimKey:         claim.ShellID,
		Observation:      streams.WorkloadRunObservation{WorkID: claim.WorkID},
	}
	adapter.RetireOpenCodePluginClaim(admission)
	require.Empty(t, adapter.OpenCodePluginClaims())
}

func TestOpenCodePluginClaimCapacityIsReleasedByRetirement(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-capacity"
	adapter.nativeGeneration = 1
	adapter.mu.Unlock()
	for index := 0; index < opencodeplugin.MaxClaims+44; index++ {
		shellID := "shell-" + strconv.Itoa(index)
		call, update := openCodeLaunchFrames("call-"+strconv.Itoa(index), shellID, "make test")
		adapter.convertToolCallUpdate("ses-capacity", call)
		adapter.convertToolCallResultUpdate("ses-capacity", update)
		claims := adapter.OpenCodePluginClaims()
		require.Len(t, claims, 1, "completed jobs must not consume live claim capacity")
		claim := claims[0]
		adapter.RetireOpenCodePluginClaim(opencodeplugin.Admission{
			BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
			NativeSessionID:  claim.NativeSessionID,
			NativeGeneration: claim.NativeGeneration,
			ClaimKey:         claim.ShellID,
			Observation:      streams.WorkloadRunObservation{WorkID: claim.WorkID},
		})
		require.Empty(t, adapter.OpenCodePluginClaims())
	}
}

func TestOpenCodePluginAdmissionGuardBlocksSessionTransition(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-guard"
	adapter.nativeGeneration = 2
	adapter.mu.Unlock()
	call, update := openCodeLaunchFrames("call-guard", "shell-guard", "make test")
	adapter.convertToolCallUpdate("ses-guard", call)
	adapter.convertToolCallResultUpdate("ses-guard", update)
	claim := adapter.OpenCodePluginClaims()[0]
	release, ok := adapter.BeginOpenCodePluginAdmission(opencodeplugin.Admission{
		BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
		NativeSessionID:  claim.NativeSessionID,
		NativeGeneration: claim.NativeGeneration,
		ClaimKey:         claim.ShellID,
		Observation:      streams.WorkloadRunObservation{WorkID: claim.WorkID},
	})
	require.True(t, ok)

	transitioned := make(chan struct{})
	go func() {
		adapter.mu.Lock()
		adapter.sessionID = "ses-successor"
		adapter.advanceNativeGenerationLocked()
		adapter.mu.Unlock()
		close(transitioned)
	}()
	select {
	case <-transitioned:
		t.Fatal("session transition completed while an admission was fenced")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case <-transitioned:
	case <-time.After(time.Second):
		t.Fatal("session transition did not complete after the admission fence released")
	}
	require.False(t, adapter.ValidateOpenCodePluginAdmission(opencodeplugin.Admission{
		BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
		NativeSessionID:  claim.NativeSessionID,
		NativeGeneration: claim.NativeGeneration,
		ClaimKey:         claim.ShellID,
		Observation:      streams.WorkloadRunObservation{WorkID: claim.WorkID},
	}))
}

func TestOpenCodePluginStaleRetirementKeepsReplacementClaim(t *testing.T) {
	adapter := openCodeBackgroundAdapter(t, true)
	adapter.mu.Lock()
	adapter.sessionID = "ses-replacement"
	adapter.nativeGeneration = 2
	adapter.openCodePluginClaims["shell-replacement"] = opencodeplugin.Claim{
		ShellID: "shell-replacement", NativeSessionID: "ses-replacement", NativeGeneration: 2,
		WorkID: "shell-replacement", RunID: "shell-replacement",
	}
	adapter.mu.Unlock()
	adapter.RetireOpenCodePluginClaim(opencodeplugin.Admission{
		BehaviorID:       opencodeplugin.BehaviorShellOutcomes,
		NativeSessionID:  "ses-replacement",
		NativeGeneration: 1,
		ClaimKey:         "shell-replacement",
		Observation:      streams.WorkloadRunObservation{WorkID: "shell-replacement"},
	})
	require.Len(t, adapter.OpenCodePluginClaims(), 1, "a stale acknowledgement must not remove a replacement claim")
}
