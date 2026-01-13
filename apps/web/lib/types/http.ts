export type TaskState =
  | 'CREATED'
  | 'SCHEDULING'
  | 'TODO'
  | 'IN_PROGRESS'
  | 'REVIEW'
  | 'BLOCKED'
  | 'WAITING_FOR_INPUT'
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
  default_executor_id?: string | null;
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
  // Worktree information (present if agent has created a worktree for this task)
  worktree_path?: string | null;
  worktree_branch?: string | null;
};

export type User = {
  id: string;
  email: string;
  created_at: string;
  updated_at: string;
};

export type UserSettings = {
  user_id: string;
  workspace_id: string;
  board_id: string;
  repository_ids: string[];
  updated_at: string;
};

export type UserSettingsResponse = {
  settings: UserSettings;
};

export type UserResponse = {
  user: User;
  settings: UserSettings;
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

export type Branch = {
  name: string;
  type: 'local' | 'remote';
  remote?: string; // remote name (e.g., "origin") for remote branches
};

export type RepositoryBranchesResponse = {
  branches: Branch[];
  total: number;
};

export type ListWorkspacesResponse = {
  workspaces: Workspace[];
  total: number;
};

export type Executor = {
  id: string;
  name: string;
  type: string;
  status: string;
  is_system: boolean;
  config?: Record<string, string>;
  created_at: string;
  updated_at: string;
};

export type Environment = {
  id: string;
  name: string;
  kind: string;
  worktree_root?: string | null;
  image_tag?: string | null;
  dockerfile?: string | null;
  build_config?: Record<string, string> | null;
  created_at: string;
  updated_at: string;
};

export type ListExecutorsResponse = {
  executors: Executor[];
  total: number;
};

export type ListEnvironmentsResponse = {
  environments: Environment[];
  total: number;
};

export type CommentAuthorType = 'user' | 'agent';
export type CommentType = 'message' | 'content' | 'tool_call' | 'progress' | 'error' | 'status';

export type Comment = {
  id: string;
  task_id: string;
  author_type: CommentAuthorType;
  author_id?: string;
  content: string;
  type: CommentType;
  metadata?: Record<string, unknown>;
  requests_input?: boolean;
  created_at: string;
};

export type AgentProfile = {
  id: string;
  agent_id: string;
  name: string;
  model: string;
  auto_approve: boolean;
  dangerously_skip_permissions: boolean;
  plan: string;
  created_at: string;
  updated_at: string;
};

export type Agent = {
  id: string;
  name: string;
  workspace_id?: string | null;
  supports_mcp: boolean;
  mcp_config_path?: string | null;
  profiles: AgentProfile[];
  created_at: string;
  updated_at: string;
};

export type AgentDiscovery = {
  name: string;
  supports_mcp: boolean;
  mcp_config_path?: string | null;
  installation_paths: string[];
  available: boolean;
  matched_path?: string | null;
};

export type ListAgentsResponse = {
  agents: Agent[];
  total: number;
};

export type ListAgentDiscoveryResponse = {
  agents: AgentDiscovery[];
  total: number;
};
