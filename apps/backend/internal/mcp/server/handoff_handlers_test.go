package mcp

import (
	"encoding/json"
	"errors"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relatedTasksResponse is a related-task graph shaped like the backend's, with
// one description long enough that dropping it is visible in the payload size.
func relatedTasksResponse(description string) map[string]interface{} {
	return map[string]interface{}{
		"task": map[string]interface{}{
			"id": "task-A", "title": "Parent work", "state": "RUNNING", "description": description,
		},
		"parent": map[string]interface{}{
			"id": "task-P", "title": "Epic", "state": "RUNNING", "description": description,
		},
		"children": []interface{}{
			map[string]interface{}{
				"id": "task-C1", "title": "Child one", "state": "CREATED",
				"description": description, "document_keys": []interface{}{"spec"},
			},
			map[string]interface{}{
				"id": "task-C2", "title": "Child two", "state": "DONE", "description": description,
			},
		},
		"siblings": []interface{}{
			map[string]interface{}{
				"id": "task-S1", "title": "Sibling task", "state": "RUNNING", "description": description,
			},
		},
		"blockers":   []interface{}{},
		"blocked_by": []interface{}{},
	}
}

func relatedTasksText(t *testing.T, args map[string]interface{}, description string) string {
	t.Helper()
	backend := &testBackend{response: relatedTasksResponse(description)}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_related_tasks_kandev", args)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestListRelatedTasks_OmitsDescriptionsByDefault(t *testing.T) {
	description := "a long task description that dominates the response size"

	text := relatedTasksText(t, map[string]interface{}{}, description)

	assert.NotContains(t, text, description)
	assert.NotContains(t, text, `"description"`)
	// The compact projection must still carry everything the caller navigates by.
	for _, want := range []string{"task-A", "task-P", "task-C1", "task-C2", "task-S1", "Child one", "CREATED", "spec"} {
		assert.Contains(t, text, want)
	}
}

func TestListRelatedTasks_VerboseKeepsDescriptions(t *testing.T) {
	description := "Depends on: task-C1"

	text := relatedTasksText(t, map[string]interface{}{"verbose": true}, description)

	assert.Contains(t, text, description)
	assert.Contains(t, text, `"description"`)
}

func TestListRelatedTasks_ToolSchemaExposesVerbose(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-A")

	tool, ok := s.mcpServer.ListTools()["list_related_tasks_kandev"]
	require.True(t, ok)

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	verbose, ok := props["verbose"].(map[string]interface{})
	require.True(t, ok, "verbose must be advertised so callers can opt into descriptions")
	assert.Equal(t, "boolean", verbose["type"])

	required, _ := parsed["required"].([]interface{})
	assert.NotContains(t, required, "verbose")
	assert.Contains(t, tool.Tool.Description, "verbose=true")
	assert.Contains(t, tool.Tool.Description, "relation-scoped by default")
	assert.Contains(t, tool.Tool.Description, "authorized Office Coordinator")
	assert.Contains(t, tool.Tool.Description, "workspace-task-tree-read")
	assert.Contains(t, tool.Tool.Description, "does not grant document keys or descriptions")
	assert.NotContains(t, tool.Tool.Description, "may inspect another task in the same workspace")
}

func TestListRelatedTasks_AttestsProfileScopeAndCallerIdentity(t *testing.T) {
	backend := &testBackend{response: relatedTasksResponse("")}
	profile := mcpprofile.New(mcpprofile.SurfaceOfficeTask, []mcpprofile.Capability{
		mcpprofile.CapabilityWorkspaceTaskTreeRead,
	}, nil)
	s := NewWithProfile(backend, "session-A", "task-A", 10005, newTestLogger(t), "", false, profile)

	result := callTool(t, s, "list_related_tasks_kandev", map[string]interface{}{
		"task_id": "task-B", "verbose": true,
	})
	require.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-B", payload["task_id"])
	assert.Equal(t, "task-A", payload["caller_task_id"])
	assert.Equal(t, "session-A", payload["caller_session_id"])
	assert.Equal(t, "office-task", payload["mcp_surface"])
	assert.Equal(t, "workspace-task-tree", payload["related_read_scope"])
	assert.Equal(t, true, payload["verbose"])

	props := toolInputProperties(t, s, "list_related_tasks_kandev")
	assert.NotContains(t, props, "related_read_scope")
}

func TestListRelatedTasks_RejectsExplicitTargetWithoutAttestedCallerIdentity(t *testing.T) {
	backend := &testBackend{response: relatedTasksResponse("secret")}
	profile := mcpprofile.New(mcpprofile.SurfaceOfficeTask, []mcpprofile.Capability{
		mcpprofile.CapabilityWorkspaceTaskTreeRead,
	}, nil)
	s := NewWithProfile(backend, "", "", 10005, newTestLogger(t), "", false, profile)

	result := callTool(t, s, "list_related_tasks_kandev", map[string]interface{}{
		"task_id": "known-target", "verbose": true,
	})

	require.True(t, result.IsError)
	assert.Nil(t, backend.lastPayload, "missing attested identity must be rejected before forwarding")
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, `"code": "FORBIDDEN"`)
	assert.Contains(t, text.Text, `"reason": "related_task_scope_required"`)
	assert.NotContains(t, text.Text, "known-target")
	assert.NotContains(t, text.Text, "secret")
}

func TestListRelatedTasks_RejectsExplicitTargetWithoutAttestedSession(t *testing.T) {
	backend := &testBackend{response: relatedTasksResponse("secret")}
	s := New(backend, "", "caller-task", 10005, newTestLogger(t), "", false, ModeTask, []string{"github"})

	result := callTool(t, s, "list_related_tasks_kandev", map[string]interface{}{
		"task_id": "known-target", "verbose": true,
	})

	require.True(t, result.IsError)
	assert.Nil(t, backend.lastPayload, "missing attested session must be rejected before forwarding")
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, `"code": "FORBIDDEN"`)
	assert.Contains(t, text.Text, `"reason": "related_task_scope_required"`)
	assert.NotContains(t, text.Text, "known-target")
	assert.NotContains(t, text.Text, "secret")
}

func TestRelatedReadScopeRequiresOfficeSurface(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{
		mcpprofile.CapabilityWorkspaceTaskTreeRead,
	}, nil)
	s := NewWithProfile(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, profile)
	assert.Equal(t, "relation", s.relatedReadScope())
}

// TestListRelatedTasks_SurfacesStructuredDenialReason is a regression test for
// a bug where a backend denial's structured Details (e.g.
// details.reason=related_task_scope_required) were discarded between the
// BackendClient and the MCP tool result, leaving the caller with only a
// flattened "backend error [FORBIDDEN]: ..." string and no machine-readable
// reason to branch on.
func TestListRelatedTasks_SurfacesStructuredDenialReason(t *testing.T) {
	backend := &testBackend{err: &BackendError{
		Code:    ws.ErrorCodeForbidden,
		Message: "related task access denied",
		Details: map[string]interface{}{"reason": "related_task_scope_required"},
	}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_related_tasks_kandev", map[string]interface{}{"task_id": "task-B"})
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, `"reason": "related_task_scope_required"`)

	structured, ok := result.StructuredContent.(map[string]interface{})
	require.True(t, ok, "structured content must carry the denial details for programmatic branching")
	details, ok := structured["details"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "related_task_scope_required", details["reason"])
}

// TestListRelatedTasks_FallsBackToPlainErrorWithoutDetails preserves the
// pre-existing behavior for backend errors that carry no structured details.
func TestListRelatedTasks_FallsBackToPlainErrorWithoutDetails(t *testing.T) {
	backend := &testBackend{err: errors.New("boom")}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_related_tasks_kandev", map[string]interface{}{"task_id": "task-B"})
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "boom", text.Text)
	assert.Nil(t, result.StructuredContent)
}
