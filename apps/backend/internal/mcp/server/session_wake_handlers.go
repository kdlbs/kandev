package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) sessionWakeHandler(action string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]string{"task_id": s.taskID, "session_id": s.sessionID, "marker": req.GetString("marker", ""), "prompt": req.GetString("prompt", ""), "cron_expression": req.GetString("cron_expression", ""), "timezone": req.GetString("timezone", ""), "expires_at": req.GetString("expires_at", "")}
		var result map[string]any
		if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) registerSessionWakeTools() {
	s.mcpServer.AddTool(mcp.NewTool("list_session_wakes_kandev", mcp.WithDescription("List recurring wake schedules for this session, including expired schedules.")), s.wrapHandler("list_session_wakes_kandev", s.sessionWakeHandler(ws.ActionMCPListSessionWakes)))
	s.mcpServer.AddTool(mcp.NewTool("upsert_session_wake_kandev", mcp.WithDescription("Create, update, or reactivate one recurring wake identified by marker for this session. cron_expression accepts five-field cron, descriptors, and @every forms; expires_at must be future RFC3339; timezone defaults to UTC and must be an IANA timezone."), mcp.WithString("marker", mcp.Required()), mcp.WithString("prompt", mcp.Required()), mcp.WithString("cron_expression", mcp.Required()), mcp.WithString("expires_at", mcp.Required()), mcp.WithString("timezone")), s.wrapHandler("upsert_session_wake_kandev", s.sessionWakeHandler(ws.ActionMCPUpsertSessionWake)))
	s.mcpServer.AddTool(mcp.NewTool("delete_session_wake_kandev", mcp.WithDescription("Delete a recurring wake from this session. This is idempotent."), mcp.WithString("marker", mcp.Required())), s.wrapHandler("delete_session_wake_kandev", s.sessionWakeHandler(ws.ActionMCPDeleteSessionWake)))
}
