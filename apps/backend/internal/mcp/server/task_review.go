package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerTaskReviewTools() {
	s.mcpServer.AddTool(mcp.NewTool("run_code_review_kandev",
		mcp.WithDescription("Start an independent review of task changes. Read results with get_code_review_kandev."),
		mcp.WithString("task_id", mcp.Description("Target task, defaulting to the current task.")),
		mcp.WithString("agent_profile_id", mcp.Description("Optional reviewer profile; the writer remains excluded.")),
		mcp.WithString("repository_id", mcp.Description("Optional repository scope.")),
	), s.wrapHandler("run_code_review_kandev", s.taskReviewActionHandler(ws.ActionTaskReviewRun)))
	s.mcpServer.AddTool(mcp.NewTool("get_code_review_kandev",
		mcp.WithDescription("Read task review runs and findings."),
		mcp.WithString("task_id", mcp.Description("Target task, defaulting to the current task.")),
	), s.wrapHandler("get_code_review_kandev", s.taskReviewActionHandler(ws.ActionTaskReviewGet)))
}

func (s *Server) taskReviewActionHandler(action string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		payload := map[string]interface{}{"task_id": taskID}
		for _, key := range []string{"agent_profile_id", "repository_id"} {
			if value, exists := req.GetArguments()[key]; exists {
				payload[key] = value
			}
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}
