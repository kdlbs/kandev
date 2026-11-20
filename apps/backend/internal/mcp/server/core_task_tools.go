package mcp

import (
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) registerListTasksTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("list_tasks_kandev",
			mcp.WithDescription("List all tasks in a workflow. Each task includes its associated GitHub pull requests (number, url, title, state) under the \"prs\" field when any exist — use the PR state (open/closed/merged) to find tasks whose work has landed."),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("The workflow ID")),
		),
		s.wrapHandler("list_tasks_kandev", s.listTasksHandler()),
	)
}

func (s *Server) registerMoveTaskTool() {
	argumentDescription := "Legacy alias for entry_options.instructions. Optional hand-off message applied once for a new step. Supplying both this and entry_options.instructions is rejected; non-empty prompts are invalid for a current-step request. Omit it when the target step does not start an agent on entry."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask || s.profile.Surface == mcpprofile.SurfaceAutomation {
		argumentDescription = "Optional hand-off message for the receiving agent at a new step. Only actual step changes are deferred during an active turn; non-empty prompts are invalid for a current-step request. Omit it when the target step does not start an agent on entry."
	}
	s.mcpServer.AddTool(
		mcp.NewTool("move_task_kandev",
			mcp.WithDescription(`Move a task to a different workflow step. A request naming the task's current workflow and step, without entry options, returns "applied" with the stored task because the destination already holds; no retry is needed. Only an actual step change is deferred to turn-end while the source session is RUNNING or STARTING. Idle-session and admin step changes apply immediately. Returns a move-result envelope: "disposition" is "applied" for a completed move or current-step no-op, or "deferred" when a step change was recorded for turn-end; "task" is the stored or moved task. An optioned step change also returns "move_id" and the accepted "entry_options" so you can correlate the one-shot override with the eventual entry. Moving to a manual step that does not start an agent on entry (for example Waiting, Blocked, or Done) must not carry a prompt or entry_options; such a call is rejected with a validation error. Record the instruction in the task description or plan instead.`),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("workflow_id", mcp.Required(), mcp.Description("Target workflow ID")),
			mcp.WithString("workflow_step_id", mcp.Required(), mcp.Description("Target workflow step ID")),
			mcp.WithNumber("position", mcp.Description("Requested position (0-based); the server determines arrival order, and a current-step request preserves the stored position.")),
			mcp.WithString("prompt", mcp.Description(argumentDescription)),
			moveTaskEntryOptionsToolOption(),
		),
		s.wrapHandler("move_task_kandev", s.moveTaskHandler()),
	)
}

func (s *Server) registerDeleteTaskTool() {
	description := "Delete a task permanently."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask || s.profile.Surface == mcpprofile.SurfaceAutomation {
		description = "Delete a task permanently. Use to clean up orphaned, duplicate, or test tasks you no longer need. This cannot be undone — prefer archive_task_kandev when the task may still be wanted. Restoring an archived task is a user action done from the UI, not via MCP."
	}
	s.mcpServer.AddTool(
		mcp.NewTool("delete_task_kandev",
			mcp.WithDescription(description),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to delete")),
			mcp.WithString("workspace_cleanup", mcp.Enum("delete", "retain"), mcp.Description("Use retain to delete the task while keeping workspace files, including when workspace inspection fails. Defaults to delete.")),
			mcp.WithBoolean("discard_worktree_changes", mcp.Description("Allow removal of local workspace changes.")),
			mcp.WithBoolean("allow_unverified_workspace_changes", mcp.Description("Acknowledge that workspace changes cannot be checked. Requires discard_worktree_changes=true and workspace_cleanup=delete. Task removal succeeds independently of pending physical cleanup.")),
		),
		s.wrapHandler("delete_task_kandev", s.deleteTaskHandler()),
	)
}

func (s *Server) registerArchiveTaskTool() {
	description := "Archive a task."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask || s.profile.Surface == mcpprofile.SurfaceAutomation {
		description = "Archive a task. The task is hidden from active board views but kept in the database. Use to tidy up finished or abandoned tasks. Archiving an already-archived task is a no-op that succeeds with already_archived: true. Unarchiving is a user action done from the UI, not via MCP."
	}
	s.mcpServer.AddTool(
		mcp.NewTool("archive_task_kandev",
			mcp.WithDescription(description),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID to archive")),
		),
		s.wrapHandler("archive_task_kandev", s.archiveTaskHandler()),
	)
}

func (s *Server) registerGetTaskConversationTool() {
	description := "Get conversation history for a task. If session_id is omitted, the primary session is used, falling back to the latest task session."
	if s.profile.Surface == mcpprofile.SurfaceKanbanTask || s.profile.Surface == mcpprofile.SurfaceAutomation {
		description = "Get conversation history for a task. If session_id is omitted, the primary session is used."
	}
	s.mcpServer.AddTool(
		mcp.NewTool("get_task_conversation_kandev",
			mcp.WithDescription(description),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("session_id", mcp.Description("Optional session ID (must belong to task_id)")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (defaults to backend setting, max backend-capped)")),
			mcp.WithString("before", mcp.Description("Optional cursor message ID to fetch messages before this ID")),
			mcp.WithString("after", mcp.Description("Optional cursor message ID to fetch messages after this ID")),
			mcp.WithString("sort", mcp.Description("Optional sort order: asc or desc")),
			mcp.WithArray("message_types", mcp.Description("Optional message type filters (e.g. message, tool_call, error)"), mcp.Items(map[string]any{"type": "string"})),
		),
		s.wrapHandler("get_task_conversation_kandev", s.getTaskConversationHandler()),
	)
}

func (s *Server) registerUpdateTaskStateTool() {
	s.mcpServer.AddTool(
		mcp.NewTool("update_task_state_kandev",
			mcp.WithDescription("Update the state of a task."),
			mcp.WithString("task_id", mcp.Required(), mcp.Description("The task ID")),
			mcp.WithString("state", mcp.Required(), mcp.Description("New state: CREATED, TODO, IN_PROGRESS, REVIEW, BLOCKED, WAITING_FOR_INPUT, COMPLETED, FAILED, CANCELLED (aliases like complete/done/in_progress accepted)")),
		),
		s.wrapHandler("update_task_state_kandev", s.updateTaskStateHandler()),
	)
}
