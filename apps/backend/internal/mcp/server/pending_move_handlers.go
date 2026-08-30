package mcp

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerPendingMoveCancellationTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("cancel_pending_move_kandev",
			mcp.WithDescription("Cancel one exact reviewed pending workflow move. Every immutable predicate is required; missing, changed, replaced, unauthorized, and cross-workspace targets share one non-leaking result."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("pending_move_id", mcp.Required(), mcp.Description("Exact pending_moves row ID")),
			mcp.WithString("session_id", mcp.Required(), mcp.Description("Exact keyed task session ID")),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("Exact target task ID")),
			mcp.WithString("move_id", mcp.Required(), mcp.Description("Exact deferred move ID")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Exact queued target workflow ID")),
			mcp.WithString("expected_current_workflow_step_id", mcp.Required(), mcp.Description("Exact current workflow step ID")),
			mcp.WithString("expected_target_workflow_step_id", mcp.Required(), mcp.Description("Exact queued target workflow step ID")),
		),
		s.wrapHandler("cancel_pending_move_kandev", s.cancelPendingMoveHandler()),
	)
}

func (s *Server) cancelPendingMoveHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		keys := []string{
			"pending_move_id", "session_id", "task_id", "move_id", "workflow_id",
			"expected_current_workflow_step_id", "expected_target_workflow_step_id",
		}
		payload := make(map[string]string, len(keys))
		for _, key := range keys {
			value, err := req.RequireString(key)
			if err != nil {
				return mcp.NewToolResultError(key + " is required"), nil
			}
			payload[key] = value
		}
		return s.forwardToBackend(ctx, ws.ActionMCPCancelPendingMove, payload)
	}
}
