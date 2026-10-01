package mcp

import (
	"context"
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerProjectCoordinatorTools() {
	s.mcpServer.AddTool(mcp.NewToolWithRawSchema(
		"get_agent_project_kandev", "Read the current Agent Project and its coordinator status.",
		json.RawMessage(`{"type":"object","properties":{}}`),
	), s.wrapHandler("get_agent_project_kandev", s.projectReadHandler(ws.ActionMCPGetAgentProject)))
	s.mcpServer.AddTool(mcp.NewToolWithRawSchema(
		"list_agent_project_workers_kandev", "List direct workers in the current Agent Project with task, session, result, and repository branch status.",
		json.RawMessage(`{"type":"object","properties":{}}`),
	), s.wrapHandler("list_agent_project_workers_kandev", s.projectReadHandler(ws.ActionMCPListAgentProjectWorkers)))
	s.mcpServer.AddTool(
		mcp.NewTool("create_agent_project_worker_kandev",
			mcp.WithDescription("Create and start a direct project worker. Choose economy or frontier. Omit repositories to use every repository selected on the project; each explicit base_branch must be an available remote branch."),
			mcp.WithString("tier", mcp.Required(), mcp.Enum("economy", "frontier"), mcp.Description("Worker profile tier.")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Short worker task title.")),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("The worker's task instructions.")),
			mcp.WithArray("repositories", mcp.Description("Optional repository subset and remote base branches."), mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"repository_id": map[string]any{"type": "string"},
					"base_branch":   map[string]any{"type": "string"},
				},
				"required": []string{"repository_id"},
			})),
		),
		s.wrapHandler("create_agent_project_worker_kandev", s.createProjectWorkerHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("message_agent_project_worker_kandev",
			mcp.WithDescription("Send a follow-up message to one direct worker in this project."),
			mcp.WithString("worker_task_id", mcp.Required(), mcp.Description("The worker task ID from list_agent_project_workers_kandev.")),
			mcp.WithString("message", mcp.Required(), mcp.Description("Message for the worker.")),
		),
		s.wrapHandler("message_agent_project_worker_kandev", s.messageProjectWorkerHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("stop_agent_project_worker_kandev",
			mcp.WithDescription("Stop one direct worker in this project."),
			mcp.WithString("worker_task_id", mcp.Required(), mcp.Description("The worker task ID from list_agent_project_workers_kandev.")),
		),
		s.wrapHandler("stop_agent_project_worker_kandev", s.stopProjectWorkerHandler()),
	)
}

func (s *Server) registerProjectWorkerTools() {
	s.mcpServer.AddTool(mcp.NewToolWithRawSchema(
		"get_agent_project_task_kandev", "Read this worker's project task, session outcome, and repository branch status.",
		json.RawMessage(`{"type":"object","properties":{}}`),
	), s.wrapHandler("get_agent_project_task_kandev", s.projectReadHandler(ws.ActionMCPGetAgentProjectTask)))
}

func (s *Server) projectReadHandler(action string) server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, action, map[string]interface{}{}, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return projectResult(result), nil
	}
}

func (s *Server) createProjectWorkerHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tier, err := req.RequireString("tier")
		if err != nil {
			return mcp.NewToolResultError("tier is required"), nil
		}
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError("title is required"), nil
		}
		prompt, err := req.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}
		payload := map[string]interface{}{"tier": tier, "title": title, "prompt": prompt}
		if repositories, ok := req.GetArguments()["repositories"]; ok && repositories != nil {
			payload["repositories"] = repositories
		}
		return s.callProjectTool(ctx, ws.ActionMCPCreateAgentProjectWorker, payload)
	}
}

func (s *Server) messageProjectWorkerHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workerID, err := req.RequireString("worker_task_id")
		if err != nil {
			return mcp.NewToolResultError("worker_task_id is required"), nil
		}
		message, err := req.RequireString("message")
		if err != nil {
			return mcp.NewToolResultError("message is required"), nil
		}
		return s.callProjectTool(ctx, ws.ActionMCPMessageAgentProjectWorker, map[string]interface{}{
			"worker_task_id": workerID, "message": message,
		})
	}
}

func (s *Server) stopProjectWorkerHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workerID, err := req.RequireString("worker_task_id")
		if err != nil {
			return mcp.NewToolResultError("worker_task_id is required"), nil
		}
		return s.callProjectTool(ctx, ws.ActionMCPStopAgentProjectWorker, map[string]interface{}{"worker_task_id": workerID})
	}
}

func (s *Server) callProjectTool(ctx context.Context, action string, payload map[string]interface{}) (*mcp.CallToolResult, error) {
	var result map[string]interface{}
	if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return projectResult(result), nil
}

func projectResult(result map[string]interface{}) *mcp.CallToolResult {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return mcp.NewToolResultError("could not format project result")
	}
	return mcp.NewToolResultText(string(data))
}
