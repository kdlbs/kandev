package streams

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCapacityContinuationSnapshotAgentEventWireShape(t *testing.T) {
	const input = `{"type":"error","prompt_generation":7,"capacity_continuation":{"support":"codex_live_session_v1","prompt_generation":7,"evidence_complete":true,"pending_tools":false,"failed_tools":false,"unknown_outcomes":false,"permission_pending":false,"unaccounted_background":false,"completed_tools":2}}`
	var event AgentEvent
	require.NoError(t, json.Unmarshal([]byte(input), &event))
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"capacity_continuation"`)
	require.Equal(t, uint64(7), event.CapacityContinuation.PromptGeneration)
	require.True(t, event.CapacityContinuation.SafeFor(7))
}

func TestCapacityContinuationSnapshotSafeForFailsClosed(t *testing.T) {
	base := CapacityContinuationSnapshot{
		Support: CapacityContinuationCodexLiveSessionV1, PromptGeneration: 7, EvidenceComplete: true,
	}
	require.True(t, base.SafeFor(7))

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
