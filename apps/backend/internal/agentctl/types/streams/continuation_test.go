package streams

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContinuationSafetySnapshotFailClosed(t *testing.T) {
	goodV1 := ContinuationSafetySnapshot{Support: ContinuationNativeSavedHistoryV1, PromptGeneration: 7, Known: true}
	require.True(t, goodV1.SafeFor(7))
	goodV2 := ContinuationSafetySnapshot{Support: ContinuationNativeSavedHistoryV2, PromptGeneration: 7, Known: true, CompletedTools: 2}
	require.True(t, goodV2.SafeFor(7))
	for _, mutate := range []func(*ContinuationSafetySnapshot){
		func(s *ContinuationSafetySnapshot) { s.Support = "future" },
		func(s *ContinuationSafetySnapshot) { s.Known = false },
		func(s *ContinuationSafetySnapshot) { s.Unsafe = true },
		func(s *ContinuationSafetySnapshot) { s.Pending = true },
		func(s *ContinuationSafetySnapshot) { s.PromptGeneration = 6 },
	} {
		candidate := goodV2
		mutate(&candidate)
		require.False(t, candidate.SafeFor(7))
	}
	require.False(t, goodV2.SafeFor(0))
	var event AgentEvent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"error","prompt_generation":7}`), &event))
	require.False(t, event.ContinuationSafety.SafeFor(7))
}

func TestContinuationSafetySnapshotAgentManagedV3AllowsResolvedPendingTools(t *testing.T) {
	const wire = `{"support":"native_history_agent_managed_tools_v3","prompt_generation":7,"known":true,"unsafe":false,"pending":true,"permissions_resolved":true}`
	var snapshot ContinuationSafetySnapshot
	require.NoError(t, json.Unmarshal([]byte(wire), &snapshot))
	require.True(t, snapshot.SafeFor(7))

	for _, incomplete := range []string{
		`{"support":"native_history_agent_managed_tools_v3","prompt_generation":7,"known":true,"pending":true}`,
		`{"support":"native_history_agent_managed_tools_v3","prompt_generation":7,"known":true,"pending":true,"permissions_resolved":false}`,
		`{"support":"native_history_agent_managed_tools_v3","prompt_generation":7,"known":true,"unsafe":true,"pending":true,"permissions_resolved":true}`,
		`{"support":"native_history_agent_managed_tools_future","prompt_generation":7,"known":true,"pending":true,"permissions_resolved":true}`,
	} {
		var candidate ContinuationSafetySnapshot
		require.NoError(t, json.Unmarshal([]byte(incomplete), &candidate))
		require.False(t, candidate.SafeFor(7))
	}
}
