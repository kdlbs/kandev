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
	ActionMCPCreateTaskPlan    = "mcp.create_task_plan"
	ActionMCPGetTaskPlan       = "mcp.get_task_plan"
	ActionMCPUpdateTaskPlan    = "mcp.update_task_plan"
	ActionMCPDeleteTaskPlan    = "mcp.delete_task_plan"
)

func (s *Server) listWorkspacesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var result []map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ActionMCPListWorkspaces, nil, &result); err != nil {
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
		if err := s.backend.RequestPayload(ctx, ActionMCPListBoards, payload, &result); err != nil {
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
		if err := s.backend.RequestPayload(ctx, ActionMCPListWorkflowSteps, payload, &result); err != nil {
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
		if err := s.backend.RequestPayload(ctx, ActionMCPListTasks, payload, &result); err != nil {
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
		if err := s.backend.RequestPayload(ctx, ActionMCPCreateTask, payload, &result); err != nil {
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
		if err := s.backend.RequestPayload(ctx, ActionMCPUpdateTask, payload, &result); err != nil {
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

		// Parse options - must be array of objects with label and description
		optionsJSON, err := json.Marshal(optionsRaw)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to parse options: %v", err)), nil
		}

		var options []map[string]interface{}
		if err := json.Unmarshal(optionsJSON, &options); err != nil {
			return mcp.NewToolResultError("options must be an array of objects with 'label' and 'description' fields. Example: [{\"label\": \"Option A\", \"description\": \"Description of option A\"}]"), nil
		}

		// Validate options
		if len(options) < 2 {
			return mcp.NewToolResultError("options must contain at least 2 choices"), nil
		}
		if len(options) > 6 {
			return mcp.NewToolResultError("options must contain at most 6 choices"), nil
		}

		for i, opt := range options {
			label, hasLabel := opt["label"].(string)
			if !hasLabel || label == "" {
				return mcp.NewToolResultError(fmt.Sprintf("option %d is missing required 'label' field (1-5 words describing the choice)", i+1)), nil
			}
			description, hasDesc := opt["description"].(string)
			if !hasDesc || description == "" {
				return mcp.NewToolResultError(fmt.Sprintf("option %d is missing required 'description' field (explanation of what this option means)", i+1)), nil
			}
			// Generate option_id if not provided
			if _, hasID := opt["option_id"].(string); !hasID {
				opt["option_id"] = fmt.Sprintf("opt_%d", i+1)
			}
		}

		questionCtx := req.GetString("context", "")

		// Build the question object in the format expected by the backend
		question := map[string]interface{}{
			"id":      "q1",
			"title":   "Question",
			"prompt":  prompt,
			"options": options,
		}

		payload := map[string]interface{}{
			"session_id": s.sessionID,
			"question":   question,
			"context":    questionCtx,
		}

		// Use the MCP request context from the agent. This ensures that if the agent's
		// MCP client times out, we'll detect it and not update the session state.
		// Previous behavior used a detached context which meant we couldn't tell if the
		// agent had given up, leading to sessions stuck in RUNNING state.
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ActionMCPAskUserQuestion, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Extract the answer from the response
		if answer, ok := result["answer"]; ok {
			if answerMap, ok := answer.(map[string]interface{}); ok {
				if selectedOptions, ok := answerMap["selected_options"].([]interface{}); ok && len(selectedOptions) > 0 {
					return mcp.NewToolResultText(fmt.Sprintf("User selected: %v", selectedOptions[0])), nil
				}
				if customText, ok := answerMap["custom_text"].(string); ok && customText != "" {
					return mcp.NewToolResultText(fmt.Sprintf("User answered: %s", customText)), nil
				}
			}
		}
		if rejected, ok := result["rejected"].(bool); ok && rejected {
			reason, _ := result["reject_reason"].(string)
			return mcp.NewToolResultText(fmt.Sprintf("User rejected the question: %s", reason)), nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) createTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError("content is required"), nil
		}
		title := req.GetString("title", "Plan")

		payload := map[string]interface{}{
			"task_id":    taskID,
			"content":    content,
			"title":      title,
			"created_by": "agent",
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ActionMCPCreateTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Plan created successfully:\n%s", string(data))), nil
	}
}

func (s *Server) getTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		payload := map[string]string{"task_id": taskID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ActionMCPGetTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Check if plan exists
		if len(result) == 0 {
			return mcp.NewToolResultText("No plan exists for this task yet."), nil
		}

		// Return the plan content for easy reading
		if content, ok := result["content"].(string); ok {
			return mcp.NewToolResultText(content), nil
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) updateTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError("content is required"), nil
		}
		title := req.GetString("title", "")

		payload := map[string]interface{}{
			"task_id":    taskID,
			"content":    content,
			"created_by": "agent",
		}
		if title != "" {
			payload["title"] = title
		}

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ActionMCPUpdateTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Plan updated successfully:\n%s", string(data))), nil
	}
}

func (s *Server) deleteTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		payload := map[string]string{"task_id": taskID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ActionMCPDeleteTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText("Plan deleted successfully."), nil
	}
}

