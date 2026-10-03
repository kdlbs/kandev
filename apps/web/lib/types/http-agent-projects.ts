export type AgentProjectTaskSummary = {
  id: string;
  title: string;
  state: string;
  parent_id?: string;
  archived_at?: string | null;
  updated_at: string;
};

export type AgentProject = {
  id: string;
  workspace_id: string;
  name: string;
  repository_ids: string[];
  primary_repository_id: string;
  coordinator_profile_id: string;
  economy_profile_id: string;
  frontier_profile_id: string;
  executor_profile_id: string;
  main_task_id: string;
  revision: number;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
  tasks: AgentProjectTaskSummary[];
};

export type AgentProjectContextEntry = {
  name: string;
  path: string;
  kind: "file" | "directory";
  size?: number;
};

export type AgentProjectContextFile = {
  path: string;
  content: string;
  hash: string;
};
