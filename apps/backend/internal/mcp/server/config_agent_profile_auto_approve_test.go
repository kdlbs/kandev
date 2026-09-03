package mcp

import (
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAgentProfileHandler_ForwardsExplicitAutoApprove(t *testing.T) {
	for _, value := range []bool{true, false} {
		backend := &testBackend{response: map[string]interface{}{"id": "profile-1"}}
		s := newTestServer(t, backend)

		result := callTool(t, s, "create_agent_profile_kandev", map[string]interface{}{
			"agent_id": "agent-1", "name": "Profile", "model": "gpt-5", "auto_approve": value,
		})

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPCreateAgentProfile, backend.lastAction)
		payload, ok := backend.lastPayload.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, value, payload["auto_approve"])
	}
}

func TestUpdateAgentProfileHandler_ForwardsExplicitAutoApprove(t *testing.T) {
	for _, value := range []bool{true, false} {
		backend := &testBackend{response: map[string]interface{}{"id": "profile-1"}}
		s := newTestServer(t, backend)

		result := callTool(t, s, "update_agent_profile_kandev", map[string]interface{}{
			"profile_id": "profile-1", "auto_approve": value,
		})

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPUpdateAgentProfile, backend.lastAction)
		payload, ok := backend.lastPayload.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, value, payload["auto_approve"])
	}
}
