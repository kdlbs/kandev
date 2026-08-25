package mcp

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers the list_task_comments_kandev tool-registration/dispatch
// half of the MCP transport (registerTaskCommentsTool / listTaskCommentsHandler
// / resolveCommentsTaskID / resolveCommentsLimit). The WS-handler half
// (handleListTaskComments, backed by service.HandoffService.ListCommentsForCaller)
// is covered separately in internal/mcp/handlers/handoff_handlers_test.go.

func commentsToolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent")
	return text.Text
}

// AC-OFFICE-AGENT-COMMENT-READS-001.1: registered on the office surface only.
func TestListTaskComments_RegisteredOnOfficeSurfaceOnly(t *testing.T) {
	office := newOfficeModeServer(t, &testBackend{}, "session-1", "task-A")
	_, ok := office.mcpServer.ListTools()["list_task_comments_kandev"]
	assert.True(t, ok, "list_task_comments_kandev must be registered in office mode")

	kanban := newTaskModeServer(t, &testBackend{}, "task-A")
	_, ok = kanban.mcpServer.ListTools()["list_task_comments_kandev"]
	assert.False(t, ok, "list_task_comments_kandev must not be registered in kanban mode")

	config := newTestServer(t, &testBackend{})
	_, ok = config.mcpServer.ListTools()["list_task_comments_kandev"]
	assert.False(t, ok, "list_task_comments_kandev must not be registered in config mode")
}

// AC-005.4/005.6/005.8: absent, empty, whitespace-only, null, and "self" all
// forward an empty task_id to the backend so the service layer resolves it
// to the caller task; caller_task_id is always the bound session task.
func TestListTaskComments_SelfResolvingTaskIDsForwardEmpty(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
	}{
		{"absent", map[string]interface{}{}},
		{"empty string", map[string]interface{}{"task_id": ""}},
		{"whitespace", map[string]interface{}{"task_id": "   "}},
		{"null", map[string]interface{}{"task_id": nil}},
		{"self", map[string]interface{}{"task_id": "self"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"comments": []interface{}{}, "total": 0, "returned": 0, "has_more": false}}
			s := newOfficeModeServer(t, backend, "session-1", "task-A")

			result := callTool(t, s, "list_task_comments_kandev", tc.args)
			require.False(t, result.IsError, "unexpected error result: %+v", result)

			payload, ok := backend.lastPayload.(map[string]interface{})
			require.True(t, ok, "payload = %T, want map[string]interface{}", backend.lastPayload)
			assert.Equal(t, "", payload["task_id"], "task_id must forward empty so the service resolves self")
			assert.Equal(t, "task-A", payload["caller_task_id"])
			assert.Equal(t, ws.ActionMCPListTaskComments, backend.lastAction)
		})
	}
}

// A non-empty string task_id is trimmed and forwarded as the explicit target.
func TestListTaskComments_ExplicitTaskIDForwarded(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"comments": []interface{}{}, "total": 0, "returned": 0, "has_more": false}}
	s := newOfficeModeServer(t, backend, "session-1", "task-A")

	result := callTool(t, s, "list_task_comments_kandev", map[string]interface{}{"task_id": "  task-B  "})
	require.False(t, result.IsError)

	payload := backend.lastPayload.(map[string]interface{})
	assert.Equal(t, "task-B", payload["task_id"])
	assert.Equal(t, "task-A", payload["caller_task_id"])
}

// AC-005.9/AC-005.10: a task_id present with a JSON type that is neither
// string nor null is a validation error naming task_id, produced by the
// handler (not argument validation, since the schema declares no type), and
// must never fall back to the caller task by calling the backend.
func TestListTaskComments_WrongTypeTaskIDRejectedWithoutBackendCall(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
	}{
		{"number", map[string]interface{}{"task_id": float64(42)}},
		{"bool", map[string]interface{}{"task_id": true}},
		{"array", map[string]interface{}{"task_id": []interface{}{"task-A"}}},
		{"object", map[string]interface{}{"task_id": map[string]interface{}{"id": "task-A"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &testBackend{}
			s := newOfficeModeServer(t, backend, "session-1", "task-A")

			result := callTool(t, s, "list_task_comments_kandev", tc.args)
			require.True(t, result.IsError, "wrong-typed task_id must be rejected")
			assert.Empty(t, backend.lastAction, "backend must not be called for a wrong-typed task_id")
			assert.Contains(t, commentsToolResultText(t, result), service.ErrDocumentTaskRequired.Error())
		})
	}
}

// AC-003.4/003.5/003.9/003.11: limit defaulting, clamping, and non-integer
// handling all happen in the handler and never produce an error.
func TestListTaskComments_LimitDefaultingAndClamping(t *testing.T) {
	cases := []struct {
		name  string
		args  map[string]interface{}
		limit int
	}{
		{"absent defaults to 20", map[string]interface{}{}, 20},
		{"null defaults to 20", map[string]interface{}{"limit": nil}, 20},
		{"zero defaults to 20", map[string]interface{}{"limit": float64(0)}, 20},
		{"negative defaults to 20", map[string]interface{}{"limit": float64(-5)}, 20},
		{"non-integer defaults to 20", map[string]interface{}{"limit": 3.5}, 20},
		{"non-numeric defaults to 20", map[string]interface{}{"limit": "20"}, 20},
		{"above 100 clamps to 100", map[string]interface{}{"limit": float64(500)}, 100},
		{"in range passes through", map[string]interface{}{"limit": float64(7)}, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend := &testBackend{response: map[string]interface{}{"comments": []interface{}{}, "total": 0, "returned": 0, "has_more": false}}
			s := newOfficeModeServer(t, backend, "session-1", "task-A")

			result := callTool(t, s, "list_task_comments_kandev", tc.args)
			require.False(t, result.IsError, "unexpected error result: %+v", result)

			payload := backend.lastPayload.(map[string]interface{})
			assert.Equal(t, tc.limit, payload["limit"])
		})
	}
}

// A backend error surfaces as a tool error carrying the backend's message.
func TestListTaskComments_SurfacesBackendError(t *testing.T) {
	backend := &testBackend{err: errors.New("document access denied")}
	s := newOfficeModeServer(t, backend, "session-1", "task-A")

	result := callTool(t, s, "list_task_comments_kandev", map[string]interface{}{"task_id": "task-B"})
	require.True(t, result.IsError)
	assert.Contains(t, commentsToolResultText(t, result), "document access denied")
}

// AC-003.11/AC-005.10: task_id and limit are declared with no JSON Schema
// type constraint, so argument validation can never reject a null or
// wrong-typed value before it reaches the handler.
func TestListTaskComments_SchemaDeclaresNoTypeConstraintOnTaskIDOrLimit(t *testing.T) {
	s := newOfficeModeServer(t, &testBackend{}, "session-1", "task-A")
	props := toolInputProperties(t, s, "list_task_comments_kandev")

	taskID, ok := props["task_id"].(map[string]interface{})
	require.True(t, ok, "task_id must be advertised")
	_, hasType := taskID["type"]
	assert.False(t, hasType, "task_id must carry no JSON Schema type constraint")

	limit, ok := props["limit"].(map[string]interface{})
	require.True(t, ok, "limit must be advertised")
	_, hasType = limit["type"]
	assert.False(t, hasType, "limit must carry no JSON Schema type constraint")

	tool := s.mcpServer.ListTools()["list_task_comments_kandev"].Tool
	assert.NotContains(t, tool.InputSchema.Required, "task_id")
	assert.NotContains(t, tool.InputSchema.Required, "limit")
}
