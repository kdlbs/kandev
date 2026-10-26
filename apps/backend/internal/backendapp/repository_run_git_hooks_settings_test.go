package backendapp

import (
	"testing"

	"github.com/stretchr/testify/require"

	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// The Git hooks opt-in changes only through the repository settings REST
// handlers; the agent-facing settings tool must neither accept nor apply it.
func TestUpdateSettingsRejectsRunGitHooks(t *testing.T) {
	for _, value := range []any{true, false, nil} {
		f := checkoutSettingsHarness(t)
		ctx := checkoutSettingsContext(t.Context())

		response := f.save(t, ctx, map[string]any{"repository.run_git_hooks": value}, "settings-ws")
		require.NotEqual(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)

		stored, err := f.store.GetRepository(t.Context(), "settings-repo")
		require.NoError(t, err)
		require.False(t, stored.RunGitHooks)
	}
}

func TestUpdateSettingsSavingOtherFieldsKeepsRunGitHooks(t *testing.T) {
	f := checkoutSettingsHarness(t)
	ctx := checkoutSettingsContext(t.Context())
	enabled := true
	_, err := f.api.UpdateRepository(ctx, "settings-repo", &taskservice.UpdateRepositoryRequest{RunGitHooks: &enabled})
	require.NoError(t, err)

	response := f.save(t, ctx, map[string]any{"repository.name": "Renamed"}, "settings-ws")
	require.Equal(t, ws.MessageTypeResponse, response.Type, "%s", response.Payload)

	stored, err := f.store.GetRepository(t.Context(), "settings-repo")
	require.NoError(t, err)
	require.True(t, stored.RunGitHooks)
	require.Equal(t, "Renamed", stored.Name)
}
