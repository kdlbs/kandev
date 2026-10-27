package profile

import "strings"

// GuardedToolClass says how a guarded session may use an MCP tool.
type GuardedToolClass string

const (
	// ClassRead tools only read, and only tasks, sessions and workflows of the
	// caller's workspace. A tool that writes anywhere is never ClassRead.
	ClassRead GuardedToolClass = "read"
	// ClassSelf tools act on the caller's own task and never take another target.
	ClassSelf GuardedToolClass = "self"
	// ClassChildWrite tools write to the caller's task or a floored child.
	ClassChildWrite GuardedToolClass = "child_write"
	// ClassInternal is the server-originated audit action; it is never offered
	// in a catalogue. A guarded session reaches it only on a trusted internal
	// call, and it skips scope checks.
	ClassInternal GuardedToolClass = "internal"
	// ClassDenied is never available to a guarded session. Unlisted tools are
	// treated as denied.
	ClassDenied GuardedToolClass = "denied"
)

// GuardedTool is one row of the classification table. Attribution lists the
// payload fields the guard overwrites with the principal's values: exactly the
// fields the action's handler request type reads.
type GuardedTool struct {
	Class       GuardedToolClass
	Attribution []string
	// PinWorkspace makes the guard set the call's workspace_id to the
	// caller's, for read tools that enumerate every workspace when it is absent.
	PinWorkspace bool
}

const mcpActionPrefix = "mcp."

var (
	attrCreate  = []string{"source_task_id", "source_session_id"}
	attrSender  = []string{"sender_task_id", "sender_session_id"}
	attrCallerT = []string{"caller_task_id"}
)

// guardedTools is keyed by bare tool name (the WebSocket action without its
// `mcp.` prefix, the injected tool name without its `_kandev` suffix).
var guardedTools = buildGuardedTools()

func buildGuardedTools() map[string]GuardedTool {
	m := map[string]GuardedTool{}
	add := func(class GuardedToolClass, attr []string, names ...string) {
		for _, n := range names {
			m[n] = GuardedTool{Class: class, Attribution: attr}
		}
	}
	add(ClassRead, nil,
		"list_workflows", "list_workflow_steps", "list_tasks", "list_repositories", "list_workspaces",
		"get_task_conversation", "list_task_sessions", "get_task_plan",
		"list_task_plan_revisions", "get_task_plan_revision", "get_walkthrough", "get_task_change_requests",
		"get_task_pr_automation", "get_task_mr_automation",
		"list_pending_agent_permissions", "list_canvases", "get_canvas", "get_canvas_state", "read_canvas_authoring_skill")
	add(ClassRead, attrCallerT, "list_related_tasks", "list_task_documents", "get_task_document")
	add(ClassSelf, nil,
		"create_task_plan", "update_task_plan", "edit_task_plan", "restore_task_plan_revision", "delete_task_plan",
		"show_walkthrough", "delete_walkthrough", "set_task_title", "step_complete", "ask_user_question",
		"ask_parent_question", "clarification_timeout", "show_rich_output", "publish_review_findings",
		"report_change_request_auto_fix_outcome", "report_pr_auto_fix_outcome", "create_canvas", "set_canvas_state")
	add(ClassSelf, attrCallerT, "write_task_document")
	add(ClassChildWrite, nil,
		"update_task", "add_task_dependency", "remove_task_dependency",
		"update_repository_base_branch", "update_task_change_request_automation", "update_task_pr_automation",
		"update_task_mr_automation")
	add(ClassChildWrite, attrCreate, "create_task")
	add(ClassChildWrite, attrSender, "spawn_session", "message_task")
	add(ClassChildWrite, []string{"sender_task_id"}, "stop_task")
	add(ClassChildWrite, []string{"sender_session_id"}, "move_task")
	add(ClassChildWrite, attrCallerT, "link_task_pr", "unlink_task_pr", "replace_task_pr", "manage_task_change_request")
	add(ClassInternal, nil, "audit_task_transfer_attempt")
	m["list_pending_questions"] = GuardedTool{Class: ClassRead, PinWorkspace: true}
	add(ClassDenied, nil, "list_executor_profiles", "list_agents", "get_diagnostic_bundle",
		"transfer_task", "archive_task", "delete_task", "resolve_agent_permission", "answer_question",
		"list_plugin_tools", "invoke_plugin_tool", "publish_canvas",
		"add_branch_to_task", "add_workspace_sources")
	return m
}

// GuardedToolByName looks a bare tool name up in the classification table.
func GuardedToolByName(name string) (GuardedTool, bool) {
	t, ok := guardedTools[name]
	return t, ok
}

// GuardedToolByAction looks up a WebSocket action such as `mcp.create_task`.
// An action without the `mcp.` prefix is not in the table.
func GuardedToolByAction(action string) (GuardedTool, bool) {
	name, ok := strings.CutPrefix(action, mcpActionPrefix)
	if !ok {
		return GuardedTool{}, false
	}
	return GuardedToolByName(name)
}

// GuardedToolNames returns every classified bare name.
func GuardedToolNames() []string {
	names := make([]string, 0, len(guardedTools))
	for n := range guardedTools {
		names = append(names, n)
	}
	return names
}
