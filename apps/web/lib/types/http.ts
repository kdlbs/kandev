export type TaskState =
  | "CREATED"
  | "SCHEDULING"
  | "TODO"
  | "IN_PROGRESS"
  | "REVIEW"
  | "BLOCKED"
  | "WAITING_FOR_INPUT"
  | "COMPLETED"
  | "FAILED"
  | "CANCELLED";

// On Enter action types
export type OnEnterActionType = "enable_plan_mode" | "auto_start_agent" | "reset_agent_context";

// On Turn Start action types
export type OnTurnStartActionType = "move_to_next" | "move_to_previous" | "move_to_step";

// On Turn Complete action types
export type OnTurnCompleteActionType =
  | "move_to_next"
  | "move_to_previous"
  | "move_to_step"
  | "disable_plan_mode";

// On Exit action types
export type OnExitActionType = "disable_plan_mode";

export type OnEnterAction = {
  type: OnEnterActionType;
  config?: Record<string, unknown>;
};

export type OnTurnStartAction = {
  type: OnTurnStartActionType;
  config?: Record<string, unknown>;
};

export type OnTurnCompleteAction = {
  type: OnTurnCompleteActionType;
  config?: Record<string, unknown>;
};

export type OnExitAction = {
  type: OnExitActionType;
  config?: Record<string, unknown>;
};

export type StepEvents = {
  on_enter?: OnEnterAction[];
  on_turn_start?: OnTurnStartAction[];
  on_turn_complete?: OnTurnCompleteAction[];
  on_exit?: OnExitAction[];
};

// Workflow Review Status
export type WorkflowReviewStatus = "pending" | "approved" | "changes_requested" | "rejected";

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
  position: number;
  color?: string;
  prompt?: string;
  events?: StepEvents;
  is_start_step?: boolean;
};

// Workflow Step - instance of a step on a workflow
export type WorkflowStep = {
  id: string;
  workflow_id: string;
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: StepEvents;
  allow_manual_move?: boolean;
  is_start_step?: boolean;
  auto_archive_after_hours?: number;
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
  | "CREATED"
  | "STARTING"
  | "RUNNING"
  | "WAITING_FOR_INPUT"
  | "COMPLETED"
  | "FAILED"
  | "CANCELLED";

export type Workflow = {
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
  scripts?: RepositoryScript[];
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
  stream: "stdout" | "stderr";
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
  workflow_id: string;
  workflow_step_id: string;
  position: number;
  title: string;
  description: string;
  state: TaskState;
  priority: number;
  repositories?: TaskRepository[];
  primary_session_id?: string | null;
  session_count?: number | null;
  review_status?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primary_executor_id?: string | null;
  primary_executor_type?: string | null;
  primary_executor_name?: string | null;
  is_remote_executor?: boolean;
  archived_at?: string | null;
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
  workflow_id: string;
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: StepEvents;
  allow_manual_move: boolean;
  is_start_step?: boolean;
  auto_archive_after_hours?: number;
  created_at?: string;
  updated_at?: string;
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
  is_passthrough?: boolean;
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

export type NotificationProviderType = "local" | "apprise" | "system";

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

export type SavedLayout = {
  id: string;
  name: string;
  is_default: boolean;
  layout: Record<string, unknown>;
  created_at: string;
};

export type UserSettings = {
  user_id: string;
  workspace_id: string;
  kanban_view_mode?: string;
  workflow_filter_id?: string;
  repository_ids: string[];
  initial_setup_complete?: boolean;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  chat_submit_key?: "enter" | "cmd_enter";
  review_auto_mark_on_scroll?: boolean;
  lsp_auto_start_languages?: string[];
  lsp_auto_install_languages?: string[];
  lsp_server_configs?: Record<string, Record<string, unknown>>;
  saved_layouts?: SavedLayout[];
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
  builtin: boolean;
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

export type WorkflowSnapshot = {
  workflow: Workflow;
  steps: WorkflowStepDTO[];
  tasks: Task[];
};

export type ListWorkflowsResponse = {
  workflows: Workflow[];
  total: number;
};

export type ListTasksResponse = {
  tasks: Task[];
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
  type: "local" | "remote";
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
  profiles?: ExecutorProfile[];
  created_at: string;
  updated_at: string;
};

export type ProfileEnvVar = {
  key: string;
  value?: string;
  secret_id?: string;
};

export type ExecutorProfile = {
  id: string;
  executor_id: string;
  executor_type?: string;
  executor_name?: string;
  name: string;
  mcp_policy?: string;
  config?: Record<string, string>;
  prepare_script: string;
  cleanup_script: string;
  env_vars?: ProfileEnvVar[];
  created_at: string;
  updated_at: string;
};

export type ListExecutorProfilesResponse = {
  profiles: ExecutorProfile[];
  total: number;
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

export type MessageAuthorType = "user" | "agent";
export type MessageType =
  | "message"
  | "content"
  | "tool_call"
  | "tool_edit"
  | "tool_read"
  | "tool_search"
  | "tool_execute"
  | "progress"
  | "log"
  | "error"
  | "status"
  | "thinking"
  | "todo"
  | "permission_request"
  | "clarification_request"
  | "script_execution";

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

export type ListTurnsResponse = {
  turns: Turn[];
  total: number;
};

export * from "./http-agents";

// Workflow Export/Import types
export type WorkflowExportData = {
  version: number;
  type: string;
  workflows: WorkflowPortable[];
};

export type WorkflowPortable = {
  name: string;
  description?: string;
  steps: StepPortable[];
};

export type StepPortable = {
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events: StepEvents;
  is_start_step: boolean;
  allow_manual_move: boolean;
  auto_archive_after_hours?: number;
};

export type ImportWorkflowsResult = {
  created: string[];
  skipped: string[];
};

// Helper function to check if a step has a specific on_enter action
export function stepHasOnEnterAction(
  step: { events?: StepEvents },
  actionType: OnEnterActionType,
): boolean {
  return step.events?.on_enter?.some((a) => a.type === actionType) ?? false;
}
