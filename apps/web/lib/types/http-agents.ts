// Agent, model, clarification, plan, and stats types extracted from http.ts

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
  created_at: string;
  updated_at: string;
};

export type TUIConfig = {
  command: string;
  display_name: string;
  model?: string;
  description?: string;
  command_args?: string[];
  wait_for_terminal: boolean;
};

export type Agent = {
  id: string;
  name: string;
  workspace_id?: string | null;
  supports_mcp: boolean;
  mcp_config_path?: string | null;
  tui_config?: TUIConfig | null;
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

// Clarification request types (for ask_user_question feature)
export type ClarificationOption = {
  option_id: string;
  label: string;
  description: string;
};

export type ClarificationQuestion = {
  id: string;
  title: string;
  prompt: string;
  options: ClarificationOption[];
};

export type ClarificationRequestMetadata = {
  pending_id: string;
  session_id: string;
  task_id?: string;
  question: ClarificationQuestion;
  context?: string;
  status?: 'pending' | 'answered' | 'rejected' | 'expired';
  response?: ClarificationAnswer;
};

export type ClarificationAnswer = {
  question_id: string;
  selected_options?: string[];
  custom_text?: string;
};

export type ClarificationResponse = {
  pending_id: string;
  answers: ClarificationAnswer[];
  rejected?: boolean;
  reject_reason?: string;
};

// Task Plan types (for session artifacts)
export type TaskPlan = {
  id: string;
  task_id: string;
  title: string;
  content: string;
  created_by: 'agent' | 'user';
  created_at: string;
  updated_at: string;
};

export type TaskPlanResponse = {
  plan: TaskPlan | null;
};

// Stats types
export type TaskStatsDTO = {
  task_id: string;
  task_title: string;
  workspace_id: string;
  workflow_id: string;
  state: string;
  session_count: number;
  turn_count: number;
  message_count: number;
  user_message_count: number;
  tool_call_count: number;
  total_duration_ms: number;
  created_at: string;
  completed_at?: string;
};

export type GlobalStatsDTO = {
  total_tasks: number;
  completed_tasks: number;
  in_progress_tasks: number;
  total_sessions: number;
  total_turns: number;
  total_messages: number;
  total_user_messages: number;
  total_tool_calls: number;
  total_duration_ms: number;
  avg_turns_per_task: number;
  avg_messages_per_task: number;
  avg_duration_ms_per_task: number;
};

export type DailyActivityDTO = {
  date: string;
  turn_count: number;
  message_count: number;
  task_count: number;
};

export type CompletedTaskActivityDTO = {
  date: string;
  completed_tasks: number;
};

export type AgentUsageDTO = {
  agent_profile_id: string;
  agent_profile_name: string;
  agent_model: string;
  session_count: number;
  turn_count: number;
  total_duration_ms: number;
};

export type RepositoryStatsDTO = {
  repository_id: string;
  repository_name: string;
  total_tasks: number;
  completed_tasks: number;
  in_progress_tasks: number;
  session_count: number;
  turn_count: number;
  message_count: number;
  user_message_count: number;
  tool_call_count: number;
  total_duration_ms: number;
  total_commits: number;
  total_files_changed: number;
  total_insertions: number;
  total_deletions: number;
};

export type GitStatsDTO = {
  total_commits: number;
  total_files_changed: number;
  total_insertions: number;
  total_deletions: number;
};

export type StatsResponse = {
  global: GlobalStatsDTO;
  task_stats: TaskStatsDTO[];
  daily_activity: DailyActivityDTO[];
  completed_activity: CompletedTaskActivityDTO[];
  agent_usage: AgentUsageDTO[];
  repository_stats: RepositoryStatsDTO[];
  git_stats: GitStatsDTO;
};
