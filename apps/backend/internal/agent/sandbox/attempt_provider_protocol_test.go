package sandbox

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAttemptProviderPreservesProtocolWithoutChangingLegacyJSON(t *testing.T) {
	original := AttemptProvider{PluginID: "fixture"}
	legacy, err := json.Marshal(original)
	require.NoError(t, err)
	require.NotContains(t, string(legacy), "protocol_major")
	var versioned AttemptProvider
	require.NoError(t, json.Unmarshal([]byte(`{"plugin_id":"fixture","protocol_major":2}`), &versioned))
	saved, err := json.Marshal(versioned)
	require.NoError(t, err)
	require.Contains(t, string(saved), `"protocol_major":2`)
}
