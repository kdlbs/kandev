package mcp

import (
	"context"
	"strings"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-PROMPT-SYNC-010.3
func TestNoMCPToolChangesPromptSyncOwnership(t *testing.T) {
	for _, surface := range []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask, mcpprofile.SurfaceConfiguration} {
		t.Run(string(surface), func(t *testing.T) {
			log := newTestLogger(t)
			backend := NewChannelBackendClient(log)
			t.Cleanup(backend.Close)
			profile := mcpprofile.New(surface, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, nil)
			s := NewWithProfile(backend, "session", "task", 10005, log, "", false, profile)
			t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
			for name, tool := range s.mcpServer.ListTools() {
				lower := strings.ToLower(name)
				for _, forbidden := range []string{"override", "revert", "prompt_sync", "prompt_conflict"} {
					require.NotContains(t, lower, forbidden, name)
				}
				if name == "update_shared_prompt_kandev" {
					require.Contains(t, tool.Tool.Description, "switch it to Local")
					require.Contains(t, tool.Tool.Description, "only the user")
				}
			}
		})
	}
}
