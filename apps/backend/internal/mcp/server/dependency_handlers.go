package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// selfTaskSentinel is the "my own task" shorthand create_task_kandev already
// accepts for parent_id; the dependency tools take it for task_id too.
const selfTaskSentinel = "self"

// stringArrayArg reads a string array argument, tolerating the []interface{}
// shape JSON decoding produces and skipping non-string entries rather than
// failing the whole call.
func stringArrayArg(req mcp.CallToolRequest, key string) []string {
	raw, ok := req.GetArguments()[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if s, ok := entry.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// addTaskDependencyHandler backs add_task_dependency_kandev.
func (s *Server) addTaskDependencyHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatchDependencyMutation(ctx, req, ws.ActionMCPAddTaskDependency)
	}
}

// removeTaskDependencyHandler backs remove_task_dependency_kandev.
func (s *Server) removeTaskDependencyHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.dispatchDependencyMutation(ctx, req, ws.ActionMCPRemoveTaskDependency)
	}
}

// dispatchDependencyMutation resolves the target task (defaulting to the
// caller's own) and forwards the edge mutation. Both tools share this because
// they differ only in the action they send.
func (s *Server) dispatchDependencyMutation(
	ctx context.Context, req mcp.CallToolRequest, action string,
) (*mcp.CallToolResult, error) {
	dependsOn, err := req.RequireString("depends_on_task_id")
	if err != nil {
		return mcp.NewToolResultError("depends_on_task_id is required"), nil
	}
	taskID := req.GetString(mcpKeyTaskID, "")
	if taskID == "" || taskID == selfTaskSentinel {
		if s.taskID == "" {
			return mcp.NewToolResultError("task_id is required: no current task context"), nil
		}
		taskID = s.taskID
	}
	// In a task-bound session the dependent end is the server's own task.
	// Ownership comes from the AgentExecution the session was bound to, never
	// from an agent-supplied id, so an explicit task_id may only confirm it —
	// the same rule add_branch_to_task_kandev applies. The predecessor
	// (depends_on_task_id) is necessarily another task and stays free; the
	// backend authorizes both ends against the session's identity.
	if s.taskID != "" && taskID != s.taskID {
		return mcp.NewToolResultError(
			"task_id must be your current task; use depends_on_task_id to name the other task",
		), nil
	}
	payload := map[string]interface{}{
		mcpKeyTaskID:         taskID,
		"depends_on_task_id": dependsOn,
	}
	var result map[string]interface{}
	if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
