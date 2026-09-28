package mcp

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerCoordinatorTools composes the fixed six-tool coordinator surface
// (docs/specs/coordinator/system-design/copilot.md#tool-surface,
// AC-COORDINATOR-COPILOT-003.1): the five existing read tools, reused
// unchanged from the kanban/configuration registrations, plus
// propose_task_kandev. Additive rather than subtractive (unlike
// registerAutomationTools), since the coordinator surface does not start
// from the ~30-tool kanban bundle.
func (s *Server) registerCoordinatorTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflows_kandev",
			mcp.WithDescription("List all workflows in a workspace."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_workflows_kandev", s.listWorkflowsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_workflow_steps_kandev",
			mcp.WithDescription("List all workflow steps in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_workflow_steps_kandev", s.listWorkflowStepsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_repositories_kandev",
			mcp.WithDescription("List repositories in a workspace. Use this to find a repository_id for propose_task_kandev when the proposed task should target a specific codebase."),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
		),
		s.wrapHandler("list_repositories_kandev", s.listRepositoriesHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("list_tasks_kandev",
			mcp.WithDescription("List all tasks in a workflow."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_tasks_kandev", s.listTasksHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_conversation_kandev",
			mcp.WithDescription("Get conversation history for a task. If session_id is omitted, the primary session is used."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("session_id", mcp.Description("Optional session ID (must belong to task_id)")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (defaults to backend setting, max backend-capped)")),
			mcp.WithString("before", mcp.Description("Optional cursor message ID to fetch messages before this ID")),
			mcp.WithString("after", mcp.Description("Optional cursor message ID to fetch messages after this ID")),
			mcp.WithString("sort", mcp.Description("Optional sort order: asc or desc")),
			mcp.WithArray("message_types", mcp.Description("Optional message type filters (e.g. message, tool_call, error)"), mcp.Items(map[string]any{typeKey: stringType})),
		),
		s.wrapHandler("get_task_conversation_kandev", s.getTaskConversationHandler()),
	)
	s.registerProposeTaskTool()
}

func (s *Server) registerProposeTaskTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_task_kandev",
			mcp.WithDescription("Propose a task for a human to review and approve. This tool never creates a task by itself: it stores a pending proposal that a workspace manager must approve before any task or agent exists. Title must be 1 to 60 characters after trimming; description and rationale must each be at most 10,000 characters. workflow_id is required; step_id, repository_id and source_task_id are optional and, when set, must belong to this workspace. When step_id is omitted, the workflow's start step is used."),
			mcp.WithString(canvasTitleArg, mcp.Required(), mcp.Description("Proposed task title, 1 to 60 characters after trimming")),
			mcp.WithString(descriptionArg, mcp.Required(), mcp.Description("Proposed task description, at most 10,000 characters")),
			mcp.WithString("rationale", mcp.Required(), mcp.Description("Why this task is being proposed, at most 10,000 characters")),
			mcp.WithString(mcpcontract.FieldWorkflowID, mcp.Required(), mcp.Description("The workflow the task would be created in")),
			mcp.WithString(mcpcontract.FieldStepID, mcp.Description("Optional workflow step the task would be created in. Defaults to the workflow's start step")),
			mcp.WithString(mcpKeyRepositoryID, mcp.Description("Optional repository the task would target")),
			mcp.WithString("source_task_id", mcp.Description("Optional existing task this proposal originated from")),
		),
		s.wrapHandler("propose_task_kandev", s.proposeTaskHandler()),
	)
}

func (s *Server) proposeTaskHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString(canvasTitleArg)
		if err != nil {
			return mcp.NewToolResultError("title is required"), nil
		}
		description, err := req.RequireString(descriptionArg)
		if err != nil {
			return mcp.NewToolResultError("description is required"), nil
		}
		rationale, err := req.RequireString("rationale")
		if err != nil {
			return mcp.NewToolResultError("rationale is required"), nil
		}
		workflowID, err := req.RequireString(mcpcontract.FieldWorkflowID)
		if err != nil {
			return mcp.NewToolResultError("workflow_id is required"), nil
		}
		payload := map[string]string{
			canvasTitleArg:              title,
			descriptionArg:              description,
			"rationale":                 rationale,
			mcpcontract.FieldWorkflowID: workflowID,
			mcpcontract.FieldStepID:     req.GetString(mcpcontract.FieldStepID, ""),
			mcpKeyRepositoryID:          req.GetString(mcpKeyRepositoryID, ""),
			"source_task_id":            req.GetString("source_task_id", ""),
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionProposeTask, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
