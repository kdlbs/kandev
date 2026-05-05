package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// Argument-name constants used across the ask_user_question_kandev handler.
// Pulled out so goconst stays happy and renames stay safe.
const (
	promptArg          = "prompt"
	questionsArg       = "questions"
	optionsArg         = "options"
	idArg              = "id"
	titleArg           = "title"
	labelArg           = "label"
	descriptionArg     = "description"
	optionIDFieldName  = "option_id"
	questionIDFieldKey = "question_id"
)

func (s *Server) listWorkspacesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Backend returns {workspaces: [...], total: N}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListWorkspaces, nil, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listWorkflowsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError("workspace_id is required"), nil
		}
		payload := map[string]string{"workspace_id": workspaceID}
		// Backend returns {workflows: [...], total: N}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListWorkflows, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listWorkflowStepsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		payload := map[string]string{"workflow_id": workflowID}
		// Backend returns {workflow_steps: [...], total: N}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListWorkflowSteps, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listTasksHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workflowID, err := req.RequireString("workflow_id")
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		payload := map[string]string{"workflow_id": workflowID}
		// Backend returns {tasks: [...], total: N}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListTasks, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) createTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil {
			return mcp.NewToolResultError("title is required"), nil
		}

		parentID := req.GetString("parent_id", "")
		if parentID == "self" {
			if s.taskID == "" {
				return mcp.NewToolResultError("cannot use 'self' as parent_id: no current task context"), nil
			}
			parentID = s.taskID
		}
		workspaceID := req.GetString("workspace_id", "")
		workflowID := req.GetString("workflow_id", "")
		workflowStepID := req.GetString("workflow_step_id", "")

		// Default start_agent to true if not provided
		startAgent := true
		if args := req.GetArguments(); args["start_agent"] != nil {
			if v, ok := args["start_agent"].(bool); ok {
				startAgent = v
			}
		}

		payload := map[string]interface{}{
			"parent_id":           parentID,
			"workspace_id":        workspaceID,
			"workflow_id":         workflowID,
			"workflow_step_id":    workflowStepID,
			"title":               title,
			"description":         req.GetString("description", ""),
			"agent_profile_id":    req.GetString("agent_profile_id", ""),
			"executor_profile_id": req.GetString("executor_profile_id", ""),
			"source_task_id":      s.taskID,
			"start_agent":         startAgent,
		}

		// Add repository info (only valid for top-level tasks)
		repositoryID := req.GetString("repository_id", "")
		localPath := req.GetString("local_path", "")
		repositoryURL := req.GetString("repository_url", "")
		baseBranch := req.GetString("base_branch", "")
		hasRepo := repositoryID != "" || localPath != "" || repositoryURL != ""
		if hasRepo && parentID != "" {
			return mcp.NewToolResultError("repository_id, local_path, and repository_url are only valid for top-level tasks; subtasks inherit their repository from the parent"), nil
		}
		if hasRepo {
			repo := map[string]string{}
			if repositoryID != "" {
				repo["repository_id"] = repositoryID
			}
			if localPath != "" {
				repo["local_path"] = localPath
			}
			if repositoryURL != "" {
				repo["github_url"] = repositoryURL
			}
			if baseBranch != "" {
				repo["base_branch"] = baseBranch
			}
			payload["repositories"] = []map[string]string{repo}
		}

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPCreateTask, payload, &result); err != nil {
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
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPUpdateTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) messageTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		prompt, err := req.RequireString(promptArg)
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}
		// Inject sender attribution from the server's own task/session so the
		// receiving task can identify who sent the message. The backend rejects
		// the request if sender_task_id is missing or matches the target task.
		payload := map[string]interface{}{
			"task_id":           taskID,
			promptArg:           prompt,
			"sender_task_id":    s.taskID,
			"sender_session_id": s.sessionID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPMessageTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) getTaskConversationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := buildTaskConversationPayload(req, taskID)

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGetTaskConversation, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func buildTaskConversationPayload(req mcp.CallToolRequest, taskID string) map[string]interface{} {
	payload := map[string]interface{}{"task_id": taskID}
	copyOptionalStringArg(payload, req, "session_id")
	copyOptionalStringArg(payload, req, "before")
	copyOptionalStringArg(payload, req, "after")
	copyOptionalStringArg(payload, req, "sort")
	copyOptionalLimitArg(payload, req)
	copyOptionalMessageTypesArg(payload, req)
	return payload
}

func copyOptionalStringArg(payload map[string]interface{}, req mcp.CallToolRequest, key string) {
	if value := req.GetString(key, ""); value != "" {
		payload[key] = value
	}
}

func copyOptionalLimitArg(payload map[string]interface{}, req mcp.CallToolRequest) {
	args := req.GetArguments()
	if raw := args["limit"]; raw != nil {
		if limit, ok := raw.(float64); ok {
			payload["limit"] = int(limit)
		}
	}
}

func copyOptionalMessageTypesArg(payload map[string]interface{}, req mcp.CallToolRequest) {
	args := req.GetArguments()
	raw := args["message_types"]
	if raw == nil {
		return
	}
	items, ok := raw.([]interface{})
	if !ok {
		return
	}
	types := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok || value == "" {
			continue
		}
		types = append(types, value)
	}
	if len(types) > 0 {
		payload["message_types"] = types
	}
}

func (s *Server) askUserQuestionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		questions, errResult := parseQuestions(req)
		if errResult != nil {
			return errResult, nil
		}

		questionCtx := req.GetString("context", "")
		payload := map[string]interface{}{
			"session_id": s.sessionID,
			questionsArg: questions,
			"context":    questionCtx,
		}

		// Use the MCP request context from the agent. This ensures that if the agent's
		// MCP client times out, we'll detect it and not update the session state.
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPAskUserQuestion, payload, &result); err != nil {
			if ctx.Err() != nil {
				// Agent's MCP client disconnected/timed out. Notify backend to cancel
				// pending clarifications so the user's answer goes through the event
				// fallback path immediately instead of waiting for the watchdog.
				go s.notifyClarificationTimeout()
			}
			return mcp.NewToolResultError(err.Error()), nil
		}

		return extractQuestionAnswers(result, questions), nil
	}
}

// parseQuestions extracts and validates the "questions" array from the request.
// Returns the normalized question payloads (with auto-assigned ids) on success
// or a tool-result error describing the first validation failure.
func parseQuestions(req mcp.CallToolRequest) ([]map[string]interface{}, *mcp.CallToolResult) {
	questions, errResult := decodeQuestionsArg(req)
	if errResult != nil {
		return nil, errResult
	}

	seenIDs := map[string]bool{}
	for i, q := range questions {
		if errResult := normalizeAndValidateQuestion(q, i, seenIDs); errResult != nil {
			return nil, errResult
		}
		questions[i] = q
	}
	return questions, nil
}

// decodeQuestionsArg unmarshals the raw "questions" argument and enforces
// the bundle-size invariants (1..4 questions).
func decodeQuestionsArg(req mcp.CallToolRequest) ([]map[string]interface{}, *mcp.CallToolResult) {
	args := req.GetArguments()
	questionsRaw, ok := args[questionsArg]
	if !ok {
		return nil, mcp.NewToolResultError("questions is required (array of 1-4 question objects)")
	}
	questionsJSON, err := json.Marshal(questionsRaw)
	if err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("failed to parse questions: %v", err))
	}
	var questions []map[string]interface{}
	if err := json.Unmarshal(questionsJSON, &questions); err != nil {
		return nil, mcp.NewToolResultError(`questions must be an array of objects with "prompt" and "options". Example: [{"prompt": "...", "options": [{"label": "A", "description": "..."}, {"label": "B", "description": "..."}]}]`)
	}
	if len(questions) < 1 {
		return nil, mcp.NewToolResultError("questions must contain at least 1 question")
	}
	if len(questions) > 4 {
		return nil, mcp.NewToolResultError("questions must contain at most 4 questions")
	}
	return questions, nil
}

// normalizeAndValidateQuestion mutates a question payload in place: assigns a
// default id, parses options, and reports the first validation failure.
func normalizeAndValidateQuestion(q map[string]interface{}, index int, seenIDs map[string]bool) *mcp.CallToolResult {
	prompt, hasPrompt := q[promptArg].(string)
	if !hasPrompt || prompt == "" {
		return mcp.NewToolResultError(fmt.Sprintf("question %d is missing required 'prompt' field", index+1))
	}
	id, _ := q[idArg].(string)
	if id == "" {
		id = fmt.Sprintf("q%d", index+1)
		q[idArg] = id
	}
	if seenIDs[id] {
		return mcp.NewToolResultError(fmt.Sprintf("question %d has duplicate id %q", index+1, id))
	}
	seenIDs[id] = true

	options, errResult := decodeOptionsForQuestion(q, index)
	if errResult != nil {
		return errResult
	}
	if errResult := validateAndNormalizeOptions(options, index+1); errResult != nil {
		return errResult
	}
	q[optionsArg] = options
	return nil
}

func decodeOptionsForQuestion(q map[string]interface{}, index int) ([]map[string]interface{}, *mcp.CallToolResult) {
	optionsRaw, hasOptions := q[optionsArg]
	if !hasOptions {
		return nil, mcp.NewToolResultError(fmt.Sprintf("question %d is missing required 'options' field", index+1))
	}
	optionsJSON, err := json.Marshal(optionsRaw)
	if err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("question %d: failed to parse options: %v", index+1, err))
	}
	var options []map[string]interface{}
	if err := json.Unmarshal(optionsJSON, &options); err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("question %d: options must be an array of objects with 'label' and 'description' fields", index+1))
	}
	if len(options) < 2 {
		return nil, mcp.NewToolResultError(fmt.Sprintf("question %d must have at least 2 options", index+1))
	}
	if len(options) > 6 {
		return nil, mcp.NewToolResultError(fmt.Sprintf("question %d must have at most 6 options", index+1))
	}
	return options, nil
}

// validateAndNormalizeOptions checks each option for required fields and assigns a default option_id.
func validateAndNormalizeOptions(options []map[string]interface{}, questionNum int) *mcp.CallToolResult {
	for i, opt := range options {
		label, hasLabel := opt[labelArg].(string)
		if !hasLabel || label == "" {
			return mcp.NewToolResultError(fmt.Sprintf("question %d option %d is missing required 'label' field (1-5 words describing the choice)", questionNum, i+1))
		}
		description, hasDesc := opt[descriptionArg].(string)
		if !hasDesc || description == "" {
			return mcp.NewToolResultError(fmt.Sprintf("question %d option %d is missing required 'description' field (explanation of what this option means)", questionNum, i+1))
		}
		// Generate option_id if not provided
		if _, hasID := opt[optionIDFieldName].(string); !hasID {
			opt[optionIDFieldName] = fmt.Sprintf("q%d_opt%d", questionNum, i+1)
		}
	}
	return nil
}

// extractQuestionAnswers converts the backend response into a JSON tool result
// keyed by question id. Returns a friendly free-form string when the bundle was
// rejected.
func extractQuestionAnswers(result map[string]interface{}, questions []map[string]interface{}) *mcp.CallToolResult {
	if rejected, ok := result["rejected"].(bool); ok && rejected {
		reason, _ := result["reject_reason"].(string)
		if reason == "" {
			reason = "no reason provided"
		}
		return mcp.NewToolResultText(fmt.Sprintf("User rejected the questions: %s", reason))
	}

	answers, _ := result["answers"].([]interface{})
	answersByID := make(map[string]map[string]interface{}, len(answers))
	for _, raw := range answers {
		ans, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		qid, _ := ans[questionIDFieldKey].(string)
		if qid == "" {
			continue
		}
		answersByID[qid] = simplifyAnswer(ans)
	}

	out := make(map[string]interface{}, len(questions))
	for _, q := range questions {
		qid, _ := q[idArg].(string)
		if qid == "" {
			continue
		}
		if entry, ok := answersByID[qid]; ok {
			out[qid] = entry
			continue
		}
		out[qid] = map[string]interface{}{"answered": false}
	}

	if len(out) == 0 {
		// Nothing matched by id — surface the raw payload so the agent can still inspect it.
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data))
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(data))
}

// simplifyAnswer normalizes the answer map into one of:
//
//	{"selected_option": "<id>"}   when the user picked an option
//	{"custom_text": "<text>"}     when the user typed a free-form answer
//	{"answered": false}            otherwise (defensive fallback)
//
// Single-select per question, so we collapse the slice to a single id.
func simplifyAnswer(ans map[string]interface{}) map[string]interface{} {
	if customText, ok := ans["custom_text"].(string); ok && customText != "" {
		return map[string]interface{}{"custom_text": customText}
	}
	if selected, ok := ans["selected_options"].([]interface{}); ok && len(selected) > 0 {
		if first, ok := selected[0].(string); ok && first != "" {
			return map[string]interface{}{"selected_option": first}
		}
	}
	return map[string]interface{}{"answered": false}
}

// notifyClarificationTimeout sends a fire-and-forget notification to the backend
// that the agent's MCP client disconnected while waiting for a clarification response.
// The backend cancels the pending clarification so the user's answer goes through
// the event fallback path (new turn) instead of the primary path (same turn).
func (s *Server) notifyClarificationTimeout() {
	payload := map[string]string{"session_id": s.sessionID}
	if err := s.backend.RequestPayload(context.Background(), ws.ActionMCPClarificationTimeout, payload, nil); err != nil {
		s.logger.Warn("failed to notify backend of clarification timeout",
			zap.String("session_id", s.sessionID),
			zap.Error(err))
	}
}

// resolveTaskID returns the server-injected taskID if available, otherwise falls back
// to the agent-provided value. This prevents LLM hallucination of task IDs.
func (s *Server) resolveTaskID(req mcp.CallToolRequest) (string, error) {
	if s.taskID != "" {
		return s.taskID, nil
	}
	return req.RequireString("task_id")
}

func (s *Server) createTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
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
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPCreateTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Plan created successfully:\n%s", string(data))), nil
	}
}

func (s *Server) getTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		payload := map[string]string{"task_id": taskID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGetTaskPlan, payload, &result); err != nil {
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
		taskID, err := s.resolveTaskID(req)
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
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPUpdateTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Plan updated successfully:\n%s", string(data))), nil
	}
}

func (s *Server) deleteTaskPlanHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		payload := map[string]string{"task_id": taskID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPDeleteTaskPlan, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText("Plan deleted successfully."), nil
	}
}
