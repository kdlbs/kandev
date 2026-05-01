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
    description: "Discover workspaces and CRUD workflows.",
    tools: [
      { name: "list_workspaces_kandev", description: "List all workspaces." },
      { name: "list_workflows_kandev", description: "List workflows in a workspace." },
      { name: "create_workflow_kandev", description: "Create a workflow." },
      { name: "update_workflow_kandev", description: "Rename or update a workflow." },
      {
        name: "delete_workflow_kandev",
        description: "Delete a workflow and all its steps (destructive).",
      },
    ],
  },
  {
    title: "Workflow steps (columns)",
    description: "Manage the columns that make up a workflow.",
    tools: [
      { name: "list_workflow_steps_kandev", description: "List all steps in a workflow." },
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
    title: "Tasks",
    description: "List, move, archive, and update task state.",
    tools: [
      { name: "list_tasks_kandev", description: "List tasks in a workflow." },
      { name: "move_task_kandev", description: "Move a task to a different step or position." },
      { name: "delete_task_kandev", description: "Delete a task permanently." },
      { name: "archive_task_kandev", description: "Archive a task." },
      {
        name: "update_task_state_kandev",
        description: "Set task state: open, in_progress, complete, blocked, cancelled.",
      },
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
