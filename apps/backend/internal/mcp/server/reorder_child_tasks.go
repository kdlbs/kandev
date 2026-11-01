package mcp

import (
	"context"
	"encoding/json"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerReorderChildTasksTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("reorder_child_tasks_kandev",
			mcp.WithDescription(`Set the processing order of your same-workspace direct children in one workflow step and band (admitted or queued). Use ordering for scheduling; use blocked_by only when a task consumes another task's output. placement="in_place" (default) permutes the named tasks within their current slots; placement="front" puts them ahead of the rest, preserving the remaining relative order. The board and queued admission use the new order. A membership conflict is retried once, then reported. Returns the band's task IDs and positions.`),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithArray("ordered_task_ids", mcp.Required(), mcp.Items(map[string]any{"type": "string"}),
				mcp.Description("Your direct child task IDs (full UUIDs) in the desired order, first = processed first.")),
			mcp.WithString("placement",
				mcp.Enum("in_place", "front"),
				mcp.DefaultString("in_place"),
				mcp.Description(`"in_place" (default): reorder only within the named tasks' current slots. "front": move the named tasks to the head of their band.`)),
		),
		s.wrapHandler("reorder_child_tasks_kandev", s.reorderChildTasksHandler()),
	)
}

func (s *Server) reorderChildTasksHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if s.taskID == "" {
			return mcp.NewToolResultError("trusted task sender is required"), nil
		}
		orderedIDs, err := req.RequireStringSlice("ordered_task_ids")
		if err != nil || len(orderedIDs) == 0 {
			return mcp.NewToolResultError("ordered_task_ids is required"), nil
		}
		// Build a fresh payload so callers cannot override trusted sender
		// attribution; the backend authorizes against sender_task_id.
		payload := map[string]interface{}{
			"ordered_task_ids": orderedIDs,
			"sender_task_id":   s.taskID,
		}
		copyOptionalStringArg(payload, req, "placement")
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPReorderChildTasks, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
