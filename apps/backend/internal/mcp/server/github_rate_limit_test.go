package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubRateLimitToolProfileAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		profile   profile.Context
		available bool
	}{
		{name: "kanban", profile: profile.New(profile.SurfaceKanbanTask, nil, nil), available: true},
		{name: "office", profile: profile.New(profile.SurfaceOfficeTask, nil, nil), available: true},
		{name: "configuration", profile: profile.New(profile.SurfaceConfiguration, nil, nil), available: false},
		{name: "external", profile: profile.New(profile.SurfaceExternal, nil, nil), available: false},
		{name: "automation", profile: profile.New(profile.SurfaceAutomation, nil, nil), available: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewChannelBackendClient(newTestLogger(t))
			defer backend.Close()
			s := NewWithProfile(backend, "test-session", "test-task", 10005, newTestLogger(t), "", false, tt.profile)

			_, ok := s.mcpServer.ListTools()["get_github_rate_limit_kandev"]
			if tt.available {
				require.True(t, ok)
				return
			}
			assert.False(t, ok)
		})
	}
}

func TestGitHubRateLimitToolUsesServerBoundTask(t *testing.T) {
	t.Parallel()

	backend := &testBackend{response: map[string]interface{}{
		"workspace_id":        "workspace-1",
		"interactive_allowed": true,
		"background_allowed":  true,
	}}
	s := newTaskModeServer(t, backend, "bound-task")

	result := callTool(t, s, "get_github_rate_limit_kandev", map[string]interface{}{})
	require.False(t, result.IsError)
	require.Equal(t, ws.ActionMCPGetGitHubRateLimit, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]string)
	require.True(t, ok)
	require.Equal(t, "bound-task", payload["task_id"])
	assert.Len(t, payload, 1, "the tool must not accept an arbitrary workspace")
}

func TestGitHubRateLimitToolDoesNotRequireGitHubProviderProfile(t *testing.T) {
	t.Parallel()

	backend := NewChannelBackendClient(newTestLogger(t))
	defer backend.Close()
	s := NewWithProfile(
		backend,
		"test-session",
		"test-task",
		10005,
		newTestLogger(t),
		"",
		false,
		profile.New(profile.SurfaceKanbanTask, nil, nil),
	)

	assert.Contains(t, s.mcpServer.ListTools(), "get_github_rate_limit_kandev")
}
