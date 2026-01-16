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

export type TaskSessionState =
  | 'CREATED'
  | 'STARTING'
  | 'RUNNING'
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
  default_environment_id?: string | null;
  default_agent_profile_id?: string | null;
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

export type TaskRepository = {
  id: string;
  task_id: string;
  repository_id: string;
  base_branch: string;
  position: number;
  metadata?: Record<string, unknown>;
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
  repositories?: TaskRepository[];
  assigned_agent_id?: string | null;
  created_at: string;
  updated_at: string;
  metadata?: Record<string, unknown> | null;
  // Worktree information (present if agent has created a worktree for this task)
  worktree_path?: string | null;
  worktree_branch?: string | null;
};

export type TaskSession = {
  id: string;
  task_id: string;
  agent_instance_id?: string;
  container_id?: string;
  agent_profile_id?: string;
  executor_id?: string;
  environment_id?: string;
  repository_id?: string;
  base_branch?: string;
  worktree_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  state: TaskSessionState;
  progress: number;
  error_message?: string;
  metadata?: Record<string, unknown> | null;
  agent_profile_snapshot?: Record<string, unknown> | null;
  executor_snapshot?: Record<string, unknown> | null;
  environment_snapshot?: Record<string, unknown> | null;
  repository_snapshot?: Record<string, unknown> | null;
  started_at: string;
  completed_at?: string | null;
  updated_at: string;
};

export type TaskSessionsResponse = {
  sessions: TaskSession[];
  total: number;
};

export type NotificationProviderType = 'local' | 'apprise';

export type NotificationProvider = {
  id: string;
  name: string;
  type: NotificationProviderType;
  config: Record<string, unknown>;
  enabled: boolean;
  events: string[];
  created_at: string;
  updated_at: string;
};

export type NotificationProvidersResponse = {
  providers: NotificationProvider[];
  apprise_available: boolean;
  events: string[];
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
  initial_setup_complete?: boolean;
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
  is_system: boolean;
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

export type ListMessagesResponse = {
  messages: Message[];
  total: number;
  has_more: boolean;
  cursor: string;
};

export type MessageAuthorType = 'user' | 'agent';
export type MessageType =
  | 'message'
  | 'content'
  | 'tool_call'
  | 'progress'
  | 'error'
  | 'status'
  | 'thinking'
  | 'todo';

export type Message = {
  id: string;
  task_session_id: string;
  task_id: string;
  author_type: MessageAuthorType;
  author_id?: string;
  content: string;
  type: MessageType;
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
