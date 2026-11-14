package mcp

import (
	"encoding/json"
	"errors"
	"testing"
	"unicode/utf8"

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
}

// A visibility lookup is deliberately a separate compact tool. A granted
// manual task gets only this tool, while caller identity stays backend-owned.
func TestGetTaskVisibilityToolRequiresExplicitCapabilityAndTakesOnlyTaskID(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceOfficeTask, nil, nil)
	office := NewWithProfile(&testBackend{}, "session-A", "caller-A", 10005, newTestLogger(t), "", false, profile)
	_, ok := office.mcpServer.ListTools()["get_task_visibility_kandev"]
	require.False(t, ok)

	manualProfile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{
		mcpprofile.CapabilityTaskVisibilityRead,
	}, nil)
	manual := NewWithProfile(&testBackend{}, "session-A", "caller-A", 10005, newTestLogger(t), "", false, manualProfile)
	props := toolInputProperties(t, manual, "get_task_visibility_kandev")
	assert.Contains(t, props, "task_id")
	assert.NotContains(t, props, "caller_task_id")
	assert.NotContains(t, props, "caller_session_id")
	assert.NotContains(t, props, "workspace_id")
	assert.NotContains(t, props, "related_read_scope")

	kanban := newTaskModeServer(t, &testBackend{}, "caller-A")
	assert.NotContains(t, kanban.mcpServer.ListTools(), "get_task_visibility_kandev")
	backend := &testBackend{response: map[string]interface{}{"task_id": "target"}}
	manual = NewWithProfile(backend, "session-A", "caller-A", 10005, newTestLogger(t), "", false, manualProfile)
	assert.Contains(t, manual.mcpServer.ListTools(), "get_task_visibility_kandev")
	props = toolInputProperties(t, manual, "get_task_visibility_kandev")
	assert.Equal(t, map[string]interface{}{"task_id": props["task_id"]}, props)
	result := callTool(t, manual, "get_task_visibility_kandev", map[string]interface{}{"task_id": "target"})
	require.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "target", payload["task_id"])
	assert.Equal(t, "task-visibility", payload["related_read_scope"])
	assert.Equal(t, "caller-A", payload["caller_task_id"])
	assert.Equal(t, "session-A", payload["caller_session_id"])
}

func TestGetTaskVisibilityMissingLocalContextUsesTargetUnavailableDenial(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{
		mcpprofile.CapabilityTaskVisibilityRead,
	}, nil)
	s := NewWithProfile(&testBackend{}, "", "", 10005, newTestLogger(t), "", false, profile)
	result := callTool(t, s, "get_task_visibility_kandev", map[string]interface{}{"task_id": "target"})
	require.True(t, result.IsError)
	structured, ok := result.StructuredContent.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, ws.ErrorCodeNotFound, structured["code"])
	assert.Equal(t, "target unavailable", structured["message"])
}

func TestGetTaskVisibilityUsesCoreBuiltinPackage(t *testing.T) {
	profile := packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageRichOutput)
	profile = profile.WithCapability(mcpprofile.CapabilityTaskVisibilityRead)
	s := NewWithProfile(&testBackend{}, "session-A", "task", 10005, newTestLogger(t), "", false, profile)
	require.Contains(t, s.mcpServer.ListTools(), "get_task_visibility_kandev")
	require.NotContains(t, s.builtinPackages[mcpprofile.PackageRichOutput].registry.ListTools(), "get_task_visibility_kandev")

	profile = packageTestProfile(mcpprofile.PackageCore)
	s = NewWithProfile(&testBackend{}, "session-A", "task", 10005, newTestLogger(t), "", false, profile)
	require.NotContains(t, s.mcpServer.ListTools(), "get_task_visibility_kandev")
}

// TestListRelatedTasks_ToolDescriptionStatesAccessBoundary guards the
// advertised access contract from issue #4921: the tool accepts another target
// only through a relation gate in the same workspace, and the verbose flag is
// a presentation option that cannot widen it.
func TestListRelatedTasks_ToolDescriptionStatesAccessBoundary(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-A")

	tool, ok := s.mcpServer.ListTools()["list_related_tasks_kandev"]
	require.True(t, ok)

	description := tool.Tool.Description
	for _, want := range []string{
		"task_id defaults to self",
		"ancestor",
		"descendant",
		"sibling with a shared non-empty parent",
		"blocker",
		"same workspace",
		"Unrelated tasks are denied",
		"verbose changes output only",
	} {
		assert.Contains(t, description, want)
	}
	assert.LessOrEqual(t, utf8.RuneCountInString(description), 500,
		"the relation rule must fit the catalog focus limit")

	props := toolInputProperties(t, s, "list_related_tasks_kandev")
	assert.Len(t, props, 2, "the advertised arguments must stay task_id and verbose")

	taskID, ok := props["task_id"].(map[string]interface{})
	require.True(t, ok)
	taskIDHelp, _ := taskID["description"].(string)
	assert.Contains(t, taskIDHelp, "current task")
	assert.Contains(t, taskIDHelp, "same workspace")

	verbose, ok := props["verbose"].(map[string]interface{})
	require.True(t, ok)
	verboseHelp, _ := verbose["description"].(string)
	assert.Contains(t, verboseHelp, "output only")
}

// TestListRelatedTasks_CallerBinding confirms the bound session task remains
// the caller for every target form, so authorization never trusts arguments.
func TestListRelatedTasks_CallerBinding(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]interface{}
		wantTarget string
	}{
		{name: "omitted target", args: map[string]interface{}{}, wantTarget: "task-A"},
		{name: "self target", args: map[string]interface{}{"task_id": "self"}, wantTarget: "task-A"},
		{name: "explicit target", args: map[string]interface{}{"task_id": "task-S1"}, wantTarget: "task-S1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{response: relatedTasksResponse("desc")}
			s := newTaskModeServer(t, backend, "task-A")

			result := callTool(t, s, "list_related_tasks_kandev", tt.args)
			require.False(t, result.IsError)

			payload, ok := backend.lastPayload.(map[string]string)
			require.True(t, ok)
			assert.Equal(t, tt.wantTarget, payload["task_id"])
			assert.Equal(t, "task-A", payload["caller_task_id"])
		})
	}
}

func TestListRelatedTasks_RejectsCallerOverride(t *testing.T) {
	backend := &testBackend{response: relatedTasksResponse("desc")}
	s := newTaskModeServer(t, backend, "task-A")

	props := toolInputProperties(t, s, "list_related_tasks_kandev")
	assert.NotContains(t, props, "caller_task_id")

	result := callTool(t, s, "list_related_tasks_kandev", map[string]interface{}{
		"task_id":        "task-S1",
		"caller_task_id": "task-other",
	})
	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "rejected arguments must not reach the backend")
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `unknown arguments: "caller_task_id"`)
}

// TestListRelatedTasks_VerboseDoesNotBypassForbidden pins the ordering of the
// access gate and the description filter: a denied target is an error for
// every verbose form, never an empty successful projection.
func TestListRelatedTasks_VerboseDoesNotBypassForbidden(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{name: "omitted", args: map[string]interface{}{}},
		{name: "false", args: map[string]interface{}{"verbose": false}},
		{name: "true", args: map[string]interface{}{"verbose": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &testBackend{err: errors.New("FORBIDDEN: document access denied")}
			s := newTaskModeServer(t, backend, "task-A")
			args := map[string]interface{}{"task_id": "task-unrelated"}
			for key, value := range tt.args {
				args[key] = value
			}

			result := callTool(t, s, "list_related_tasks_kandev", args)
			require.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			content, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			assert.Contains(t, content.Text, "FORBIDDEN")
			for _, leaked := range []string{"children", "siblings", "blockers", "description"} {
				assert.NotContains(t, content.Text, leaked)
			}
		})
	}
}
