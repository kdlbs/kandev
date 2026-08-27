// Automation types matching backend models in internal/automation/models.go

export type TriggerType =
  | "scheduled"
  | "github_pr"
  | "github_pr_merged"
  | "github_push"
  | "github_ci"
  | "webhook";

export type RunStatus =
  | "triggered"
  | "task_created"
  | "succeeded"
  | "failed"
  | "skipped"
  | "archived"
  | "cancelled";

export type ContinuationPolicy = "new_task" | "reuse_thread";
export type TaskMode = "automation_run" | "normal_task";
export type RepositoryMode = "workspace_default" | "selected" | "none";

export type AutomationRepository = {
  repository_id: string;
  base_branch: string;
};

export type Automation = {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  workflow_id: string;
  workflow_step_id: string;
  agent_profile_id: string;
  executor_profile_id: string;
  task_mode?: TaskMode;
  repository_mode?: RepositoryMode;
  repository_ids: string[];
  repositories?: AutomationRepository[];
  prompt: string;
  task_title_template: string;
  enabled: boolean;
  max_concurrent_runs: number;
  /** How later firings get their task and conversation context. */
  continuation_policy?: ContinuationPolicy;
  last_triggered_at: string | null;
  created_at: string;
  updated_at: string;
  triggers: AutomationTrigger[];
  /**
   * This automation predates the withdrawal of execution modes and was stored
   * in the `task` mode — the default — so its firings used to put a card on
   * the kanban and no longer do. The server derives it from a column nothing
   * else reads; it exists to explain the change once, not to describe how the
   * automation runs now, which is the same for every automation.
   *
   * Optional because a backend older than the migration doesn't send it, and
   * because it stops being interesting the moment the notice is dismissed —
   * absent means "nothing changed for this automation".
   */
  legacy_board_card?: boolean;
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
  /** Tail of the agent's last message on the generated task, truncated server-side. */
  summary?: string;
  /**
   * The run's conversation. Absent when the run never produced a task or the
   * task is gone — the detail view mounts its transcript from this, so a run
   * without one is reported rather than offered as something to open.
   */
  session_id?: string;
  /** Exact provider turn represented by this run when a session is shared. */
  turn_id?: string;
  thread_action?: "created" | "resumed" | "replaced";
  thread_reason?: string;
  /** Snapshot of the rendered task title at admission time. */
  display_title?: string;
};

/**
 * A run as it appears in the workspace-wide feed. A per-automation run log can
 * take the automation for granted; a mixed feed cannot, so the server
 * denormalises the name and execution mode onto every row rather than making
 * the client join against the automation list.
 */
export type WorkspaceAutomationRun = AutomationRun & {
  automation_name: string;
};

/**
 * One automation's health, answered per automation.
 *
 * The runs list used to derive this from the workspace feed, which is capped —
 * past the cap a quiet automation's last run falls out of the window and its
 * row claims it has never run. The server answers per automation instead, so a
 * row's two claims do not depend on how noisy its neighbours are.
 */
export type AutomationSummary = {
  automation_id: string;
  /** Open under the same definition the concurrency cap uses. */
  open_runs: number;
  /** Absent when the automation has never run, or its runs were all deleted. */
  last_run?: AutomationRun;
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
  repository_ids?: string[];
  repositories?: AutomationRepository[];
  prompt?: string;
  task_title_template?: string;
  max_concurrent_runs?: number;
  continuation_policy?: ContinuationPolicy;
  task_mode?: TaskMode;
  repository_mode?: RepositoryMode;
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
  repository_ids?: string[];
  repositories?: AutomationRepository[];
  prompt?: string;
  task_title_template?: string;
  enabled?: boolean;
  max_concurrent_runs?: number;
  continuation_policy?: ContinuationPolicy;
  task_mode?: TaskMode;
  repository_mode?: RepositoryMode;
};

// CreateAutomationResponse mirrors the backend's one-time webhook secret
// payload — the server returns the plaintext secret in the create response
// only, never in list/get. The reveal endpoint is the way back to it.
export type CreateAutomationResponse = Automation & {
  webhook_secret: string;
};

export type RevealWebhookSecretResponse = {
  webhook_secret: string;
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
