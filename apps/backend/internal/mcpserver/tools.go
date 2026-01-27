package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

func registerTools(s *server.MCPServer, cfg Config, log *logger.Logger) {
	// List Workspaces tool
	s.AddTool(
		mcp.NewTool("list_workspaces",
			mcp.WithDescription("List all workspaces. Use this first to get workspace IDs for other operations."),
		),
		listWorkspacesHandler(cfg, log),
	)

	// List Boards tool
	s.AddTool(
		mcp.NewTool("list_boards",
			mcp.WithDescription("List all boards in a workspace. Use this to get board IDs for task operations."),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("The workspace ID to list boards from"),
			),
		),
		listBoardsHandler(cfg, log),
	)

	// List Workflow Steps tool
	s.AddTool(
		mcp.NewTool("list_workflow_steps",
			mcp.WithDescription("List all workflow steps in a board. Use this to get workflow step IDs for creating tasks."),
			mcp.WithString("board_id",
				mcp.Required(),
				mcp.Description("The board ID to list workflow steps from"),
			),
		),
		listWorkflowStepsHandler(cfg, log),
	)

	// List Tasks tool
	s.AddTool(
		mcp.NewTool("list_tasks",
			mcp.WithDescription("List all tasks on a board"),
			mcp.WithString("board_id",
				mcp.Required(),
				mcp.Description("The board ID to list tasks from"),
			),
		),
		listTasksHandler(cfg, log),
	)

	// Create Task tool
	s.AddTool(
		mcp.NewTool("create_task",
			mcp.WithDescription("Create a new task on a board"),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("The workspace ID"),
			),
			mcp.WithString("board_id",
				mcp.Required(),
				mcp.Description("The board ID"),
			),
			mcp.WithString("workflow_step_id",
				mcp.Required(),
				mcp.Description("The workflow step ID to place the task in"),
			),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("The task title"),
			),
			mcp.WithString("description",
				mcp.Description("The task description (optional)"),
			),
		),
		createTaskHandler(cfg, log),
	)

	// Update Task tool
	s.AddTool(
		mcp.NewTool("update_task",
			mcp.WithDescription("Update an existing task"),
			mcp.WithString("task_id",
				mcp.Required(),
				mcp.Description("The task ID to update"),
			),
			mcp.WithString("title",
				mcp.Description("New title (optional)"),
			),
			mcp.WithString("description",
				mcp.Description("New description (optional)"),
			),
			mcp.WithString("state",
				mcp.Description("New state: not_started, in_progress, paused, completed, cancelled (optional)"),
			),
		),
		updateTaskHandler(cfg, log),
	)

	log.Info("registered MCP tools", zap.Int("count", 6))
}

func listWorkspacesHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url := fmt.Sprintf("%s/api/v1/workspaces", cfg.KandevURL)
		resp, err := http.Get(url)
		if err != nil {
			log.Error("failed to fetch workspaces", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch workspaces: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		formatted, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(formatted)), nil
	}
}

func listBoardsHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		url := fmt.Sprintf("%s/api/v1/workspaces/%s/boards", cfg.KandevURL, workspaceID)
		resp, err := http.Get(url)
		if err != nil {
			log.Error("failed to fetch boards", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch boards: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		formatted, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(formatted)), nil
	}
}

func listWorkflowStepsHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		url := fmt.Sprintf("%s/api/v1/boards/%s/workflow/steps", cfg.KandevURL, boardID)
		resp, err := http.Get(url)
		if err != nil {
			log.Error("failed to fetch workflow steps", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch workflow steps: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		formatted, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(formatted)), nil
	}
}

func listTasksHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		url := fmt.Sprintf("%s/api/v1/boards/%s/tasks", cfg.KandevURL, boardID)
		resp, err := http.Get(url)
		if err != nil {
			log.Error("failed to fetch tasks", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch tasks: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		formatted, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(formatted)), nil
	}
}

func createTaskHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		boardID, err := req.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		workflowStepID, err := req.RequireString("workflow_step_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		payload := map[string]interface{}{
			"workspace_id":     workspaceID,
			"board_id":         boardID,
			"workflow_step_id": workflowStepID,
			"title":            title,
		}
		if desc := req.GetString("description", ""); desc != "" {
			payload["description"] = desc
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/tasks", cfg.KandevURL)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create request: %v", err)), nil
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			log.Error("failed to create task", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create task: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		if resp.StatusCode >= 400 {
			return mcp.NewToolResultError(fmt.Sprintf("API error (%d): %s", resp.StatusCode, string(result))), nil
		}

		formatted, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(formatted)), nil
	}
}

func updateTaskHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		payload := make(map[string]interface{})
		if title := req.GetString("title", ""); title != "" {
			payload["title"] = title
		}
		if desc := req.GetString("description", ""); desc != "" {
			payload["description"] = desc
		}
		if state := req.GetString("state", ""); state != "" {
			payload["state"] = state
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/tasks/%s", cfg.KandevURL, taskID)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create request: %v", err)), nil
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			log.Error("failed to update task", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update task: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		var result json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		if resp.StatusCode >= 400 {
			return mcp.NewToolResultError(fmt.Sprintf("API error (%d): %s", resp.StatusCode, string(result))), nil
		}

		formatted, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(formatted)), nil
	}
}

