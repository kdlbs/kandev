package process

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func decodePluginConfig(t *testing.T, value string) map[string]any {
	t.Helper()
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(value), &document))
	return document
}

func TestOpenCodePluginConfigAddsOneEntryWithoutChangingUserFields(t *testing.T) {
	existing := `{
	  // user comment survives semantically, not byte-for-byte
	  "username": "operator",
	  "plugins": [
	    "opencode-user-plugin",
	    {"package": "./local/plugin", "options": {"strict": true}},
	  ],
	  "provider": {"openai": {"options": {"baseURL": "http://127.0.0.1:1/v1"}}},
	}`
	composed, err := composeOpenCodePluginConfig(existing, "/tmp/plugin-hash", "/tmp/launch/bootstrap.json")
	require.NoError(t, err)
	document := decodePluginConfig(t, composed)
	require.Equal(t, "operator", document["username"])
	plugins, ok := document["plugins"].([]any)
	require.True(t, ok)
	require.Len(t, plugins, 3)
	require.Equal(t, "opencode-user-plugin", plugins[0])
	require.Equal(t, "./local/plugin", plugins[1].(map[string]any)["package"])
	entry := plugins[2].(map[string]any)
	require.Equal(t, "/tmp/plugin-hash", entry["package"])
	require.Equal(t, "/tmp/launch/bootstrap.json", entry["options"].(map[string]any)["bootstrapFile"])
}

func TestOpenCodePluginConfigTreatsNullAsAnEmptyObject(t *testing.T) {
	composed, err := composeOpenCodePluginConfig("null", "/tmp/plugin-hash", "/tmp/bootstrap.json")
	require.NoError(t, err)
	document := decodePluginConfig(t, composed)
	require.Len(t, document["plugins"].([]any), 1)
}

func TestOpenCodePluginConfigReplacesItsPreviousBootstrapOnRestart(t *testing.T) {
	first, err := composeOpenCodePluginConfig("{}", "/tmp/plugin-hash", "/tmp/launch-1/bootstrap.json")
	require.NoError(t, err)
	second, err := composeOpenCodePluginConfig(first, "/tmp/plugin-hash", "/tmp/launch-2/bootstrap.json")
	require.NoError(t, err)
	document := decodePluginConfig(t, second)
	plugins := document["plugins"].([]any)
	require.Len(t, plugins, 1)
	require.Equal(t, "/tmp/launch-2/bootstrap.json", plugins[0].(map[string]any)["options"].(map[string]any)["bootstrapFile"])
}

func TestOpenCodePluginConfigIsIdempotent(t *testing.T) {
	first, err := composeOpenCodePluginConfig(`{}`, "/tmp/plugin-hash", "/tmp/bootstrap.json")
	require.NoError(t, err)
	second, err := composeOpenCodePluginConfig(first, "/tmp/plugin-hash", "/tmp/bootstrap.json")
	require.NoError(t, err)
	require.JSONEq(t, first, second)
}

func TestOpenCodePluginConfigPreservesWindowsPaths(t *testing.T) {
	composed, err := composeOpenCodePluginConfig(
		`{"plugins":[]}`,
		`C:\Users\kandev\AppData\Local\kandev\opencode-plugin\hash`,
		`C:\Users\kandev\AppData\Local\kandev\opencode-plugin\launch\bootstrap.json`,
	)
	require.NoError(t, err)
	document := decodePluginConfig(t, composed)
	entry := document["plugins"].([]any)[0].(map[string]any)
	require.Equal(t, `C:\Users\kandev\AppData\Local\kandev\opencode-plugin\hash`, entry["package"])
	require.Equal(t, `C:\Users\kandev\AppData\Local\kandev\opencode-plugin\launch\bootstrap.json`,
		entry["options"].(map[string]any)["bootstrapFile"])
}

func TestOpenCodePluginConfigRejectsReservedIdentityAndMalformedInput(t *testing.T) {
	cases := []string{
		`{"plugins":["kandev"]}`,
		`{"plugins":[{"package":"kandev","options":{}}]}`,
		`{"plugins":{"package":"other"}}`,
		`{"plugins":[{"package":"/tmp/plugin-hash","options":{"bootstrapFile":"/tmp/other.json","extra":true}}]}`,
		`{"plugins":[/broken]}`,
		`[]`,
	}
	for _, existing := range cases {
		_, err := composeOpenCodePluginConfig(existing, "/tmp/plugin-hash", "/tmp/bootstrap.json")
		require.Error(t, err, "input %s", existing)
	}
	_, err := composeOpenCodePluginConfig("{not json", "/tmp/plugin-hash", "/tmp/bootstrap.json")
	require.ErrorIs(t, err, errOpenCodePluginConfigInvalid)
}

func TestOpenCodePluginConfigConflictIsDistinctFromMalformed(t *testing.T) {
	_, err := composeOpenCodePluginConfig(`{"plugins":["kandev"]}`, "/tmp/plugin-hash", "/tmp/bootstrap.json")
	require.True(t, errors.Is(err, errOpenCodePluginConfigConflict))
}
