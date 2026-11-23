package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Operation readback is a kanban task capability, independent of the fixed
// automation catalog that also composes the ordinary message tool.
func (s *Server) registerGetTaskMessageOperationTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_message_operation_kandev",
			mcp.WithDescription(`Read a retained message_task_kandev operation without another dispatch. Pending or outcome_unknown forbids resend. Committed means dispatch was accepted; queued does not prove a provider reply. Only dispatch_rejected is retry_safe. No retained record is not proof that no historical delivery occurred. Scoped to the calling kanban task/session and never returns prompt content.`),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("operation_id", mcp.Required(), mcp.Description("The same operation_id supplied to the send whose outcome needs readback")),
		),
		s.wrapHandler("get_task_message_operation_kandev", s.getTaskMessageOperationHandler()),
	)
}

func (s *Server) getTaskMessageOperationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		operationID, err := req.RequireString("operation_id")
		if err != nil {
			return mcp.NewToolResultError("operation_id is required"), nil
		}
		payload := map[string]interface{}{
			"task_id": s.taskID, "operation_id": operationID,
			"sender_task_id": s.taskID, "sender_session_id": s.sessionID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGetTaskMessageOperation, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
