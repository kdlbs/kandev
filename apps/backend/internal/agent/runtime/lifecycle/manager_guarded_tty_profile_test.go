package lifecycle

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardedTTYProfileForAgentIsCodexKanbanOnly(t *testing.T) {
	base := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{
		mcpprofile.CapabilityUserQuestion,
	}, []string{"github"})

	codex := guardedTTYProfileForAgent(&base, "codex-acp", false)
	require.NotNil(t, codex)
	assert.True(t, codex.HasCapability(mcpprofile.CapabilityGuardedTTYExec))
	assert.True(t, codex.HasCapability(mcpprofile.CapabilityUserQuestion))
	assert.Equal(t, []string{"github"}, codex.Providers)
	assert.False(t, base.HasCapability(mcpprofile.CapabilityGuardedTTYExec), "source profile must not be mutated")

	for _, test := range []struct {
		name        string
		agentID     string
		passthrough bool
		surface     mcpprofile.Surface
	}{
		{name: "other adapter", agentID: "claude-acp", surface: mcpprofile.SurfaceKanbanTask},
		{name: "passthrough", agentID: "codex-acp", passthrough: true, surface: mcpprofile.SurfaceKanbanTask},
		{name: "office", agentID: "codex-acp", surface: mcpprofile.SurfaceOfficeTask},
		{name: "automation", agentID: "codex-acp", surface: mcpprofile.SurfaceAutomation},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := mcpprofile.New(test.surface, []mcpprofile.Capability{mcpprofile.CapabilityGuardedTTYExec}, nil)
			got := guardedTTYProfileForAgent(&input, test.agentID, test.passthrough)
			require.NotNil(t, got)
			assert.False(t, got.HasCapability(mcpprofile.CapabilityGuardedTTYExec))
		})
	}
	assert.Nil(t, guardedTTYProfileForAgent(nil, "codex-acp", false))
}
