// Automation types matching backend models in internal/automation/models.go

export type TriggerType = "scheduled" | "github_pr" | "github_push" | "github_ci" | "webhook";

export type RunStatus = "triggered" | "task_created" | "failed" | "skipped";

export type Automation = {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  workflow_id: string;
  workflow_step_id: string;
  agent_profile_id: string;
  executor_profile_id: string;
  prompt: string;
  task_title_template: string;
  enabled: boolean;
  max_concurrent_runs: number;
  last_triggered_at: string | null;
  created_at: string;
  updated_at: string;
  triggers: AutomationTrigger[];
};

export type AutomationTrigger = {
  id: string;
  automation_id: string;
  type: TriggerType;
  config: Record<string, unknown>;
  enabled: boolean;
  last_evaluated_at: string | null;
  created_at: string;
  updated_at: string;
};

export type AutomationRun = {
  id: string;
  automation_id: string;
  trigger_id: string;
  trigger_type: TriggerType;
  task_id: string;
  status: RunStatus;
  dedup_key: string;
  trigger_data: Record<string, unknown>;
  error_message: string;
  created_at: string;
};

// --- Trigger config types ---

export type ScheduledTriggerConfig = {
  cron_expression: string;
  timezone?: string;
};

export type GitHubPRTriggerConfig = {
  events: string[];
  repos: Array<{ owner: string; name: string }>;
  branches?: string[];
  authors?: string[];
  labels?: string[];
  exclude_draft?: boolean;
};

export type GitHubPushTriggerConfig = {
  repos: Array<{ owner: string; name: string }>;
  branches: string[];
};

export type GitHubCITriggerConfig = {
  repos: Array<{ owner: string; name: string }>;
  conclusions: string[];
  check_names?: string[];
};

export type WebhookTriggerConfig = {
  filter_expression?: string;
};

// --- Trigger type metadata (from backend registry) ---

export type PlaceholderInfo = {
  key: string;
  description: string;
  example: string;
};

export type TriggerTypeInfo = {
  type: TriggerType;
  label: string;
  description: string;
  category: string;
  enabled: boolean;
  placeholders: PlaceholderInfo[];
  default_prompt: string;
  default_task_title: string;
  default_config: Record<string, unknown>;
};

// --- Request/response DTOs ---

export type CreateAutomationRequest = {
  workspace_id: string;
  name: string;
  description?: string;
  workflow_id: string;
  workflow_step_id: string;
  agent_profile_id: string;
  executor_profile_id: string;
  prompt?: string;
  task_title_template?: string;
  max_concurrent_runs?: number;
  triggers?: Array<{
    type: TriggerType;
    config: Record<string, unknown>;
    enabled: boolean;
  }>;
};

export type UpdateAutomationRequest = {
  name?: string;
  description?: string;
  workflow_id?: string;
  workflow_step_id?: string;
  agent_profile_id?: string;
  executor_profile_id?: string;
  prompt?: string;
  task_title_template?: string;
  enabled?: boolean;
  max_concurrent_runs?: number;
};

export type AddTriggerRequest = {
  automation_id: string;
  type: TriggerType;
  config: Record<string, unknown>;
  enabled: boolean;
};

export type UpdateTriggerRequest = {
  config?: Record<string, unknown>;
  enabled?: boolean;
};
