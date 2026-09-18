package models

import "net/http"

// WorkspaceBrokerTool is shared by MCP discovery and the runtime capability
// directory. Authorization remains enforced by the signed runtime handlers.
type WorkspaceBrokerTool struct {
	Name, Description, Method, Path string
}

func WorkspaceBrokerTools() []WorkspaceBrokerTool {
	return []WorkspaceBrokerTool{
		{"workspace", "Read the workspace's delivery workflows, steps, repositories and execution_profiles. Use these IDs when creating, assigning or moving tasks.", http.MethodGet, "/runtime/workspace"},
		{"workspace_tasks", "List workspace tasks. Optional query: after, limit. Follow next_cursor to read more tasks.", http.MethodGet, "/runtime/tasks"},
		{"task_details", "Read native task id, its sessions and worker results before deciding what to change.", http.MethodGet, "/runtime/tasks/:id/details"},
		{"comments", "Read Orchestrator conversation comments by id. Optional query: before, limit.", http.MethodGet, "/tasks/:id/comments"},
		{"capabilities", "Read the task controls available to this workspace Orchestrator. Use workspace for workflow, repository and execution-profile IDs.", http.MethodGet, "/runtime/capabilities"},
		{"memory", "Read this Orchestrator's workspace memory. Optional query: memory_id, layer, key. Memory is context, not authorization.", http.MethodGet, "/runtime/memory"},
		{"create_task", "Create a native workspace task. request requires title; accepts description, workflow_id, workflow_step_id, repository_id, parent_id, assignee (execution profile ID), execution_mode (design or execute), external_id. Select workflow_id when multiple workflows exist. No objective or private-assistant setup is required. Omit operation_id and expected_intent_revision in this workspace scope. Inspect native tasks before retrying an unknown outcome.", http.MethodPost, "/runtime/tasks"},
		{"manage_task", "Manage native task id. request requires action: edit (title, description, priority, parent_id; empty parent_id unnests); move (workflow_step_id, optional workflow_id and position); assign (assignee execution profile ID); adopt; start; stop; message (prompt, optional session_id); archive; delete. Use move to progress the board. Delete runs native cleanup and refuses unsafe worktree removal. No objective is required. Omit operation_id and expected_intent_revision. Inspect task_details after mutations; never blindly retry unknown outcomes.", http.MethodPost, "/runtime/tasks/:id/manage"},
		{"task_status", "Set task id status: request.status is todo, in_progress, in_review or done. Native completion gates apply. Use manage_task action move to change its board column. Omit operation_id and expected_intent_revision.", http.MethodPost, "/runtime/tasks/:id/status"},
		{"comment", "Add an internal conversation receipt. request contains body; omit task_id to use this conversation. Your final reply is already recorded automatically.", http.MethodPost, "/runtime/comments"},
	}
}
