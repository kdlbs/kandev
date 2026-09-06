package mcp

import (
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCanvasTestServer(t *testing.T, backend BackendClient) *Server {
	t.Helper()
	return NewWithProfile(
		backend,
		"session-1",
		"task-1",
		10005,
		newTestLogger(t),
		"",
		false,
		mcpprofile.New(
			mcpprofile.SurfaceKanbanTask,
			[]mcpprofile.Capability{mcpprofile.CapabilityCanvas},
			nil,
		),
	)
}

func TestCanvasTools_AreNotRegisteredWithoutCapability(t *testing.T) {
	s := newTestServer(t, &testBackend{})

	for _, name := range []string{
		"list_canvases_kandev",
		"read_canvas_authoring_skill_kandev",
		"create_canvas_kandev",
		"get_canvas_kandev",
		"publish_canvas_kandev",
		"get_canvas_state_kandev",
		"set_canvas_state_kandev",
	} {
		assert.NotContains(t, s.mcpServer.ListTools(), name)
	}
}

func TestCanvasTools_RegisterAsOneCapabilityGroup(t *testing.T) {
	s := newCanvasTestServer(t, &testBackend{})

	for _, name := range []string{
		"list_canvases_kandev",
		"read_canvas_authoring_skill_kandev",
		"create_canvas_kandev",
		"get_canvas_kandev",
		"publish_canvas_kandev",
		"get_canvas_state_kandev",
		"set_canvas_state_kandev",
	} {
		assert.Contains(t, s.mcpServer.ListTools(), name)
	}
}

func TestCanvasTools_CreateForwardsOnlyAuthoringInput(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"canvas_id":     "canvas-1",
		"source_root":   ".kandev/canvases/canvas-1",
		"skill_version": "v1",
	}}
	s := newCanvasTestServer(t, backend)

	result := callTool(t, s, "create_canvas_kandev", map[string]interface{}{
		"title":   "Release dashboard",
		"summary": "Shows deployment health.",
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateCanvas, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Release dashboard", payload["title"])
	assert.Equal(t, "Shows deployment health.", payload["summary"])
	assert.NotContains(t, payload, "task_id")
	assert.NotContains(t, payload, "session_id")
	assert.NotContains(t, payload, "workspace_id")
}

func TestCanvasTools_PublishRequiresCanvasAndSourceRoot(t *testing.T) {
	s := newCanvasTestServer(t, &testBackend{})

	result := callTool(t, s, "publish_canvas_kandev", map[string]interface{}{
		"canvas_id": "canvas-1",
	})

	require.True(t, result.IsError)
}

func TestCanvasTools_SetStatePreservesJSONValue(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"revision": float64(4)}}
	s := newCanvasTestServer(t, backend)

	result := callTool(t, s, "set_canvas_state_kandev", map[string]interface{}{
		"canvas_id":         "canvas-1",
		"key":               "filters",
		"value":             map[string]interface{}{"status": "failed", "limit": float64(3)},
		"expected_revision": float64(3),
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPSetCanvasState, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, map[string]interface{}{"status": "failed", "limit": float64(3)}, payload["value"])
	assert.Equal(t, int64(3), payload["expected_revision"])
}
