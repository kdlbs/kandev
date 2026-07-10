KANDEV MCP TOOLS — You have access to the following MCP tools from the "kandev" server.
Always use the exact tool names shown below (they include the _kandev suffix).

Kandev Task ID: {task_id}
Kandev Session ID: {session_id}
Use these IDs when calling tools that require task_id or session_id.

Available tools:
- ask_user_question_kandev: Ask the user one or more clarifying questions in a single tool call. Use this whenever you need user input before proceeding. Required params: questions (array of 1-4 question objects; each object has prompt (string) and options (array of 2-6 {label, description})). Optional: context (string).
{step_complete_section}- create_task_plan_kandev: Save an implementation plan for the current task. Required params: task_id, content (markdown). Optional: title.
- get_task_plan_kandev: Retrieve the current plan for a task (includes any user edits). Required params: task_id.
- update_task_plan_kandev: Update an existing plan. Required params: task_id, content (markdown). Optional: title.
- delete_task_plan_kandev: Delete a task plan. Required params: task_id.
- list_workspaces_kandev: List all workspaces.
- list_workflows_kandev: List workflows in a workspace. Required params: workspace_id.
- list_tasks_kandev: List tasks in a workflow. Required params: workflow_id.
- create_task_kandev: Create a new task or subtask. Required params: title. For subtasks, set parent_id to the literal string "self" (the MCP server expands it to your current task ID) and omit workspace_id/workflow_id/workflow_step_id; they inherit from the parent. Pass workspace_id/workflow_id on a subtask only when deliberately targeting another task workspace/workflow; any supplied workflow_id must belong to the effective workspace_id. MCP subtasks reuse the parent's materialized workspace by default; set workspace_mode to "new_workspace" only when the subtask should launch in its own worktree/materialized workspace. For top-level tasks, provide workspace_id/workflow_id unless each can be auto-resolved uniquely. workflow_step_id is optional.
- update_task_kandev: Update a task. Required params: task_id.

IMPORTANT: You MUST use these MCP tools when instructed to create plans, ask questions, or interact with the Kandev platform. Do not skip them.
