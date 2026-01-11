export type TaskState =
  | 'TODO'
  | 'IN_PROGRESS'
  | 'REVIEW'
  | 'BLOCKED'
  | 'COMPLETED'
  | 'FAILED'
  | 'CANCELLED';

export type Board = {
  id: string;
  workspace_id: string;
  name: string;
  description?: string | null;
  created_at: string;
  updated_at: string;
};

export type Workspace = {
  id: string;
  name: string;
  description?: string | null;
  owner_id: string;
  created_at: string;
  updated_at: string;
};

export type Column = {
  id: string;
  board_id: string;
  name: string;
  position: number;
  state: TaskState;
  color: string;
  created_at: string;
  updated_at: string;
};

export type Repository = {
  id: string;
  workspace_id: string;
  name: string;
  source_type: string;
  local_path: string;
  provider: string;
  provider_repo_id: string;
  provider_owner: string;
  provider_name: string;
  default_branch: string;
  setup_script: string;
  cleanup_script: string;
  created_at: string;
  updated_at: string;
};

export type RepositoryScript = {
  id: string;
  repository_id: string;
  name: string;
  command: string;
  position: number;
  created_at: string;
  updated_at: string;
};

export type Task = {
  id: string;
  workspace_id: string;
  board_id: string;
  column_id: string;
  position: number;
  title: string;
  description: string;
  state: TaskState;
  priority: number;
  agent_type?: string | null;
  repository_url?: string | null;
  branch?: string | null;
  assigned_agent_id?: string | null;
  created_at: string;
  updated_at: string;
  metadata?: Record<string, unknown> | null;
};

export type BoardSnapshot = {
  board: Board;
  columns: Column[];
  tasks: Task[];
};

export type ListBoardsResponse = {
  boards: Board[];
  total: number;
};

export type ListColumnsResponse = {
  columns: Column[];
  total: number;
};

export type ListRepositoriesResponse = {
  repositories: Repository[];
  total: number;
};

export type ListRepositoryScriptsResponse = {
  scripts: RepositoryScript[];
  total: number;
};

export type LocalRepository = {
  path: string;
  name: string;
  default_branch?: string;
};

export type RepositoryDiscoveryResponse = {
  roots: string[];
  repositories: LocalRepository[];
  total: number;
};

export type RepositoryPathValidationResponse = {
  path: string;
  exists: boolean;
  is_git: boolean;
  allowed: boolean;
  default_branch?: string;
  message?: string;
};

export type ListWorkspacesResponse = {
  workspaces: Workspace[];
  total: number;
};
