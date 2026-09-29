package mcp

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerCoordinatorTools registers each tool the session's bound list names
// that has a handler in this build; a bound name with no handler is skipped.
// Read tools reuse the kanban/configuration registrations unchanged, so the
// surface is additive rather than subtractive.
func (s *Server) registerCoordinatorTools() {
	catalog := s.coordinatorToolCatalog()
	for _, name := range mcpprofile.BoundCoordinatorToolNames(s.profile) {
		if register, ok := catalog[name]; ok {
			register()
		}
	}
}

// coordinatorToolCatalog maps a coordinator tool name to its registration.
func (s *Server) coordinatorToolCatalog() map[string]func() {
	return map[string]func(){
		"list_workflows_kandev": func() {
			s.mcpServer.AddTool(
				mcp.NewTool("list_workflows_kandev",
					mcp.WithDescription("List all workflows in a workspace."),
					mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
				),
				s.wrapHandler("list_workflows_kandev", s.listWorkflowsHandler()),
			)
		},
		"list_workflow_steps_kandev": func() {
			s.mcpServer.AddTool(
				mcp.NewTool("list_workflow_steps_kandev",
					mcp.WithDescription("List all workflow steps in a workflow."),
					mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
				),
				s.wrapHandler("list_workflow_steps_kandev", s.listWorkflowStepsHandler()),
			)
		},
		"list_repositories_kandev": func() {
			s.mcpServer.AddTool(
				mcp.NewTool("list_repositories_kandev",
					mcp.WithDescription("List repositories in a workspace. Use this to find a repository_id for propose_task_kandev when the proposed task should target a specific codebase."),
					mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID")),
				),
				s.wrapHandler("list_repositories_kandev", s.listRepositoriesHandler()),
			)
		},
		"list_tasks_kandev": func() {
			s.mcpServer.AddTool(
				mcp.NewTool("list_tasks_kandev",
					mcp.WithDescription("List all tasks in a workflow."),
					mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
				),
				s.wrapHandler("list_tasks_kandev", s.listTasksHandler()),
			)
		},
		"get_task_conversation_kandev": func() {
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
		},
		"propose_task_kandev":         s.registerProposeTaskTool,
		"get_coordinator_item_kandev": s.registerGetCoordinatorItemTool,
	}
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

// registerGetCoordinatorItemTool registers get_coordinator_item_kandev
// (docs/specs/coordinator/system-design/copilot-tools.md#item-read): the
// read behind an Ask about this bracketed reference. task references are
// deliberately not a kind here; the tool description points callers at
// list_tasks_kandev and get_task_conversation_kandev instead.
func (s *Server) registerGetCoordinatorItemTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("get_coordinator_item_kandev",
			mcp.WithDescription("Read the record behind a bracketed Ask about this reference. kind \"proposal\" returns the coordinator's own proposal (spec, status, error, timestamps); kind \"stall\" returns the stall record of a task in this workspace. A \"task\" reference is read with list_tasks_kandev and get_task_conversation_kandev instead, not with this tool."),
			mcp.WithString("kind", mcp.Required(), mcp.Description(`Either "proposal" or "stall"`)),
			mcp.WithString("id", mcp.Required(), mcp.Description("The referenced id: a proposal id for kind \"proposal\", a task id for kind \"stall\"")),
		),
		s.wrapHandler("get_coordinator_item_kandev", s.getCoordinatorItemHandler()),
	)
}

func (s *Server) getCoordinatorItemHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		kind, err := req.RequireString("kind")
		if err != nil {
			return mcp.NewToolResultError("kind is required"), nil
		}
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError("id is required"), nil
		}
		payload := map[string]string{"kind": kind, "id": id}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionGetItem, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
