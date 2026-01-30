package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCP action constants for backend WS requests
const (
	ActionMCPListWorkspaces    = "mcp.list_workspaces"
	ActionMCPListBoards        = "mcp.list_boards"
	ActionMCPListWorkflowSteps = "mcp.list_workflow_steps"
	ActionMCPListTasks         = "mcp.list_tasks"
	ActionMCPCreateTask        = "mcp.create_task"
	ActionMCPUpdateTask        = "mcp.update_task"
	ActionMCPAskUserQuestion   = "mcp.ask_user_question"
)

func (s *Server) listWorkspacesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var result []map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPListWorkspaces, nil, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listBoardsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError("workspace_id is required"), nil
		}
		payload := map[string]string{"workspace_id": workspaceID}
		var result []map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPListBoards, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listWorkflowStepsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError("board_id is required"), nil
		}
		payload := map[string]string{"board_id": boardID}
		var result []map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPListWorkflowSteps, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listTasksHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		boardID, err := req.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError("board_id is required"), nil
		}
		payload := map[string]string{"board_id": boardID}
		var result []map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPListTasks, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) createTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError("workspace_id is required"), nil
		}
		boardID, err := req.RequireString("board_id")
		if err != nil {
			return mcp.NewToolResultError("board_id is required"), nil
		}
		workflowStepID, err := req.RequireString("workflow_step_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_step_id is required"), nil
		}
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError("title is required"), nil
		}
		description := req.GetString("description", "")

		payload := map[string]string{
			"workspace_id":     workspaceID,
			"board_id":         boardID,
			"workflow_step_id": workflowStepID,
			"title":            title,
			"description":      description,
		}
		var result map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPCreateTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) updateTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]interface{}{"task_id": taskID}
		if title := req.GetString("title", ""); title != "" {
			payload["title"] = title
		}
		if desc := req.GetString("description", ""); desc != "" {
			payload["description"] = desc
		}
		if state := req.GetString("state", ""); state != "" {
			payload["state"] = state
		}
		var result map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPUpdateTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) askUserQuestionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		prompt, err := req.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}
		args := req.GetArguments()
		optionsRaw, ok := args["options"]
		if !ok {
			return mcp.NewToolResultError("options is required"), nil
		}
		optionsJSON, err := json.Marshal(optionsRaw)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse options: %v", err)), nil
		}
		var options []map[string]interface{}
		if err := json.Unmarshal(optionsJSON, &options); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse options: %v", err)), nil
		}
		questionCtx := req.GetString("context", "")

		payload := map[string]interface{}{
			"session_id": s.sessionID,
			"prompt":     prompt,
			"options":    options,
			"context":    questionCtx,
		}
		var result map[string]interface{}
		if err := s.wsClient.RequestPayload(ctx, ActionMCPAskUserQuestion, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		answer, _ := result["answer"].(string)
		return mcp.NewToolResultText(fmt.Sprintf("User answered: %s", answer)), nil
	}
}

