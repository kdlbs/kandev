package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreviewAgentCommandSafeComposition(t *testing.T) {
	router, _, registry := newSettingsHarness(t, newFakeSettingsRepo(), nil)
	registerTestTUIAgent(t, registry, "preview-agent", 1)
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-command-preview/preview-agent",
		`{"safe_only":true,"command_prefix":"/private/tools/wrapper --token=opaque-secret {{kandev.work_dir}}","cli_flags":[{"flag":"--opaque=value","enabled":true}],"cli_passthrough":true}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.NotContains(t, response.Body.String(), "opaque-secret")
	require.NotContains(t, response.Body.String(), "--opaque=value")
	require.NotContains(t, response.Body.String(), "/private/tools")
	var result map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	composition := result["composition"].(map[string]any)
	require.Equal(t, "preview-agent-binary", composition["agent"].(map[string]any)["executable"])
	wrapper := composition["wrapper"].(map[string]any)["command"].(map[string]any)
	require.Equal(t, "wrapper", wrapper["executable"])
	require.Equal(t, []any{"{{kandev.work_dir}}"}, wrapper["references"])
}

func TestPreviewAgentCommandSafeValidationErrors(t *testing.T) {
	router, _, registry := newSettingsHarness(t, newFakeSettingsRepo(), nil)
	registerTestTUIAgent(t, registry, "preview-agent", 1)
	response := doSettingsRequest(router, http.MethodPost, "/api/v1/agent-command-preview/preview-agent", `{"safe_only":true,"command_prefix":"wrapper \"opaque-secret"}`)
	requireErrorBody(t, response, http.StatusBadRequest, "invalid command prefix")
	response = doSettingsRequest(router, http.MethodPost, "/api/v1/agent-command-preview/unknown-secret", `{"safe_only":true}`)
	requireErrorBody(t, response, http.StatusInternalServerError, "command preview unavailable")
}
