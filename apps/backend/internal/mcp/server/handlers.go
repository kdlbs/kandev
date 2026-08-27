package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// askQuestionKeepAliveInterval is how often ask_user_question streams a progress
// notification to the agent while waiting for the user's answer. The agent's MCP
// client (auggie runs on Node, whose fetch/undici applies a 300s idle timeout to
// the in-flight tool-call request) aborts the call with "fetch failed" if no bytes
// arrive for that long. Emitting a progress notification well inside that window
// keeps the streamed POST/SSE response alive so the call survives until the user
// responds. Declared as a var so tests can shorten it.
var askQuestionKeepAliveInterval = 20 * time.Second

// Argument-name constants used across the ask_user_question_kandev handler.
// Pulled out so goconst stays happy and renames stay safe.
const (
	promptArg            = "prompt"
	questionsArg         = "questions"
	optionsArg           = "options"
	idArg                = "id"
	titleArg             = "title"
	labelArg             = "label"
	descriptionArg       = "description"
	optionIDFieldName    = "option_id"
	questionIDFieldKey   = "question_id"
	answeredFieldKey     = "answered"
	rejectedFieldKey     = "rejected"
	documentArg          = "document"
	messageArg           = "message"
	autopilotArg         = "autopilot"
	contextParagraphsArg = "context_paragraphs"
	objType              = "object"
	propsKey             = "properties"
	reqKey               = "required"
	typeKey              = "type"
	stringType           = "string"
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

func (s *Server) getDiagnosticBundleHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source, err := req.RequireString("source")
		if err != nil || (source != "backend" && source != "frontend" && source != "all") {
			return mcp.NewToolResultError("source must be backend, frontend, or all"), nil
		}
		payload := map[string]string{
			"source": source, "task_id": s.taskID, "session_id": s.sessionID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(
			ctx, ws.ActionMCPGetDiagnosticBundle, payload, &result,
		); err != nil {
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

func (s *Server) listRepositoriesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		workspaceID, err := req.RequireString("workspace_id")
		if err != nil {
			return mcp.NewToolResultError("workspace_id is required"), nil
		}
		payload := map[string]string{"workspace_id": workspaceID}
		// Backend returns {repositories: [...], total: N}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListRepositories, payload, &result); err != nil {
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
			"workspace_mode":      req.GetString("workspace_mode", ""),
			"title":               title,
			"description":         req.GetString("prompt", ""),
			autopilotArg:          req.GetBool(autopilotArg, false),
			"agent_profile_id":    req.GetString("agent_profile_id", ""),
			"executor_profile_id": req.GetString("executor_profile_id", ""),
			"source_task_id":      s.taskID,
			"start_agent":         startAgent,
		}
		if s.sessionID != "" && s.taskID != "" {
			payload["source_session_id"] = s.sessionID
		}
		if externalID := req.GetString("external_id", ""); externalID != "" {
			payload["external_id"] = externalID
		}

		// Dependency edges declared at create time. The handler already read
		// blocked_by from its payload before this feature, but the tool schema
		// never declared the parameter, so no agent could reach it.
		if blockedBy := stringArrayArg(req, "blocked_by"); len(blockedBy) > 0 {
			payload["blocked_by"] = blockedBy
		}
		// Omitted means "derive": with blocked_by set, a start request becomes a
		// start-when-unblocked intent. start_agent defaults to true and agents
		// pass it by habit, so deriving is what makes an agent-built chain run in
		// order instead of launching every step at once.
		if args := req.GetArguments(); args["start_when_unblocked"] != nil {
			if v, ok := args["start_when_unblocked"].(bool); ok {
				payload["start_when_unblocked"] = v
			}
		}

		// Add repository info. For subtasks an explicit repo overrides the
		// parent's; if omitted the backend inherits from the parent.
		repositoryID := req.GetString("repository_id", "")
		localPath := req.GetString("local_path", "")
		repositoryURL := req.GetString("repository_url", "")
		baseBranch := req.GetString("base_branch", "")
		hasRepo := repositoryID != "" || localPath != "" || repositoryURL != ""
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
		} else if baseBranch != "" {
			// Forward base_branch at the top level only when the caller
			// supplied no repo identifier — the backend uses it as a fallback
			// applied to inherited subtask repos. When explicit repo entries
			// are present, the per-repo base_branch above is authoritative
			// and a top-level value here would be ignored.
			payload["base_branch"] = baseBranch
		}

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPCreateTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// The created task's description is the prompt this call just sent;
		// echoing it back only duplicates it in the caller's context.
		delete(result, descriptionArg)
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
		if launchPrompt := req.GetString("deferred_launch_prompt", ""); launchPrompt != "" {
			payload["deferred_launch_prompt"] = launchPrompt
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPUpdateTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// A write tool confirms the write; it is not a way to read a task's
		// prose back. The description is either what this call just sent or
		// unrelated to it — echoing it costs the caller thousands of tokens
		// either way. Read it deliberately via list_related_tasks_kandev.
		delete(result, descriptionArg)
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) getTaskPRAutomationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var result map[string]interface{}
		if err := s.backend.RequestPayload(
			ctx, ws.ActionMCPGetTaskPRAutomation, map[string]interface{}{"task_id": s.taskID}, &result,
		); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		removeLifecyclePromptFields(result)
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) updateTaskPRAutomationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{"task_id": s.taskID}
		args := req.GetArguments()
		if hasLifecyclePromptOverrideArgument(args) {
			return mcp.NewToolResultError("lifecycle prompt overrides are not supported"), nil
		}
		for _, key := range []string{
			"auto_fix_enabled", "auto_merge_enabled",
			"prompt_on_review_requested", "prompt_on_merged", "prompt_on_closed",
		} {
			if value, ok := args[key].(bool); ok {
				payload[key] = value
			}
		}
		for _, key := range []string{"auto_fix_prompt_override"} {
			if value, ok := args[key].(string); ok {
				payload[key] = value
			}
		}
		if len(payload) == 1 {
			return mcp.NewToolResultError("at least one PR automation option is required"), nil
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPUpdateTaskPRAutomation, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func hasLifecyclePromptOverrideArgument(args map[string]interface{}) bool {
	for _, field := range []string{"review_prompt_override", "merged_prompt_override", "closed_prompt_override"} {
		if _, ok := args[field]; ok {
			return true
		}
	}
	return false
}

func removeLifecyclePromptFields(result map[string]interface{}) {
	for _, field := range []string{
		"review_prompt_override", "merged_prompt_override", "closed_prompt_override",
		"effective_review_prompt", "effective_merged_prompt", "effective_closed_prompt",
	} {
		delete(result, field)
	}
}

// mrAutomationToolError logs the underlying backend error (which may still
// carry database/GitLab-client detail forwarded from the dispatcher) and
// returns a stable, sanitized tool error to the MCP client.
func (s *Server) mrAutomationToolError(logMsg string, err error) (*mcp.CallToolResult, error) {
	s.logger.Error(logMsg, zap.Error(err))
	return mcp.NewToolResultError("failed to process MR automation request"), nil
}

func (s *Server) getTaskMRAutomationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var result map[string]interface{}
		if err := s.backend.RequestPayload(
			ctx, ws.ActionMCPGetTaskMRAutomation, map[string]interface{}{"task_id": s.taskID}, &result,
		); err != nil {
			return s.mrAutomationToolError("get task MR automation failed", err)
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// copyMRIdentityArgs copies the optional repository_id/project_path/mr_iid
// triple that scopes a patch to one linked MR. repository_id is frequently
// an empty string on self-managed hosts without a numeric project ID (R6),
// so presence in args — not non-emptiness — is what marks it as sent;
// copyOptionalStringArg's "empty means absent" rule would silently turn a
// complete-but-empty identity into a partial one and get it rejected.
func copyMRIdentityArgs(payload, args map[string]interface{}) error {
	for _, key := range []string{"repository_id", "project_path"} {
		if value, ok := args[key]; ok {
			if s, ok := value.(string); ok {
				payload[key] = s
			}
		}
	}
	if value, ok := args["mr_iid"].(float64); ok {
		if !isValidMRIID(value) {
			return fmt.Errorf("mr_iid must be a positive integer")
		}
		payload["mr_iid"] = int(value)
	}
	return nil
}

func isValidMRIID(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && math.Trunc(value) == value
}

func (s *Server) updateTaskMRAutomationHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{"task_id": s.taskID}
		args := req.GetArguments()
		if hasLifecyclePromptOverrideArgument(args) {
			return mcp.NewToolResultError("lifecycle prompt overrides are not supported"), nil
		}
		if err := copyMRIdentityArgs(payload, args); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fieldCount := 0
		for _, key := range []string{
			"auto_fix_enabled", "auto_merge_enabled",
			"prompt_on_review_requested", "prompt_on_merged", "prompt_on_closed",
		} {
			if value, ok := args[key].(bool); ok {
				payload[key] = value
				fieldCount++
			}
		}
		if value, ok := args["auto_fix_prompt_override"].(string); ok {
			payload["auto_fix_prompt_override"] = value
			fieldCount++
		}
		if fieldCount == 0 {
			return mcp.NewToolResultError("at least one MR automation option is required"), nil
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPUpdateTaskMRAutomation, payload, &result); err != nil {
			return s.mrAutomationToolError("update task MR automation failed", err)
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
		copyOptionalStringArg(payload, req, "delivery_mode")
		copyOptionalStringArg(payload, req, "session_id")
		copyOptionalStringArg(payload, req, "reply_to_question_id")
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPMessageTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) stopTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString(mcpKeyTaskID)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		// Build a fresh payload so callers cannot override trusted sender
		// attribution or supply runtime-level session, reason, or force controls.
		payload := map[string]interface{}{
			mcpKeyTaskID:     taskID,
			"sender_task_id": s.taskID,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPStopTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// spawnSessionHandler spawns an additional agent session on an existing task.
// task_id defaults to the server's own task; sender identity is injected so
// the spawned session can identify and reply to its spawner.
func (s *Server) spawnSessionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		prompt, err := req.RequireString(promptArg)
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}
		taskID := req.GetString("task_id", s.taskID)
		if taskID == "" {
			return mcp.NewToolResultError("task_id is required (no current task in this context)"), nil
		}
		payload := map[string]interface{}{
			"task_id":           taskID,
			promptArg:           prompt,
			"sender_task_id":    s.taskID,
			"sender_session_id": s.sessionID,
		}
		copyOptionalStringArg(payload, req, "agent_profile_id")
		copyOptionalStringArg(payload, req, "name")
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPSpawnSession, payload, &result); err != nil {
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

func (s *Server) listTaskSessionsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString(mcpKeyTaskID)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		// current_session_id comes from the session this MCP server is bound
		// to, never from the caller, so "is_current" cannot be spoofed.
		payload := map[string]interface{}{
			mcpKeyTaskID:         taskID,
			"current_session_id": s.sessionID,
		}

		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListTaskSessions, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) listPendingAgentPermissionsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString(mcpKeyTaskID)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]interface{}{mcpKeyTaskID: taskID}
		copyOptionalStringArg(payload, req, "session_id")
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPListPendingAgentPermissions, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) resolveAgentPermissionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := make(map[string]interface{}, 5)
		for _, field := range []string{mcpKeyTaskID, "session_id", "request_id", "pending_id", "option_id"} {
			value, err := req.RequireString(field)
			if err != nil {
				return mcp.NewToolResultError(field + " is required"), nil
			}
			payload[field] = value
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPResolveAgentPermission, payload, &result); err != nil {
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

		questionCtx := readQuestionContext(req)
		payload := map[string]interface{}{
			"session_id": s.sessionID,
			questionsArg: questions,
			"context":    questionCtx,
		}

		// Waiting on a human answer routinely outlasts the agent MCP client's
		// idle timeout on the in-flight tool call. Stream periodic progress
		// notifications until the backend responds; mcp-go flushes them onto the
		// POST/SSE response, resetting the client's idle timer so the call is not
		// aborted mid-question.
		stop := make(chan struct{})
		defer close(stop)
		go emitKeepAlivePings(ctx, stop, askQuestionKeepAliveInterval, s.clarificationKeepAlive(ctx, req))

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

func (s *Server) askParentQuestionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		questions, errResult := parseQuestions(req)
		if errResult != nil {
			return errResult, nil
		}
		payload := map[string]interface{}{
			"task_id":    s.taskID,
			"session_id": s.sessionID,
			questionsArg: questions,
			"context":    readQuestionContext(req),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPAskParentQuestion, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func readQuestionContext(req mcp.CallToolRequest) string {
	paragraphs := req.GetStringSlice(contextParagraphsArg, nil)
	if len(paragraphs) > 0 {
		return strings.Join(paragraphs, "\n\n")
	}
	return req.GetString("context", "")
}

// emitKeepAlivePings invokes send on every interval tick until stop is closed or
// ctx is cancelled. It is the transport-agnostic core of the ask_user_question
// keepalive, split out so the timing loop is unit-testable without a live MCP
// session.
func emitKeepAlivePings(ctx context.Context, stop <-chan struct{}, interval time.Duration, send func()) {
	if interval <= 0 || send == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

// clarificationKeepAlive builds the keepalive callback that streams a
// notifications/progress message to the agent. The progress token mirrors the one
// the client attached to the tool call when present, so spec-compliant clients
// associate the updates with the in-flight request; clients that omitted a token
// ignore the unknown one. Returns a no-op when no MCP server is bound to the
// context (e.g. direct unit-test invocation of the handler).
func (s *Server) clarificationKeepAlive(ctx context.Context, req mcp.CallToolRequest) func() {
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return func() {}
	}
	var token mcp.ProgressToken = fmt.Sprintf("ask_user_question:%s", s.sessionID)
	if req.Params.Meta != nil && req.Params.Meta.ProgressToken != nil {
		token = req.Params.Meta.ProgressToken
	}
	var progress float64
	return func() {
		progress++
		_ = srv.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      progress,
			messageArg:      "Waiting for your response in Kandev",
		})
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
// keyed by question id. The shape is consistent across happy / rejected paths
// so the agent can always parse the response as a JSON object — rejected
// bundles emit an envelope { "rejected": true, "reject_reason": "..." } as
// well as per-question stub entries.
func extractQuestionAnswers(result map[string]interface{}, questions []map[string]interface{}) *mcp.CallToolResult {
	rejected, _ := result["rejected"].(bool)
	rejectReason, _ := result["reject_reason"].(string)

	out := make(map[string]interface{}, len(questions)+2)
	if rejected {
		out[rejectedFieldKey] = true
		if rejectReason != "" {
			out["reject_reason"] = rejectReason
		}
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
		answersByID[qid] = simplifyAnswer(ans, rejected)
	}

	for _, q := range questions {
		qid, _ := q[idArg].(string)
		if qid == "" {
			continue
		}
		if entry, ok := answersByID[qid]; ok {
			out[qid] = entry
			continue
		}
		stub := map[string]interface{}{answeredFieldKey: false}
		if rejected {
			stub[rejectedFieldKey] = true
		}
		out[qid] = stub
	}

	if len(out) == 0 {
		// Nothing matched by id — surface the raw payload so the agent can still inspect it.
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultStructured(result, string(data))
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultStructured(out, string(data))
}

// simplifyAnswer normalizes the answer map. Single-choice per question, but
// the user can also type a custom text alongside an option pick — we preserve
// both fields so the agent gets the full context. When the bundle was
// rejected we annotate the entry to make the partial-answer scenario explicit.
//
// Output shape (one or more keys may be set):
//
//	{"selected_option": "<id>"}
//	{"custom_text": "<text>"}
//	{"selected_option": "<id>", "custom_text": "<text>"}
//	{"answered": false}
func simplifyAnswer(ans map[string]interface{}, rejected bool) map[string]interface{} {
	out := map[string]interface{}{}
	if selected, ok := ans["selected_options"].([]interface{}); ok && len(selected) > 0 {
		if first, ok := selected[0].(string); ok && first != "" {
			out["selected_option"] = first
		}
	}
	if customText, ok := ans["custom_text"].(string); ok && customText != "" {
		out["custom_text"] = customText
	}
	if rejected && len(out) == 0 {
		return map[string]interface{}{answeredFieldKey: false, rejectedFieldKey: true}
	}
	if len(out) == 0 {
		return map[string]interface{}{answeredFieldKey: false}
	}
	return out
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

// resolveTaskID resolves the task a plan/walkthrough/review tool call targets.
//
// An explicitly provided task_id argument wins; the session-bound task is only a
// fallback for when the argument is absent (or the server is not session-bound,
// e.g. external mode). The tool schemas advertise task_id as a first-class
// parameter, so a caller that names another task — a parent reading its child's
// plan, say — must reach that task, exactly like message_task_kandev /
// stop_task_kandev already address tasks other than the caller's own.
//
// Honoring the explicit value is safe because cross-task access is authorized on
// the backend against the *stream owner's* identity: internal/mcp/scope attaches
// that identity from the agent's own execution (task → workspace → owner), never
// from the request payload, and the plan/walkthrough/review services then gate
// the target task_id through the same workspace-ownership check every other
// cross-task surface uses. A task outside the caller's reach is rejected there,
// not served.
//
// This replaces the earlier behavior, which silently discarded the argument and
// always used the bound task — turning a cross-task read into a wrong-task read
// (and a cross-task write into a wrong-task write) with no error. The pin was a
// blunt guard against LLM-hallucinated task IDs; the backend authorization above
// is the precise one, so the silent misdirection is no longer worth its cost.
func (s *Server) resolveTaskID(req mcp.CallToolRequest) (string, error) {
	if explicit := req.GetString(mcpKeyTaskID, ""); explicit != "" {
		return explicit, nil
	}
	if s.taskID != "" {
		return s.taskID, nil
	}
	return "", fmt.Errorf("task_id is required")
}

// stringField reads a string value from a decoded backend response, returning
// "" for a missing key or a non-string value.
func stringField(result map[string]interface{}, key string) string {
	value, _ := result[key].(string)
	return value
}

// planWriteAck renders the acknowledgement returned by the plan create/update
// tools. The backend echoes the stored plan in full, and handing that back to
// the agent that just sent it duplicated the plan in the agent's context on
// every sync — measurably the largest single contributor to context growth in
// plan-heavy sessions. Confirm the write, its identity and its size instead,
// and leave the content to get_task_plan_kandev.
func planWriteAck(action string, result map[string]interface{}, sentContent string) *mcp.CallToolResult {
	// Prefer the stored length: it confirms what the backend persisted rather
	// than what this call happened to send.
	size := len(sentContent)
	if stored, ok := result["content"].(string); ok {
		size = len(stored)
	}
	ack := fmt.Sprintf("Plan %s successfully: task_id=%s, title=%q, %d bytes",
		action, stringField(result, mcpKeyTaskID), stringField(result, titleArg), size)
	if updatedAt := stringField(result, "updated_at"); updatedAt != "" {
		ack += ", updated_at=" + updatedAt
	}
	ack += ". Plan content is omitted from this response; read it back with get_task_plan_kandev if needed."
	if warning := stringField(result, "plan_write_warning"); warning != "" {
		ack += "\n\n" + warning
	}
	return mcp.NewToolResultText(ack)
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
		return planWriteAck("created", result, content), nil
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
		return planWriteAck("updated", result, content), nil
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

func (s *Server) showWalkthroughHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		args := req.GetArguments()
		stepsRaw, ok := args["steps"]
		if !ok {
			return mcp.NewToolResultError("steps is required (array of {file, line, text} objects)"), nil
		}

		payload := map[string]interface{}{
			"task_id": taskID,
			"title":   req.GetString("title", "Walkthrough"),
			"steps":   stepsRaw,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPShowWalkthrough, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Walkthrough saved:\n%s", string(data))), nil
	}
}

func (s *Server) publishReviewFindingsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		findingsRaw, ok := req.GetArguments()["findings"]
		if !ok {
			return mcp.NewToolResultError(
				"findings is required (array of {file, line, severity, category, title, body} objects)"), nil
		}

		payload := map[string]interface{}{
			"task_id":  taskID,
			"summary":  req.GetString("summary", ""),
			"findings": findingsRaw,
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPPublishReviewFindings, payload, &result); err != nil {
			// A validation failure rejects the whole batch, so the agent can fix
			// the offending entry and call again.
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(fmt.Sprintf("Review findings published:\n%s", string(data))), nil
	}
}

func (s *Server) getWalkthroughHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{"task_id": taskID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPGetWalkthrough, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if len(result) == 0 {
			return mcp.NewToolResultText("No walkthrough exists for this task yet."), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) deleteWalkthroughHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := s.resolveTaskID(req)
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}
		payload := map[string]string{"task_id": taskID}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, ws.ActionMCPDeleteWalkthrough, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText("Walkthrough deleted successfully."), nil
	}
}
