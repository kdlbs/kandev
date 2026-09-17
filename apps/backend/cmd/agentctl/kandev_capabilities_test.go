package main

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrchestrationCapabilitiesUsesScopedDirectory(t *testing.T) {
	t.Setenv("KANDEV_PERSONAL_ASSISTANT_ENABLED", "true")
	t.Setenv("KANDEV_RUNTIME_API_PREFIX", orchestrationAPIPrefix)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "synthetic-token")
	captured := setupMockTransport(t, 200, `{"entries":[]}`)
	require.Zero(t, runKandevCLI([]string{"capabilities", "--kind", "plugin", "--session", "session", "--after", "cursor", "--limit", "500"}))
	require.Equal(t, "GET", captured.Method)
	require.Equal(t, "/api/v1/orchestration/runtime/capabilities", captured.Path)
	query, err := url.ParseQuery(captured.Query)
	require.NoError(t, err)
	require.Equal(t, "100", query.Get("limit"))
	require.Equal(t, "plugin", query.Get("kind"))
	require.Equal(t, "session", query.Get("session_id"))
	require.Equal(t, "cursor", query.Get("after"))
	t.Setenv("KANDEV_PERSONAL_ASSISTANT_ENABLED", "false")
	require.Equal(t, 1, runKandevCLI([]string{"capabilities"}))
}
