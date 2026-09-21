package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestAssistantBrokerForwardsOnlyNamedOperations(t *testing.T) {
	t.Setenv("KANDEV_ORCHESTRATOR_SCOPE", "private")
	calls := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer synthetic", r.Header.Get("Authorization"))
		require.Equal(t, "/api/v1/orchestration/runtime/tasks/target/manage", r.URL.Path)
		require.Equal(t, "run", r.Header.Get("X-Kandev-Run-Id"))
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"operation_outcome_unknown"}`))
	}))
	defer backend.Close()
	c := &kandevClient{apiURL: backend.URL, apiKey: "synthetic", runID: "run", http: backend.Client()}
	s := newAssistantMCP(c)
	require.Len(t, s.ListTools(), len(assistantBrokerTools))
	require.Nil(t, s.GetTool("shell"))
	require.Nil(t, s.GetTool("invoke_plugin"))
	for _, name := range []string{"improvements", "improvement", "maintenance_file", "maintenance_artifact", "maintenance"} {
		require.NotNil(t, s.GetTool(name), name)
	}
	for _, name := range []string{"grant_maintenance", "review_improvement", "publish_repair"} {
		require.Nil(t, s.GetTool(name), name)
	}
	definition := assistantBrokerTool{name: "manage_task", method: "POST", path: "/runtime/tasks/:id/manage"}
	result, err := callAssistantBroker(c, definition, map[string]any{"id": "target", "request": map[string]any{"action": "start"}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, 1, calls, "unknown delivery must not be retried")
	for _, id := range []string{"../settings", "../..", "%2f", "x?admin=1", "x/y"} {
		result, err = callAssistantBroker(c, definition, map[string]any{"id": id})
		require.NoError(t, err)
		require.True(t, result.IsError)
	}
	require.Equal(t, 1, calls)
}

func TestWorkspaceBrokerAdvertisesUsableTaskControls(t *testing.T) {
	t.Setenv("KANDEV_ORCHESTRATOR_SCOPE", "workspace")
	s := newAssistantMCP(&kandevClient{})
	for _, name := range []string{"workspace", "workspace_tasks", "task_details", "capabilities", "memory", "create_task", "manage_task", "task_status"} {
		require.NotNil(t, s.GetTool(name), name)
	}
	for _, name := range []string{"create_objective", "objectives", "maintenance", "workspace_links", "answer_question"} {
		require.Nil(t, s.GetTool(name), name)
	}
	for _, action := range []string{"edit", "move", "archive", "delete", "assign", "start", "stop", "message"} {
		require.Contains(t, s.GetTool("manage_task").Tool.Description, action)
	}
	require.Contains(t, s.GetTool("create_task").Tool.Description, "No objective")
}

func TestAssistantBrokerReportsEmptyHTTPFailure(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }))
	defer backend.Close()
	result, err := callAssistantBroker(&kandevClient{apiURL: backend.URL, http: backend.Client()}, assistantBrokerTool{method: "GET", path: "/runtime/objectives"}, nil)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content, mcp.TextContent{Type: "text", Text: "Kandev returned HTTP 404 (Not Found)"})
}

func TestOrchestratorBrokerAdvertisesWorkspaceAdministration(t *testing.T) {
	for _, scope := range []string{"workspace", "assistant"} {
		t.Run(scope, func(t *testing.T) {
			t.Setenv("KANDEV_ORCHESTRATOR_SCOPE", scope)
			tool := newAssistantMCP(&kandevClient{}).GetTool("manage_workspace")
			require.NotNil(t, tool)
			for _, resource := range []string{"workflow", "step", "repository", "configuration"} {
				require.Contains(t, tool.Tool.Description, resource)
			}
		})
	}
}
