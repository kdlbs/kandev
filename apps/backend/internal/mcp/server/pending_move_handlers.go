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
		s.wrapAuditedSensitiveHandler("cancel_pending_move_kandev", s.cancelPendingMoveHandler()),
	)
}

func (s *Server) cancelPendingMoveHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ws.ActionMCPCancelPendingMove, req.GetRawArguments())
	}
}

// registerPendingMoveReadTool registers the read-only census companion to
// cancel_pending_move_kandev. It requires only the target task ID and returns
// the immutable row/session/current-lane/target identity a Coordinator needs
// to call the cancellation safely — an authorized, empty result reports
// found=false rather than a leaking or ambiguous null.
func (s *Server) registerPendingMoveReadTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("read_pending_move_kandev",
			mcp.WithDescription("Read the immutable identity of one task's currently armed pending workflow move, if any, without mutating state or resuming its session. Returns found=false when no row is armed. Use the returned fields to call cancel_pending_move_kandev safely."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("Target task ID to inspect for an armed pending move")),
		),
		s.wrapAuditedSensitiveHandler("read_pending_move_kandev", s.readPendingMoveHandler()),
	)
}

func (s *Server) readPendingMoveHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ws.ActionMCPReadPendingMove, req.GetRawArguments())
	}
}
