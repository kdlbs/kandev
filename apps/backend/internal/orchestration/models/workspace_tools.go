package models

import "net/http"

// WorkspaceBrokerTool is shared by MCP discovery and the runtime capability
// directory. Authorization remains enforced by the signed runtime handlers.
type WorkspaceBrokerTool struct {
	Name, Description, Method, Path string
}

func WorkspaceBrokerTools() []WorkspaceBrokerTool {
	return []WorkspaceBrokerTool{
		WorkspaceAdministrationTool(),
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

// WorkspaceAdministrationTool is shared with private Orchestrator conversations.
func WorkspaceAdministrationTool() WorkspaceBrokerTool {
	return WorkspaceBrokerTool{"manage_workspace", `Manage configuration in the assigned home workspace using native Kandev services. No API key, objective or operation_id is required. Private conversations require execute mode. request: resource (workspace, workflow, step, repository), action (create, update, delete, reorder), optional id, configuration (object). Inspect workspace before and after changes; never blindly retry an unknown outcome.
workspace: update only; configuration accepts name, description, default_executor_id, default_environment_id, default_agent_profile_id, default_config_agent_profile_id.
workflow: create (name required, description, prompt, workflow_template_id from workspace.workflow_templates); update id (name, description, prompt, agent_profile_id); delete id archives its remaining tasks; reorder uses request.ids.
step: create configuration requires workflow_id and name; update/delete use request.id; reorder uses request.workflow_id and request.ids. Configuration accepts position, color, prompt, stage_type, agent_profile_id, events, allow_manual_move, is_start_step, show_in_command_panel, wip_limit, pull_from_step_id, auto_advance_requires_signal, cancel_triggers_turn_complete, profile_session_start_policy, profile_session_end_policy. Update also accepts auto_archive_after_hours. Use native step event shapes from workspace.workflow_steps or workflow_templates. Deleting an occupied column leaves tasks needing reassignment; move tasks first when appropriate.
repository: create registers an existing local Git checkout (name, local_path, source_type=local) or remote repository (name, source_type=remote, remote_url, provider identity); update/delete use id. Configuration also accepts default_branch, worktree_branch_prefix, worktree_branch_template, pull_before_worktree, setup_script, cleanup_script, dev_script, copy_files and secret_bindings (references only). Delete uses native active-session checks.
Workspace ownership/access, global settings, hidden system workflows, linked-workspace administration and GitHub-synced workflow definitions are outside this tool.`, http.MethodPost, "/runtime/workspace/manage"}
}
