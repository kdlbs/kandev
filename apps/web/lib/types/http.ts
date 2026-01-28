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

// Workflow Step Types
export type WorkflowStepType =
  | 'backlog'
  | 'planning'
  | 'implementation'
  | 'review'
  | 'verification'
  | 'done'
  | 'blocked';

// Workflow Review Status
export type WorkflowReviewStatus =
  | 'pending'
  | 'approved'
  | 'changes_requested'
  | 'rejected';

// Step Behaviors - controls auto-start and other step-specific behaviors
export type StepBehaviors = {
  autoStartAgent?: boolean;
  planMode?: boolean;
  requireApproval?: boolean;
  promptPrefix?: string;
  promptSuffix?: string;
};

// Workflow Template - pre-defined workflow configurations
export type WorkflowTemplate = {
  id: string;
  name: string;
  description?: string | null;
  is_system: boolean;
  default_steps?: StepDefinition[];
  created_at: string;
  updated_at: string;
};

// Step Definition - template step configuration
export type StepDefinition = {
  name: string;
  step_type: WorkflowStepType;
  position: number;
  color?: string;
  behaviors?: StepBehaviors;
};

// Workflow Step - instance of a step on a board
export type WorkflowStep = {
  id: string;
  board_id: string;
  name: string;
  step_type: WorkflowStepType;
  position: number;
  color: string;
  behaviors?: StepBehaviors;
  created_at: string;
  updated_at: string;
};

// Session Step History - audit trail
export type SessionStepHistory = {
  id: string;
  session_id: string;
  from_step_id?: string;
  to_step_id: string;
  trigger: string;
  actor_id?: string;
  notes?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
};

// Response types for workflow APIs
export type ListWorkflowTemplatesResponse = {
  templates: WorkflowTemplate[];
  total: number;
};

export type ListWorkflowStepsResponse = {
  steps: WorkflowStep[];
  total: number;
};

export type ListSessionStepHistoryResponse = {
  history: SessionStepHistory[];
  total: number;
};

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
  workflow_template_id?: string | null;
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
  worktree_branch_prefix: string;
  pull_before_worktree: boolean;
  setup_script: string;
  cleanup_script: string;
  dev_script: string;
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

export type ProcessOutputChunk = {
  stream: 'stdout' | 'stderr';
  data: string;
  timestamp: string;
};

export type ProcessInfo = {
  id: string;
  session_id: string;
  kind: string;
  script_name?: string;
  command: string;
  working_dir: string;
  status: string;
  exit_code?: number | null;
  started_at: string;
  updated_at: string;
  output?: ProcessOutputChunk[];
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
  workflow_step_id: string;
  position: number;
  title: string;
  description: string;
  state: TaskState;
  priority: number;
  repositories?: TaskRepository[];
  primary_session_id?: string | null;
  created_at: string;
  updated_at: string;
  metadata?: Record<string, unknown> | null;
};

export type CreateTaskResponse = Task & {
  session_id?: string;
  agent_execution_id?: string;
};

// Backend workflow step DTO (flat fields, as returned from API)
export type WorkflowStepDTO = {
  id: string;
  board_id: string;
  name: string;
  step_type: string;
  position: number;
  color: string;
  auto_start_agent: boolean;
  plan_mode: boolean;
  require_approval: boolean;
  prompt_prefix?: string;
  prompt_suffix?: string;
  allow_manual_move: boolean;
};

// Response from moving a task - includes workflow step info for automation
export type MoveTaskResponse = {
  task: Task;
  workflow_step: WorkflowStepDTO;
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
  error_message?: string;
  metadata?: Record<string, unknown> | null;
  agent_profile_snapshot?: Record<string, unknown> | null;
  executor_snapshot?: Record<string, unknown> | null;
  environment_snapshot?: Record<string, unknown> | null;
  repository_snapshot?: Record<string, unknown> | null;
  started_at: string;
  completed_at?: string | null;
  updated_at: string;
  // Workflow fields
  is_primary?: boolean;
  workflow_step_id?: string;
  review_status?: WorkflowReviewStatus;
};

export type TaskSessionsResponse = {
  sessions: TaskSession[];
  total: number;
};

export type TaskSessionResponse = {
  session: TaskSession;
};

export type ApproveSessionResponse = {
  success: boolean;
  session: TaskSession;
  workflow_step?: WorkflowStepDTO;
};

export type NotificationProviderType = 'local' | 'apprise' | 'system';

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
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  updated_at: string;
};

export type UserSettingsResponse = {
  settings: UserSettings;
  shell_options?: Array<{ value: string; label: string }>;
};

export type EditorOption = {
  id: string;
  type: string;
  name: string;
  kind: string;
  command?: string;
  scheme?: string;
  config?: Record<string, unknown>;
  installed: boolean;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
};

export type EditorsResponse = {
  editors: EditorOption[];
};

export type CustomPrompt = {
  id: string;
  name: string;
  content: string;
  created_at: string;
  updated_at: string;
};

export type PromptsResponse = {
  prompts: CustomPrompt[];
};

export type UserResponse = {
  user: User;
  settings: UserSettings;
};

export type BoardSnapshot = {
  board: Board;
  steps: WorkflowStepDTO[];
  tasks: Task[];
};

export type ListBoardsResponse = {
  boards: Board[];
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
  | 'log'
  | 'error'
  | 'status'
  | 'thinking'
  | 'todo'
  | 'permission_request'
  | 'script_execution';

export type Message = {
  id: string;
  session_id: string;
  task_id: string;
  turn_id?: string;
  author_type: MessageAuthorType;
  author_id?: string;
  content: string;
  type: MessageType;
  metadata?: Record<string, unknown>;
  requests_input?: boolean;
  created_at: string;
};

export type Turn = {
  id: string;
  session_id: string;
  task_id: string;
  started_at: string;
  completed_at?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type AgentProfile = {
  id: string;
  agent_id: string;
  name: string;
  agent_display_name: string;
  model: string;
  auto_approve: boolean;
  dangerously_skip_permissions: boolean;
  allow_indexing: boolean;
  cli_passthrough: boolean;
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

export type McpServerType = 'stdio' | 'http' | 'sse' | 'streamable_http';
export type McpServerMode = 'shared' | 'per_session' | 'auto';

export type McpServerDef = {
  type?: McpServerType;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
  mode?: McpServerMode;
  meta?: Record<string, unknown>;
  extra?: Record<string, unknown>;
};

export type AgentProfileMcpConfig = {
  profile_id: string;
  enabled: boolean;
  servers: Record<string, McpServerDef>;
  meta?: Record<string, unknown>;
};

export type AgentDiscovery = {
  name: string;
  supports_mcp: boolean;
  mcp_config_path?: string | null;
  installation_paths: string[];
  available: boolean;
  matched_path?: string | null;
};

export type AgentCapabilities = {
  supports_session_resume: boolean;
  supports_shell: boolean;
  supports_workspace_only: boolean;
};

export type ModelEntry = {
  id: string;
  name: string;
  provider: string;
  context_window: number;
  is_default: boolean;
  source?: 'static' | 'dynamic';
};

export type ModelConfig = {
  default_model: string;
  available_models: ModelEntry[];
  supports_dynamic_models: boolean;
};

export type DynamicModelsResponse = {
  agent_name: string;
  models: ModelEntry[];
  cached: boolean;
  cached_at?: string;
  error: string | null;
};

export type PermissionSetting = {
  supported: boolean;
  default: boolean;
  label: string;
  description: string;
  apply_method?: string;
  cli_flag?: string;
  cli_flag_value?: string;
};

export type PassthroughConfig = {
  supported: boolean;
  label: string;
  description: string;
};

export type AvailableAgent = {
  name: string;
  display_name: string;
  supports_mcp: boolean;
  mcp_config_path?: string | null;
  installation_paths: string[];
  available: boolean;
  matched_path?: string | null;
  capabilities: AgentCapabilities;
  model_config: ModelConfig;
  permission_settings?: Record<string, PermissionSetting>;
  passthrough_config?: PassthroughConfig;
  updated_at: string;
};

export type ListAgentsResponse = {
  agents: Agent[];
  total: number;
};

export type ListAgentDiscoveryResponse = {
  agents: AgentDiscovery[];
  total: number;
};

export type ListAvailableAgentsResponse = {
  agents: AvailableAgent[];
  total: number;
};
