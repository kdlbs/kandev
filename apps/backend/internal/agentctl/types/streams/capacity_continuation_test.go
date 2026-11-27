package streams

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCapacityContinuationSnapshotAgentEventWireShape(t *testing.T) {
	const input = `{"type":"error","prompt_generation":7,"capacity_continuation":{"support":"codex_live_session_v1","prompt_generation":7,"evidence_complete":true,"pending_tools":false,"failed_tools":false,"unknown_outcomes":false,"permission_pending":false,"unaccounted_background":false,"completed_tools":2},"supports_audio":false,"supports_embedded_context":false,"supports_image":false,"supports_prompt_queueing":false}`
	var event AgentEvent
	require.NoError(t, json.Unmarshal([]byte(input), &event))
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.JSONEq(t, input, string(raw))
	require.Equal(t, uint64(7), event.CapacityContinuation.PromptGeneration)
	require.True(t, event.CapacityContinuation.SafeFor(7))
}

func TestCapacityContinuationSnapshotSafeForFailsClosed(t *testing.T) {
	base := CapacityContinuationSnapshot{
		Support: CapacityContinuationCodexLiveSessionV1, PromptGeneration: 7, EvidenceComplete: true,
		CompletedTools: 1,
	}
	require.True(t, base.SafeFor(7))
	noCompletedTools := base
	noCompletedTools.CompletedTools = 0
	require.False(t, noCompletedTools.SafeFor(7))

	cases := []struct {
		name   string
		mutate func(*CapacityContinuationSnapshot)
	}{
		{name: "unknown support", mutate: func(s *CapacityContinuationSnapshot) { s.Support = "future_live_session_v2" }},
		{name: "missing evidence", mutate: func(s *CapacityContinuationSnapshot) { s.EvidenceComplete = false }},
		{name: "stale generation", mutate: func(s *CapacityContinuationSnapshot) { s.PromptGeneration++ }},
		{name: "pending tool", mutate: func(s *CapacityContinuationSnapshot) { s.PendingTools = true }},
		{name: "failed tool", mutate: func(s *CapacityContinuationSnapshot) { s.FailedTools = true }},
		{name: "unknown outcome", mutate: func(s *CapacityContinuationSnapshot) { s.UnknownOutcomes = true }},
		{name: "pending permission", mutate: func(s *CapacityContinuationSnapshot) { s.PermissionPending = true }},
		{name: "background work", mutate: func(s *CapacityContinuationSnapshot) { s.UnaccountedBackground = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := base
			tc.mutate(&snapshot)
			require.False(t, snapshot.SafeFor(7))
		})
	}
	require.False(t, base.SafeFor(0))
	require.False(t, (*CapacityContinuationSnapshot)(nil).SafeFor(7))
}

func TestCapacityContinuationV2KnownFailedShellTools(t *testing.T) {
	base := CapacityContinuationSnapshot{
		Support: CapacityContinuationCodexLiveSessionV2, PromptGeneration: 7, EvidenceComplete: true,
	}
	require.False(t, base.SafeFor(7), "V2 needs at least one successful tool or known command failure")

	knownFailure := base
	knownFailure.KnownFailedShellTools = 1
	require.True(t, knownFailure.SafeFor(7))

	completedAndFailed := knownFailure
	completedAndFailed.CompletedTools = 2
	require.True(t, completedAndFailed.SafeFor(7))

	mock := CapacityContinuationSnapshot{
		Support: CapacityContinuationMockLiveSessionV2, PromptGeneration: 3, EvidenceComplete: true,
		KnownFailedShellTools: 1,
	}
	require.True(t, mock.SafeFor(3))
	require.False(t, mock.SafeFor(4))

	blocked := completedAndFailed
	blocked.FailedTools = true
	require.False(t, blocked.SafeFor(7), "an unclassified failure still blocks V2")
}

func TestCapacityContinuationV1RejectsKnownFailedShellTools(t *testing.T) {
	for _, support := range []CapacityContinuationSupport{
		CapacityContinuationCodexLiveSessionV1,
		CapacityContinuationOpenCodeLiveSessionV1,
		CapacityContinuationMockLiveSessionV1,
	} {
		t.Run(string(support), func(t *testing.T) {
			snapshot := CapacityContinuationSnapshot{
				Support: support, PromptGeneration: 7, EvidenceComplete: true,
				CompletedTools: 1, KnownFailedShellTools: 1,
			}
			require.False(t, snapshot.SafeFor(7), "V1 cannot infer V2 permission")
			require.False(t, snapshot.SupportsKnownFailedShellTools())
		})
	}
}

// @covers AC-PLATFORM-TURN-CONTINUITY-003.6
func TestCapacityContinuationSnapshotOpenCodeSupport(t *testing.T) {
	snapshot := CapacityContinuationSnapshot{
		Support: CapacityContinuationOpenCodeLiveSessionV1, PromptGeneration: 3, EvidenceComplete: true,
		CompletedTools: 1,
	}
	require.True(t, snapshot.SafeFor(3))
	require.False(t, snapshot.SafeFor(4))
	snapshot.UnaccountedBackground = true
	require.False(t, snapshot.SafeFor(3))
}
