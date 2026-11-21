package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/coordinator/mcpcontract"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	ws "github.com/kandev/kandev/pkg/websocket"
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
		"propose_answer_kandev":            s.registerProposeAnswerTool,
		"propose_merge_kandev":             s.registerProposeMergeTool,
		"propose_improvement_kandev":       s.registerProposeImprovementTool,
		"get_coordinator_item_kandev":      s.registerGetCoordinatorItemTool,
		"list_coordinator_activity_kandev": s.registerListCoordinatorActivityTool,
		"list_coordinator_turns_kandev":    s.registerListCoordinatorTurnsTool,
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
			mcp.WithString("in_reply_to", mcp.Description("Optional id of a returned proposal of yours that this one revises after the manager's reply")),
			standingOrderIDsOption(),
			s.noteIDsOption(),
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
			"in_reply_to":               req.GetString("in_reply_to", ""),
			standingOrderIDsArg:         req.GetStringSlice(standingOrderIDsArg, nil),
		}
		withNoteIDs(payload, req)
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

const turnsTaskArg = "task"

// registerListCoordinatorTurnsTool registers list_coordinator_turns_kandev
// (docs/specs/coordinator/system-design/turn-ledger.md): the coordinator's own
// turn ledger, resolved from the principal alone.
func (s *Server) registerListCoordinatorTurnsTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_coordinator_turns_kandev",
			mcp.WithDescription("List your own recent turns, newest first: trigger, outcome, verdict, the configuration stamp the turn ran under, proposals made, a digest of the tool calls it made and their allowed or refused result, and token and cost totals. Returns no message text. Filter with since (RFC 3339, default 30 days ago), verdict, trigger and task. Use limit (1 to 50, default 20) and the returned next_before as before to page."),
			mcp.WithString("since", mcp.Description("Optional RFC 3339 time; only turns started at or after it (default 30 days ago, at most 400 days back)")),
			mcp.WithString("verdict", mcp.Description("Optional verdict filter: blocked, acted, proposed, needs_you or nothing_needed")),
			mcp.WithString("trigger", mcp.Description("Optional trigger filter: message, wake or dream")),
			mcp.WithString(turnsTaskArg, mcp.Description("Optional task id; only turns that made a call targeting this task")),
			mcp.WithNumber("limit", mcp.Description("Optional page size, 1 to 50 (default 20)")),
			mcp.WithString("before", mcp.Description("Optional cursor: the next_before of the previous page")),
		),
		s.wrapHandler("list_coordinator_turns_kandev", s.listCoordinatorTurnsHandler()),
	)
}

func (s *Server) listCoordinatorTurnsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{}
		if args := req.GetArguments(); args != nil {
			for _, key := range []string{"since", "verdict", "trigger", turnsTaskArg, "limit", "before"} {
				if v, ok := args[key]; ok {
					payload[key] = v
				}
			}
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionListTurns, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

const standingOrderIDsArg = "standing_order_ids"

const noteIDsArg = "note_ids"

// noteIDsOption declares the optional note citation list every proposal tool
// accepts when the conversation was bound with notes on; otherwise it adds nothing. The value is forwarded untouched; the coordinator service judges it.
func (s *Server) noteIDsOption() mcp.ToolOption {
	if p := s.profile.CoordinatorToolPolicy; p == nil || !p.Notes {
		return func(*mcp.Tool) {}
	}
	return mcp.WithArray(noteIDsArg,
		mcp.Description("Optional ids of the notes from this turn's coordinator notes index that this proposal relies on, at most 5, no duplicates."),
		mcp.Items(map[string]any{typeKey: stringType}))
}

// withNoteIDs copies the call's raw note_ids into a forwarded payload.
func withNoteIDs(payload map[string]interface{}, req mcp.CallToolRequest) map[string]interface{} {
	if v, ok := req.GetArguments()[noteIDsArg]; ok {
		payload[noteIDsArg] = v
	}
	return payload
}

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
			s.noteIDsOption(),
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
			s.noteIDsOption(),
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
			s.noteIDsOption(),
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
		withNoteIDs(payload, req)
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) registerProposeAnswerTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_answer_kandev",
			mcp.WithDescription("Propose an answer to the pending question or permission request of a task on a board you watch, for a manager to approve, edit or reject. Nothing is delivered until a manager approves. target is \"clarification\" or \"permission\"; pending_id is the pending item's id from get_coordinator_item_kandev. draft is {answers: [{question_id, option_ids, custom_text}]} for a clarification, or {option_id} for a permission. sources lists the links or references you relied on as {ref, label} objects, confidence is one of low, med or high, needs_decision flags a judgement call, rationale says why. A call for a target and pending_id that already has an open proposal returns that proposal unchanged with deduplicated true, whatever draft it carries; a proposal is replaced only once its target is no longer current."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task holding the pending item")),
			mcp.WithString("target", mcp.Required(), mcp.Description("clarification or permission")),
			mcp.WithString("pending_id", mcp.Required(), mcp.Description("The pending clarification bundle or permission request id")),
			mcp.WithAny("draft", mcp.Required(), mcp.Description("The proposed answer")),
			mcp.WithArray("sources", mcp.Description("Links or references behind the answer"), mcp.Items(map[string]any{
				typeKey: objType,
				propsKey: map[string]any{
					"ref":   map[string]any{typeKey: stringType},
					"label": map[string]any{typeKey: stringType},
				},
				reqKey: []string{"ref"},
			})),
			mcp.WithString("confidence", mcp.Required(), mcp.Description("How sure you are: low, med or high"), mcp.Enum("low", "med", "high")),
			mcp.WithBoolean("needs_decision", mcp.Required(), mcp.Description("True when the answer is a judgement call for the manager")),
			mcp.WithString("rationale", mcp.Description("Why this answer, at most 10,000 characters")),
			standingOrderIDsOption(),
			s.noteIDsOption(),
		),
		s.wrapHandler("propose_answer_kandev", s.proposeAnswerHandler()),
	)
}

// proposeAnswerHandler forwards the argument object with its types intact. The
// answer kind validates every member and names the offending field; the error
// renders as "<message> (field: <field>)", or the bare message when no field is named.
func (s *Server) proposeAnswerHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := req.GetArguments()
		if payload == nil {
			payload = map[string]interface{}{}
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionProposeAnswer, payload, &result); err != nil {
			return mcp.NewToolResultError(answerErrorText(err)), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) registerProposeMergeTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_merge_kandev",
			mcp.WithDescription("Propose merging the pull request or merge request linked to a task on a board you watch, at the head commit you looked at. Nothing is merged until a manager approves it, or until the repository is set to automatic and every condition holds. task_id is the task, change is optional and names which linked change when the task has several (provider|host|repository|number), expected_head_sha is the head you reviewed. A call that cannot create a proposal returns proposal_id null, status \"refused\" and a creation_code (change_unknown, policy_denied, predecessor, method, unavailable or head_mismatch). A call for a change that already has an open proposal returns that proposal with deduplicated true."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task whose linked change should be merged")),
			mcp.WithString("change", mcp.Description("Which linked change, when the task has several")),
			mcp.WithString("expected_head_sha", mcp.Required(), mcp.Description("The head commit SHA you reviewed")),
			s.noteIDsOption(),
		),
		s.wrapHandler("propose_merge_kandev", s.proposeMergeHandler()),
	)
}

func (s *Server) proposeMergeHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := req.GetArguments()
		if payload == nil {
			payload = map[string]interface{}{}
		}
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionProposeMerge, payload, &result); err != nil {
			return mcp.NewToolResultError(answerErrorText(err)), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

// answerErrorText renders a refusal as "<message> (field: <field>)", or the
// bare message when the refusal names no field. Other errors keep their text.
func answerErrorText(err error) string {
	var be *BackendError
	if !errors.As(err, &be) || be.Code != ws.ErrorCodeValidation {
		return err.Error()
	}
	if field, _ := be.Details["field"].(string); field != "" {
		return fmt.Sprintf("%s (field: %s)", be.Message, field)
	}
	return be.Message
}

const improvementContextArg = "context"

func (s *Server) registerProposeImprovementTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("propose_improvement_kandev",
			mcp.WithDescription("Propose a replacement for your own context text, with the unattended runs that show why, for a human to review. Nothing is applied by approving it: a manager applies the change separately in your settings. title is 1 to 60 characters, rationale 1 to 10,000, context the full replacement text (1 to 4,000 characters, different from your current context). evidence is 1 to 10 objects, each with exactly one of run_id (one of your unattended turns) or task_id (a task in this workspace); at least one is a run_id."),
			mcp.WithString(canvasTitleArg, mcp.Required(), mcp.Description("Short title, 1 to 60 characters after trimming")),
			mcp.WithString("rationale", mcp.Required(), mcp.Description("Why the change helps, at most 10,000 characters")),
			mcp.WithString(improvementContextArg, mcp.Required(), mcp.Description("The full replacement context text, at most 4,000 characters")),
			mcp.WithArray(mcpcontract.FieldEvidence, mcp.Required(),
				mcp.Description("1 to 10 references, each an object with exactly one of run_id or task_id"),
				mcp.Items(map[string]any{
					typeKey: objType,
					propsKey: map[string]any{
						"run_id":     map[string]any{typeKey: stringType},
						mcpKeyTaskID: map[string]any{typeKey: stringType},
					},
				})),
			s.noteIDsOption(),
		),
		s.wrapHandler("propose_improvement_kandev", s.proposeImprovementHandler()),
	)
}

// proposeImprovementHandler forwards the arguments untouched; the coordinator
// service validates every field and names the offending one.
func (s *Server) proposeImprovementHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		payload := map[string]interface{}{
			canvasTitleArg:            req.GetString(canvasTitleArg, ""),
			"rationale":               req.GetString("rationale", ""),
			improvementContextArg:     req.GetString(improvementContextArg, ""),
			mcpcontract.FieldEvidence: req.GetArguments()[mcpcontract.FieldEvidence],
		}
		withNoteIDs(payload, req)
		var result map[string]interface{}
		if err := s.backend.RequestPayload(ctx, mcpcontract.ActionProposeImprovement, payload, &result); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}
