package watcher

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContinuationSafetySnapshotRemoteRoundTrip(t *testing.T) {
	const input = `{"session_id":"s1","agent_execution_id":"e1","prompt_generation":7,"continuation_safety":{"support":"native_saved_history_v1","prompt_generation":7,"known":true,"unsafe":false,"pending":false,"completed_reads":1}}`
	var data AgentEventData
	require.NoError(t, json.Unmarshal([]byte(input), &data))
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"continuation_safety"`)

	const inputV2 = `{"session_id":"s1","agent_execution_id":"e1","prompt_generation":7,"continuation_safety":{"support":"native_saved_history_completed_tools_v2","prompt_generation":7,"known":true,"unsafe":false,"pending":false,"completed_tools":2}}`
	var dataV2 AgentEventData
	require.NoError(t, json.Unmarshal([]byte(inputV2), &dataV2))
	rawV2, err := json.Marshal(dataV2)
	require.NoError(t, err)
	require.Contains(t, string(rawV2), `"completed_tools":2`)
	require.True(t, dataV2.ContinuationSafety.SafeFor(7))
}
