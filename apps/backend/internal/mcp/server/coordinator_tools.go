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
		"propose_task_kandev":              s.registerProposeTaskTool,
		"propose_resume_kandev":            s.registerProposeResumeTool,
		"propose_message_kandev":           s.registerProposeMessageTool,
		"propose_move_kandev":              s.registerProposeMoveTool,
		"get_coordinator_item_kandev":      s.registerGetCoordinatorItemTool,
		"list_coordinator_activity_kandev": s.registerListCoordinatorActivityTool,
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
			standingOrderIDsOption(),
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
		payload := map[string]interface{}{
			canvasTitleArg:              title,
			descriptionArg:              description,
			"rationale":                 rationale,
			mcpcontract.FieldWorkflowID: workflowID,
			mcpcontract.FieldStepID:     req.GetString(mcpcontract.FieldStepID, ""),
			mcpKeyRepositoryID:          req.GetString(mcpKeyRepositoryID, ""),
			"source_task_id":            req.GetString("source_task_id", ""),
			standingOrderIDsArg:         req.GetStringSlice(standingOrderIDsArg, nil),
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

// registerListCoordinatorActivityTool registers list_coordinator_activity_kandev
// (docs/specs/coordinator/system-design/activity-log.md#read-tool): the
// coordinator's own activity log, resolved from the principal alone. Which
// sessions may call it is decided by the coordinator tool profile.
func (s *Server) registerListCoordinatorActivityTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_coordinator_activity_kandev",
			mcp.WithDescription("List what this coordinator has proposed and what happened to each proposal, newest first: outcome, action class, target task, reason and timestamps. Returns your own activity only. Use limit (1 to 50, default 20) and the returned next_cursor as before to page."),
			mcp.WithNumber("limit", mcp.Description("Optional page size, 1 to 50 (default 20)")),
			mcp.WithString("before", mcp.Description("Optional cursor: the next_cursor of the previous page")),
		),
		s.wrapHandler("list_coordinator_activity_kandev", s.listCoordinatorActivityHandler()),
	)
}

func (s *Server) listCoordinatorActivityHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{}
		if args := req.GetArguments(); args != nil {
			if v, ok := args["limit"]; ok {
				payload["limit"] = v
			}
			if v, ok := args["before"]; ok {
				payload["before"] = v
			}
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionListActivity, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

const standingOrderIDsArg = "standing_order_ids"

func standingOrderIDsOption() mcp.ToolOption {
	return mcp.WithArray(standingOrderIDsArg,
		mcp.Description("Optional ids of the standing orders that shaped this proposal, at most 5, no duplicates. Each must be one of your active standing orders."),
		mcp.Items(map[string]any{typeKey: stringType}))
}

func (s *Server) registerProposeResumeTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_resume_kandev",
			mcp.WithDescription("Propose resuming a stopped or interrupted task session for a human to approve. Nothing is resumed until a workspace manager approves. The task must be in a workflow you watch, not archived, and its primary session must be in a resumable state. A repeat call for the same task returns the open proposal with deduplicated true."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task whose session would be resumed")),
			mcp.WithString("rationale", mcp.Description("Why the session should be resumed, at most 10,000 characters")),
			standingOrderIDsOption(),
		),
		s.wrapHandler("propose_resume_kandev", s.proposeKindHandler(mcpcontract.ActionProposeResume, "task_id", "rationale")),
	)
}

func (s *Server) registerProposeMessageTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_message_kandev",
			mcp.WithDescription("Propose sending a message to a task's agent for a human to approve. Nothing is sent until a workspace manager approves. The text is 1 to 4,000 characters. A repeat call for the same task returns the open proposal with deduplicated true."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task whose agent would receive the message")),
			mcp.WithString("text", mcp.Required(), mcp.Description("The message, 1 to 4,000 characters after trimming")),
			mcp.WithString("rationale", mcp.Description("Why the message should be sent, at most 10,000 characters")),
			standingOrderIDsOption(),
		),
		s.wrapHandler("propose_message_kandev", s.proposeKindHandler(mcpcontract.ActionProposeMessage, "task_id", "text", "rationale")),
	)
}

func (s *Server) registerProposeMoveTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_move_kandev",
			mcp.WithDescription("Propose moving a task to another step of its workflow for a human to approve. Nothing moves until a workspace manager approves. The destination is never a Done step. A repeat call for the same task returns the open proposal with deduplicated true."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task to move")),
			mcp.WithString(mcpcontract.FieldStepID, mcp.Required(), mcp.Description("The destination step in the task's current workflow")),
			mcp.WithString("rationale", mcp.Description("Why the task should move, at most 10,000 characters")),
			standingOrderIDsOption(),
		),
		s.wrapHandler("propose_move_kandev", s.proposeKindHandler(mcpcontract.ActionProposeMove, "task_id", mcpcontract.FieldStepID, "rationale")),
	)
}

// proposeKindHandler forwards the named string arguments and the standing
// order citations to the propose action. Required-argument and value checks
// belong to the coordinator service, which names the offending field.
func (s *Server) proposeKindHandler(action string, fields ...string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{standingOrderIDsArg: req.GetStringSlice(standingOrderIDsArg, nil)}
		for _, name := range fields {
			payload[name] = req.GetString(name, "")
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
