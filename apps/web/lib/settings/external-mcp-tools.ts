// Static catalog of MCP tools exposed by the Kandev backend's external endpoint.
// Mirrors apps/backend/internal/mcp/server (ModeExternal). Keep in sync when
// tools are added, removed, or renamed.

export type ExternalMcpTool = { name: string; description: string };

export type ExternalMcpToolGroup = {
  title: string;
  description: string;
  tools: ExternalMcpTool[];
};

export const EXTERNAL_MCP_TOOL_GROUPS: ExternalMcpToolGroup[] = [
  {
    title: "Workspaces & Workflows",
    description: "Discover workspaces, manage workflows, and list repositories.",
    tools: [
      { name: "list_workspaces_kandev", description: "List all workspaces." },
      { name: "list_workflows_kandev", description: "List workflows in a workspace." },
      { name: "list_repositories_kandev", description: "List repositories in a workspace." },
      { name: "create_workflow_kandev", description: "Create a workflow." },
      { name: "update_workflow_kandev", description: "Rename or update a workflow." },
      {
        name: "delete_workflow_kandev",
        description: "Delete a workflow and all its steps (destructive).",
      },
      { name: "get_workflow_kandev", description: "Get a single workflow by ID." },
      {
        name: "reorder_workflows_kandev",
        description: "Reorder workflows in a workspace by providing the full ordered list.",
      },
    ],
  },
  {
    title: "Workflow steps (columns)",
    description: "Manage the columns that make up a workflow.",
    tools: [
      { name: "list_workflow_steps_kandev", description: "List all steps in a workflow." },
      { name: "get_workflow_step_kandev", description: "Get a single workflow step by ID." },
      {
        name: "create_workflow_step_kandev",
        description: "Add a new column with prompt, color, and event hooks.",
      },
      { name: "update_workflow_step_kandev", description: "Edit a step's settings or events." },
      { name: "delete_workflow_step_kandev", description: "Delete a step (destructive)." },
      {
        name: "reorder_workflow_steps_kandev",
        description: "Set the full ordered list of step IDs.",
      },
    ],
  },
  {
    title: "Agents",
    description: "Inspect and configure the agent catalog.",
    tools: [
      {
        name: "list_agents_kandev",
        description: "List configured agents and their profiles.",
      },
      { name: "update_agent_kandev", description: "Toggle MCP support or set the config path." },
      {
        name: "create_agent_profile_kandev",
        description: "Create a new agent profile (model, auto-approve).",
      },
      { name: "delete_agent_profile_kandev", description: "Delete an agent profile." },
    ],
  },
  {
    title: "Agent profiles & MCP config",
    description: "Manage profile-level MCP server configuration.",
    tools: [
      { name: "list_agent_profiles_kandev", description: "List profiles for an agent." },
      { name: "update_agent_profile_kandev", description: "Update name, model, or auto-approve." },
      { name: "get_mcp_config_kandev", description: "Read the MCP server map for a profile." },
      {
        name: "update_mcp_config_kandev",
        description: "Replace the MCP server map for a profile.",
      },
    ],
  },
  {
    title: "Executors",
    description: "CRUD executor profiles (Docker, standalone, Sprites...).",
    tools: [
      { name: "list_executors_kandev", description: "List all executors." },
      { name: "list_executor_profiles_kandev", description: "List profiles for an executor." },
      {
        name: "create_executor_profile_kandev",
        description: "Create a profile with config, prepare/cleanup scripts.",
      },
      { name: "update_executor_profile_kandev", description: "Update an executor profile." },
      { name: "delete_executor_profile_kandev", description: "Delete an executor profile." },
    ],
  },
  {
    title: "Workspace CRUD",
    description: "Create, update, and delete workspaces.",
    tools: [
      { name: "create_workspace_kandev", description: "Create a new workspace." },
      { name: "get_workspace_kandev", description: "Get a single workspace by ID." },
      { name: "update_workspace_kandev", description: "Update an existing workspace." },
      { name: "delete_workspace_kandev", description: "Delete a workspace and all its data (destructive)." },
    ],
  },
  {
    title: "Repository CRUD",
    description: "Create and delete repositories in a workspace.",
    tools: [
      { name: "create_repository_kandev", description: "Create a new repository in a workspace." },
      { name: "delete_repository_kandev", description: "Delete a repository from a workspace." },
    ],
  },
  {
    title: "Tasks",
    description: "List, move, archive, update state, and get task details.",
    tools: [
      { name: "list_tasks_kandev", description: "List tasks in a workflow." },
      { name: "get_task_kandev", description: "Get a single task by ID." },
      { name: "list_tasks_by_workspace_kandev", description: "List tasks across a workspace with optional filtering." },
      { name: "move_task_kandev", description: "Move a task to a different step or position." },
      { name: "bulk_move_tasks_kandev", description: "Move all tasks from a source workflow/step to a target workflow/step." },
      { name: "delete_task_kandev", description: "Delete a task permanently." },
      { name: "archive_task_kandev", description: "Archive a task." },
      {
        name: "update_task_state_kandev",
        description: "Set task state: open, in_progress, complete, blocked, cancelled.",
      },
      {
        name: "get_task_conversation_kandev",
        description: "Get conversation history for a task.",
      },
    ],
  },
  {
    title: "Sessions",
    description: "Launch, stop, and list sessions for tasks.",
    tools: [
      { name: "launch_session_kandev", description: "Launch an agent session on a task." },
      { name: "stop_session_kandev", description: "Stop an active agent session." },
      { name: "get_task_sessions_kandev", description: "List all sessions for a task." },
    ],
  },
  {
    title: "Task creation",
    description: "Spawn new top-level tasks or subtasks from external agents.",
    tools: [
      {
        name: "create_task_kandev",
        description: "Create a top-level task or a subtask under an explicit parent_id.",
      },
    ],
  },
];

export function countExternalMcpTools(): number {
  return EXTERNAL_MCP_TOOL_GROUPS.reduce((sum, group) => sum + group.tools.length, 0);
}
