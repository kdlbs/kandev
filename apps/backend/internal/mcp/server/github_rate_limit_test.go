package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestTaskProfilesDoNotExposeGitHubRateSnapshot(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		profile profile.Context
	}{
		{name: "kanban", profile: profile.New(profile.SurfaceKanbanTask, nil, nil)},
		{name: "office", profile: profile.New(profile.SurfaceOfficeTask, nil, nil)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewChannelBackendClient(newTestLogger(t))
			defer backend.Close()
			s := NewWithProfile(backend, "test-session", "test-task", 10005, newTestLogger(t), "", false, tt.profile)

			require.NotContains(t, s.mcpServer.ListTools(), "get_github_rate_limit_kandev")
		})
	}
}
