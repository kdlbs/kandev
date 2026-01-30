package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

	// Ask User Question tool
	s.AddTool(
		mcp.NewTool("ask_user_question",
			mcp.WithDescription(
				"Ask the user a clarifying question when you need more information to proceed. "+
					"Use this tool when you need to:\n"+
					"1. Gather user preferences or requirements\n"+
					"2. Clarify ambiguous instructions\n"+
					"3. Get decisions on implementation choices\n"+
					"4. Offer choices about what direction to take\n\n"+
					"The question includes 2-4 multiple-choice options. Users can also provide custom text input.\n"+
					"Returns the user's answer.",
			),
			mcp.WithString("session_id",
				mcp.Required(),
				mcp.Description("The session ID for this clarification request"),
			),
			mcp.WithString("prompt",
				mcp.Required(),
				mcp.Description("The question to ask the user"),
			),
			mcp.WithArray("options",
				mcp.Required(),
				mcp.Description("2-4 options for the user to choose from. Each option should have: label (concise 1-5 words) and description (explanation of the option)"),
			),
			mcp.WithString("context",
				mcp.Description("Optional context explaining why this question is being asked"),
			),
		),
		askUserQuestionHandler(cfg, log),
	)

	log.Info("registered MCP tools", zap.Int("count", 7))
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

func askUserQuestionHandler(cfg Config, log *logger.Logger) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID, err := req.RequireString("session_id")
		if err != nil || sessionID == "" {
			return mcp.NewToolResultError("session_id is required - this tool can only be used within a Kandev task session"), nil
		}

		prompt, err := req.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		args := req.GetArguments()
		optionsRaw, ok := args["options"]
		if !ok {
			return mcp.NewToolResultError("options is required"), nil
		}

		// Parse options from the raw interface
		optionsJSON, err := json.Marshal(optionsRaw)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse options: %v", err)), nil
		}

		var options []map[string]interface{}
		if err := json.Unmarshal(optionsJSON, &options); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse options: %v", err)), nil
		}

		if len(options) < 2 || len(options) > 4 {
			return mcp.NewToolResultError("Must provide 2-4 options"), nil
		}

		// Generate option IDs to ensure consistency between request and response
		// The backend also generates these, but we need them here for matching
		for i := range options {
			options[i]["option_id"] = fmt.Sprintf("q1_opt%d", i+1)
		}

		context := req.GetString("context", "")

		// Build the single question
		question := map[string]interface{}{
			"id":      "q1",
			"title":   "Question",
			"prompt":  prompt,
			"options": options,
		}

		// Create the clarification request (backend still expects questions array for now)
		payload := map[string]interface{}{
			"session_id": sessionID,
			"question":   question,
			"context":    context,
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/api/v1/clarification/request", cfg.KandevURL)

		log.Debug("creating clarification request",
			zap.String("session_id", sessionID))

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create request: %v", err)), nil
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			log.Error("failed to create clarification request", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create clarification request: %v", err)), nil
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 400 {
			var errBody json.RawMessage
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			return mcp.NewToolResultError(fmt.Sprintf("API error (%d): %s", resp.StatusCode, string(errBody))), nil
		}

		var createResp struct {
			PendingID string `json:"pending_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}

		// Now wait for the user's response
		waitURL := fmt.Sprintf("%s/api/v1/clarification/%s/wait", cfg.KandevURL, createResp.PendingID)

		log.Debug("waiting for user response",
			zap.String("pending_id", createResp.PendingID))

		waitReq, err := http.NewRequestWithContext(ctx, http.MethodGet, waitURL, nil)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create wait request: %v", err)), nil
		}

		waitResp, err := http.DefaultClient.Do(waitReq)
		if err != nil {
			log.Error("failed to wait for response", zap.Error(err))
			return mcp.NewToolResultError(fmt.Sprintf("Failed to wait for response: %v", err)), nil
		}
		defer func() { _ = waitResp.Body.Close() }()

		if waitResp.StatusCode >= 400 {
			var errBody json.RawMessage
			_ = json.NewDecoder(waitResp.Body).Decode(&errBody)
			return mcp.NewToolResultError(fmt.Sprintf("Wait error (%d): %s", waitResp.StatusCode, string(errBody))), nil
		}

		var userResponse struct {
			PendingID string `json:"pending_id"`
			Answer    struct {
				SelectedOptions []string `json:"selected_options"`
				CustomText      string   `json:"custom_text"`
			} `json:"answer"`
			Rejected     bool   `json:"rejected"`
			RejectReason string `json:"reject_reason"`
		}
		if err := json.NewDecoder(waitResp.Body).Decode(&userResponse); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse user response: %v", err)), nil
		}

		// Format the response for the agent
		result := formatClarificationResponse(prompt, options, userResponse.Answer, userResponse.Rejected, userResponse.RejectReason)

		log.Debug("clarification response received",
			zap.String("pending_id", createResp.PendingID),
			zap.Bool("rejected", userResponse.Rejected))

		return mcp.NewToolResultText(result), nil
	}
}

func formatClarificationResponse(prompt string, options []map[string]interface{}, answer struct {
	SelectedOptions []string `json:"selected_options"`
	CustomText      string   `json:"custom_text"`
}, rejected bool, rejectReason string) string {
	if rejected {
		if rejectReason != "" {
			return fmt.Sprintf("The user declined to answer and said: %s\n\nProceed with your best judgment based on the available information.", rejectReason)
		}
		return "The user declined to answer this question. Proceed with your best judgment based on the available information."
	}

	var result strings.Builder
	result.WriteString("# User Response\n\n")
	result.WriteString(fmt.Sprintf("**Question asked:** %s\n\n", prompt))

	hasAnswer := false

	if len(answer.SelectedOptions) > 0 {
		// Find the label and description for the selected option
		for _, opt := range options {
			optID, _ := opt["option_id"].(string)
			if optID == "" {
				optID, _ = opt["id"].(string)
			}
			for _, selected := range answer.SelectedOptions {
				if optID == selected {
					label, _ := opt["label"].(string)
					description, _ := opt["description"].(string)
					result.WriteString(fmt.Sprintf("**User's choice:** %s\n", label))
					if description != "" {
						result.WriteString(fmt.Sprintf("**Choice description:** %s\n", description))
					}
					hasAnswer = true
				}
			}
		}
	}

	if answer.CustomText != "" {
		result.WriteString(fmt.Sprintf("\n**Additional user input:** %s\n", answer.CustomText))
		hasAnswer = true
	}

	if !hasAnswer {
		result.WriteString("The user did not provide an answer.\n")
	}

	result.WriteString("\nProceed with the task based on the user's response above.")

	return result.String()
}
