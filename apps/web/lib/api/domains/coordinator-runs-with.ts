// Mirrors internal/coordinator/task_agent.go's RunsWith: the agent a pending
// or failed create-task proposal's task will run with. `none` carries an empty
// id and name.
export type RunsWithSource = "step" | "workflow" | "workspace" | "coordinator" | "none";
export type RunsWith = {
  agent_profile_id: string;
  agent_profile_name: string;
  source: RunsWithSource;
};
