package models

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPendingStepSignalEntryRoundTrip(t *testing.T) {
	for _, entry := range []float64{0, 42} {
		signal, ok := LoadPendingStepSignal(map[string]interface{}{SessionMetaKeyPendingStepCompletion: map[string]interface{}{"step_id": "step", "workflow_step_transition_id": entry}})
		require.True(t, ok)
		raw, err := json.Marshal(signal)
		require.NoError(t, err)
		var decoded map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &decoded))
		require.Equal(t, entry, decoded["workflow_step_transition_id"])
	}
}

func TestPendingStepSignalInvalidEntry(t *testing.T) {
	for _, entry := range []interface{}{-1, float64(1.5), float64(9007199254740992), "42", true} {
		_, ok := LoadPendingStepSignal(map[string]interface{}{SessionMetaKeyPendingStepCompletion: map[string]interface{}{"step_id": "step", "workflow_step_transition_id": entry}})
		require.False(t, ok, "invalid entry %v must not authorize completion", entry)
	}
}
