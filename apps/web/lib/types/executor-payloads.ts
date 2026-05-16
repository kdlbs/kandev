// Executor and environment payload types for WS events

export type ExecutorPayload = {
  id: string;
  name: string;
  type: string;
  status: string;
  is_system: boolean;
  config?: Record<string, string>;
  created_at?: string;
  updated_at?: string;
};

export type ExecutorProfilePayload = {
  id: string;
  executor_id: string;
  name: string;
  mcp_policy?: string;
  config?: Record<string, string>;
  prepare_script: string;
  cleanup_script: string;
  created_at?: string;
  updated_at?: string;
};

export type PrepareProgressPayload = {
  task_id: string;
  session_id: string;
  execution_id: string;
  step_name: string;
  step_command?: string;
  step_index: number;
  total_steps: number;
  status: string;
  output?: string;
  error?: string;
  warning?: string;
  warning_detail?: string;
  started_at?: string;
  ended_at?: string;
  timestamp: string;
};

export type PrepareCompletedPayload = {
  task_id: string;
  session_id: string;
  execution_id: string;
  success: boolean;
  error_message?: string;
  duration_ms: number;
  workspace_path?: string;
  steps?: Array<{
    name: string;
    command?: string;
    status: string;
    output?: string;
    error?: string;
    warning?: string;
    warning_detail?: string;
    started_at?: string;
    ended_at?: string;
  }>;
  timestamp: string;
};

export type EnvironmentPayload = {
  id: string;
  name: string;
  kind: string;
  is_system: boolean;
  worktree_root?: string;
  image_tag?: string;
  dockerfile?: string;
  build_config?: Record<string, string>;
  created_at?: string;
  updated_at?: string;
};
