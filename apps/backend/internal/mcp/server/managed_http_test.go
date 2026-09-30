package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	protocolmcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestManagedHTTPHandlerExposesOnlyTaskProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler, err := NewManagedHTTPHandler(
		NewManagedDispatcherBackendClient(ws.NewDispatcher(), newTestLogger(t)),
		"session-1", "task-1", newTestLogger(t),
		mcpprofile.New(mcpprofile.SurfaceManagedTask, []mcpprofile.Capability{mcpprofile.CapabilityUserQuestion}, nil),
		"/managed/grant-1",
	)
	require.NoError(t, err)
	router.Any("/managed/grant-1", gin.WrapH(handler))
	server := httptest.NewServer(router)
	defer server.Close()

	initialized := postJSONRPC(t, server.URL+"/managed/grant-1", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`, "")
	require.Equal(t, http.StatusOK, initialized.statusCode, initialized.body)
	listed := postJSONRPC(t, server.URL+"/managed/grant-1", `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, initialized.sessionID)
	require.Equal(t, http.StatusOK, listed.statusCode, listed.body)

	line := extractDataLine(listed.body)
	require.NotEmpty(t, line, listed.body)
	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(line), &response))
	names := make([]string, 0, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "ask_user_question_kandev")
	require.Contains(t, names, "step_complete_kandev")
	require.NotContains(t, names, "create_task_kandev")
	require.NotContains(t, names, "list_workspaces_kandev")
	require.NotContains(t, names, "run_shell_command")
}

func TestManagedHTTPHandlerRejectsExternalSurface(t *testing.T) {
	_, err := NewManagedHTTPHandler(
		NewExternalDispatcherBackendClient(ws.NewDispatcher(), newTestLogger(t)),
		"session-1", "task-1", newTestLogger(t),
		mcpprofile.New(mcpprofile.SurfaceExternal, nil, nil), "/managed/grant-1",
	)
	require.Error(t, err)
}

func TestManagedHTTPHandlerRejectsCrossTaskPlanArguments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	dispatcher := ws.NewDispatcher()
	dispatcher.RegisterFunc(ws.ActionMCPCreateTaskPlan, func(_ context.Context, message *ws.Message) (*ws.Message, error) {
		called = true
		return ws.NewResponse(message.ID, message.Action, map[string]any{"saved": true})
	})
	backend := NewManagedDispatcherBackendClient(dispatcher, newTestLogger(t))
	handler, err := NewManagedHTTPHandler(
		backend, "session-1", "task-1", newTestLogger(t),
		mcpprofile.New(mcpprofile.SurfaceManagedTask, nil, nil), "/managed/grant-1",
	)
	require.NoError(t, err)
	router := gin.New()
	router.Any("/managed/grant-1", gin.WrapH(handler))
	server := httptest.NewServer(router)
	defer server.Close()

	initialized := postJSONRPC(t, server.URL+"/managed/grant-1", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`, "")
	require.Equal(t, http.StatusOK, initialized.statusCode, initialized.body)
	request := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_task_plan_kandev","arguments":{"task_id":"task-other","content":"forged"}}}`
	response := postJSONRPC(t, server.URL+"/managed/grant-1", request, initialized.sessionID)
	require.Equal(t, http.StatusOK, response.statusCode, response.body)
	require.Contains(t, response.body, "only their current task")
	require.False(t, called, "forged task target reached backend dispatch")
}

func TestManagedTaskToolArgumentsAreNotLogged(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	server := newServerWithProfile(
		NewManagedDispatcherBackendClient(ws.NewDispatcher(), log), "session-1", "task-1", log, "",
		mcpprofile.New(mcpprofile.SurfaceManagedTask, nil, nil),
	)
	server.managedRuntime = true
	secretPrompt := "question-text-canary"
	wrapper := server.wrapHandler("create_task_plan_kandev", func(context.Context, protocolmcp.CallToolRequest) (*protocolmcp.CallToolResult, error) {
		return protocolmcp.NewToolResultText("saved"), nil
	})
	_, err = wrapper(context.Background(), protocolmcp.CallToolRequest{Params: protocolmcp.CallToolParams{
		Name: "create_task_plan_kandev", Arguments: map[string]any{"content": secretPrompt},
	}})
	require.NoError(t, err)
	entries := observed.All()
	for _, entry := range entries {
		require.NotContains(t, entry.ContextMap(), "args")
		require.NotContains(t, entry.ContextMap(), "result")
	}
	require.NotEmpty(t, entries)
	require.NotContains(t, observed.FilterMessage("MCP tool call").All()[0].ContextMap(), secretPrompt)
}
