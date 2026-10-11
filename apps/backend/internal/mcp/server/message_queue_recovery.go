package mcp

import (
	"context"
	"encoding/json"
	taskcontract "github.com/kandev/kandev/internal/task/contract"
	"math"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerQueueRecoveryTools() {
	definitions := []struct {
		name, action, description string
		child, page               bool
	}{
		{"get_terminal_session_message_queue_kandev", ws.ActionMCPGetTerminalSessionMessageQueue, "Read bounded immutable FIFO body fragments from one exact terminal predecessor on your own task. Never consumes or replays entries. Changed snapshots reject continuation.", false, true},
		{"get_child_queue_inventory_kandev", ws.ActionMCPGetChildQueueInventory, "Read bounded body-free queue manifests for all sessions of one exact same-workspace direct child. No queue mutation or retirement acknowledgement.", true, false},
		{"get_child_session_message_queue_kandev", ws.ActionMCPGetChildSessionMessageQueue, "Read bounded FIFO body fragments from one exact session on your same-workspace direct child. Pin the inventory digest on first page. Changed snapshots reject continuation.", true, true},
	}
	for _, definition := range definitions {
		options := []mcp.ToolOption{mcp.WithDescription(definition.description), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithIdempotentHintAnnotation(true), mcp.WithOpenWorldHintAnnotation(false)}
		if definition.child {
			options = append(options, mcp.WithString("task_id", mcp.Required(), mcp.Description("Exact direct child task UUID")))
		}
		if definition.page {
			options = append(options, mcp.WithString("session_id", mcp.Required(), mcp.Description("Exact target session UUID")), mcp.WithString("cursor", mcp.Description("Opaque next_cursor from the preceding page")), mcp.WithString("expected_snapshot_sha256", mcp.Description("Pin first page to the inventory SHA-256 digest")), mcp.WithNumber("max_body_bytes", mcp.Description("UTF-8 body budget: 1024..262144, default 65536")))
		}
		s.mcpServer.AddTool(mcp.NewTool(definition.name, options...), s.wrapSensitiveHandler(definition.name, s.queueRecoveryHandler(definition.action, definition.child, definition.page)))
	}
}

func (s *Server) queueRecoveryHandler(action string, child, page bool) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{"task_id": s.taskID}
		if child {
			taskID, err := request.RequireString("task_id")
			if err != nil {
				return mcp.NewToolResultError("task_id is required"), nil
			}
			payload["task_id"] = taskID
		}
		if page {
			sessionID, err := request.RequireString("session_id")
			if err != nil {
				return mcp.NewToolResultError("session_id is required"), nil
			}
			payload["session_id"] = sessionID
			copyOptionalStringArg(payload, request, "cursor")
			copyOptionalStringArg(payload, request, "expected_snapshot_sha256")
			if value, exists := request.GetArguments()["max_body_bytes"]; exists {
				n, ok := value.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n < 1024 || n > 262144 {
					return mcp.NewToolResultError("max_body_bytes must be an integer between 1024 and 262144"), nil
				}
				payload["max_body_bytes"] = int(n)
			}
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return boundedQueueRecoveryToolResult(result), nil
	}
}

func boundedQueueRecoveryToolResult(payload map[string]interface{}) *mcp.CallToolResult {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError("queue recovery response unavailable")
	}
	result := mcp.NewToolResultText(string(encoded))
	wire, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError("queue recovery response unavailable")
	}
	if len(wire) > taskcontract.MaxQueueRecoveryResponseBytes {
		return mcp.NewToolResultError("queue recovery limit exceeded")
	}
	return result
}
