import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { TaskPlanEventPayload, TaskPlanRevisionEventPayload } from "./task-plan-events";
import type { CaptureRequest } from "@/lib/logger/capture";

export const SYSTEM_AGENT_RUNTIME_STATUS_CHANGED = "system.agent_runtime.status_changed" as const;

export type BackendMessageType = keyof BackendMessageMap;

export type { BackendMessage } from "./backend-message";
import type { BackendMessage } from "./backend-message";
import type { OfficeBackendMessageMap } from "./office-events";
export type { OfficeEventType, OfficeEventPayload } from "./office-events";
import type { RunEventAppendedPayload } from "./run-events";
export type { RunEventAppendedPayload } from "./run-events";

import type {
  AvailableAgent,
  ForegroundActivity,
  TaskPendingAction,
  TaskSessionState,
  StepEvents,
  TaskState,
  ToolStatus,
  UserSettings,
} from "@/lib/types/http";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { GitEventPayload } from "@/lib/types/git-events";
import type {
  GitHubRateLimitUpdate,
  TaskCIAutomationOptions,
  TaskPR,
  TaskPRDeletedEvent,
} from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { TaskMRAutomationOptions } from "@/lib/types/gitlab";
import type { SystemMetricsSnapshot } from "./system";
import type { AgentRuntimeAvailability } from "./agent-runtime";
import type { FileChangeNotificationPayload } from "./workspace-files";
import type {
  AgentCapabilitiesPayload,
  SessionInfoPayload,
  SessionModelsPayload,
  SessionMCPStatusPayload,
  SessionPromptUsagePayload,
  SessionTodosPayload,
} from "./session-runtime-payloads";
import type {
  ExecutorPayload,
  ExecutorProfilePayload,
  PrepareProgressPayload,
  PrepareCompletedPayload,
  EnvironmentPayload,
} from "./executor-payloads";

export type KanbanUpdatePayload = {
  workflowId: string;
  steps: Array<{
    id: string;
    title: string;
    color?: string;
    position?: number;
    events?: {
      on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
      on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    };
    show_in_command_panel?: boolean;
    wip_limit?: number;
    pull_from_step_id?: string | null;
  }>;
  tasks: Array<{
    id: string;
    workflowStepId: string;
    title: string;
    position?: number;
    description?: string;
    state?: TaskState;
  }>;
};

export type TaskEventPayload = {
  task_id: string;
  workspace_id?: string;
  workflow_id: string;
  old_workflow_id?: string | null;
  workflow_step_id: string;
  title: string;
  description?: string;
  state?: TaskState;
  priority?: number;
  wip_admitted?: boolean;
  queued_for_step_id?: string | null;
  queued_at?: string | null;
  position?: number;
  repository_id?: string;
  repositories?: Array<{
    id?: string;
    repository_id: string;
    base_branch?: string;
    checkout_branch?: string;
    position?: number;
  }>;
  primary_session_id?: string | null;
  primary_session_state?: TaskSessionState | null;
  primary_session_pending_action?: TaskPendingAction | null;
  task_pending_action?: TaskPendingAction | null;
  // Task-level MOST-ACTIVE-WINS activity aggregate across the task's sessions;
  // absent/null when no session is running.
  foreground_activity?: ForegroundActivity | null;
  active_subagent_count?: number;
  session_count?: number | null;
  review_status?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  archived_at?: string | null;
  updated_at?: string;
  is_ephemeral: boolean;
  /** Task origin (e.g. "manual", "automation_run"). */
  origin?: string;
  parent_id?: string | null;
  metadata?: Record<string, unknown> | null;
  /** Deletion reason on task.deleted (e.g. "pr_approved_by_user"). Absent otherwise. */
  reason?: string;
  status_summary?: TaskStatusSummary | null;
};

export type AgentUpdatePayload = {
  agentId: string;
  status: "idle" | "running" | "error";
  message?: string;
};

export type AgentAvailableUpdatedPayload = {
  agents: AvailableAgent[];
  tools?: ToolStatus[];
};

export type AgentInstallJobPayload = {
  job_id: string;
  agent_name: string;
  status: "queued" | "running" | "succeeded" | "failed";
  output?: string;
  error?: string;
  exit_code?: number;
  started_at: string;
  finished_at?: string;
};

export type AgentInstallOutputPayload = {
  job_id: string;
  agent_name: string;
  chunk: string;
};

export type AgentUpdateJobPayload = {
  job_id: string;
  agent_name: string;
  status: "queued" | "resolving" | "updating" | "refreshing" | "succeeded" | "failed";
  current_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

export type AgentUpdateOutputPayload = {
  job_id: string;
  agent_name: string;
  chunk: string;
};

export type TerminalOutputPayload = {
  terminalId: string;
  data: string;
  stream?: "stdout" | "stderr";
};

export type DiffUpdatePayload = {
  taskId: string;
  files: Array<{
    path: string;
    status: "A" | "M" | "D";
    plus: number;
    minus: number;
  }>;
};

export type UpdateAvailablePayload = {
  version: string;
  url?: string;
  title: string;
  body: string;
  occurrence_id: string;
};

export type WorkspacePayload = {
  id: string;
  name: string;
  description?: string;
  owner_id?: string;
  default_executor_id?: string | null;
  default_environment_id?: string | null;
  default_agent_profile_id?: string | null;
  default_config_agent_profile_id?: string | null;
  created_at?: string;
  updated_at?: string;
};

export type WorkflowPayload = {
  id: string;
  workspace_id: string;
  name: string;
  description?: string;
  prompt?: string;
  agent_profile_id?: string;
  hidden?: boolean;
  /** Phase 2 (ADR-0004) UX hint — frontend-only. */
  style?: "kanban" | "office" | "custom";
  created_at?: string;
  updated_at?: string;
};

export type StepPayload = {
  id: string;
  workflow_id: string;
  name: string;
  position: number;
  state: string;
  color: string;
  prompt?: string;
  events?: StepEvents;
  is_start_step?: boolean;
  allow_manual_move?: boolean;
  show_in_command_panel?: boolean;
  auto_archive_after_hours?: number;
  agent_profile_id?: string;
  wip_limit?: number;
  pull_from_step_id?: string | null;
  /** Phase 2 (ADR-0004) UX hint — frontend-only. */
  stage_type?: "work" | "review" | "approval" | "custom";
  created_at?: string;
  updated_at?: string;
};

export type WorkflowStepEventPayload = {
  step: StepPayload;
};

export type MessageAddedPayload = {
  task_id: string;
  message_id: string;
  session_id: string;
  turn_id?: string;
  author_type: "user" | "agent";
  author_id?: string;
  content: string;
  raw_content?: string;
  type?: string;
  metadata?: Record<string, unknown>;
  requests_input?: boolean;
  created_at: string;
  updated_at?: string;
};

export type TaskSessionStateChangedPayload = {
  task_id: string;
  session_id: string;
  old_state?: string;
  new_state?: string;
  /** Authoritative row timestamp — used to drop out-of-order subscribe snapshots. */
  updated_at?: string;
  /**
   * Agent profile id — drives the per-agent live-session selectors on the
   * sidebar. Empty for sessions launched without a profile.
   */
  agent_profile_id?: string;
  agent_profile_snapshot?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  session_metadata?: Record<string, unknown>;
  is_passthrough?: boolean;
  error_message?: string;
  /** User-supplied session tab label; present (possibly "") on rename broadcasts. */
  name?: string;
  /** When true, the frontend should not show an error toast for this state change. */
  suppress_toast?: boolean;
  // Workflow-related fields (sent during workflow transitions)
  review_status?: string;
  // Task environment (for session→environment mapping)
  task_environment_id?: string;
  // Fine-grained busy substate (see ADR-0049), carried on coarse transitions;
  // live flips arrive on session.activity_changed.
  foreground_activity?: ForegroundActivity | null;
  active_subagent_count?: number;
  /** Backend-owned cancellation projection carried by state snapshots. */
  cancellation_pending?: boolean;
  /** Process-local cancellation transition generation carried by state snapshots. */
  cancellation_revision?: number;
  /** True when a send right now would steer the running turn; see http.ts. */
  supports_steering?: boolean;
};

/**
 * Payload for `session.activity_changed` — the fine-grained busy signal
 * (see ADR-0049). Fires when foreground ownership or detached background
 * liveness changes, including after the foreground turn settles.
 */
export type TaskSessionActivityChangedPayload = {
  task_id: string;
  session_id: string;
  foreground_activity: ForegroundActivity | null;
  active_subagent_count: number;
  /** True when a send right now would steer the running turn; see http.ts. */
  supports_steering?: boolean;
};

export type TaskSessionCancellationChangedPayload = {
  session_id: string;
  cancellation_pending: boolean;
  cancellation_revision: number;
};

export type TaskSessionNotificationPayload = {
  task_id: string;
  session_id: string;
  occurrence_id: string;
  title: string;
  body: string;
};

export type OfficeInboxItemNotificationPayload = {
  task_id?: string;
  session_id?: string;
  title: string;
  body: string;
};

export type TaskSessionAgentctlPayload = {
  task_id: string;
  session_id: string;
  task_environment_id?: string;
  agent_execution_id?: string;
  error_message?: string;
  worktree_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  /** Effective task workspace root when the agentctl payload carries it. */
  workspace_path?: string;
  /** Task root that contains every per-repo worktree as a sibling subdir.
   *  Set only when the event signals a sibling worktree addition (multi-branch
   *  add_branch flow) — the frontend repoints the file browser to it instead of
   *  staying on the original primary worktree. */
  task_workspace_path?: string;
};

export type FileInfo = {
  path: string;
  status: "modified" | "added" | "deleted" | "untracked" | "renamed";
  staged: boolean;
  additions?: number;
  deletions?: number;
  old_path?: string;
  diff?: string;
  diff_skip_reason?: "too_large" | "binary" | "truncated" | "budget_exceeded";
};

export type ProcessOutputPayload = {
  session_id: string;
  process_id: string;
  kind: string;
  stream: "stdout" | "stderr";
  data: string;
  timestamp?: string;
};

export type ProcessStatusPayload = {
  session_id: string;
  process_id: string;
  kind: string;
  script_name?: string;
  status: string;
  command?: string;
  working_dir?: string;
  exit_code?: number | null;
  timestamp?: string;
};

// Executor and environment payload types (extracted to reduce file size)
export {
  type ExecutorPayload,
  type ExecutorProfilePayload,
  type PrepareProgressPayload,
  type PrepareCompletedPayload,
  type EnvironmentPayload,
} from "./executor-payloads";

export type AgentProfilePayload = {
  id: string;
  agent_id: string;
  name: string;
  agent_display_name: string;
  model: string;
  auto_approve: boolean;
  dangerously_skip_permissions: boolean;
  allow_indexing: boolean;
  cli_passthrough?: boolean;
  plan: string;
  created_at?: string;
  updated_at?: string;
};

export type AgentProfileDeletedPayload = {
  profile: AgentProfilePayload;
};

export type AgentProfileChangedPayload = {
  profile: AgentProfilePayload;
};

export type UserSettingsUpdatedPayload = Omit<
  Partial<UserSettings>,
  "user_id" | "workspace_id" | "repository_ids"
> & {
  user_id: string;
  workspace_id: string;
  repository_ids: string[];
};
export type ShellOutputPayload = {
  task_id: string;
  session_id: string;
  type: "output" | "exit";
  data?: string;
  code?: number;
};

export type TurnEventPayload = {
  id: string;
  session_id: string;
  task_id: string;
  started_at: string;
  completed_at?: string;
  metadata?: Record<string, unknown>;
  /** Whether the completed turn produced any agent output. Only set on turn.completed. */
  had_output?: boolean;
  created_at: string;
  updated_at: string;
};

export type AvailableCommandPayload = {
  name: string;
  description?: string;
  input_hint?: string;
};

export type AvailableCommandsPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  available_commands: AvailableCommandPayload[];
  timestamp: string;
};

export type SessionModeChangedPayload = {
  task_id: string;
  session_id: string;
  agent_id: string;
  current_mode_id: string;
  available_modes?: {
    id: string;
    name: string;
    description?: string;
  }[];
  timestamp?: string;
};

// Session runtime payload types (extracted to reduce file size)
export {
  type AuthMethodInfoPayload,
  type AgentCapabilitiesPayload,
  type SessionModelInfoPayload,
  type ConfigOptionPayload,
  type SessionModelsPayload,
  type SessionMCPStatusPayload,
  type SessionInfoPayload,
  type SessionTodosPayload,
} from "./session-runtime-payloads";

export type { TaskPlanEventPayload, TaskPlanRevisionEventPayload } from "./task-plan-events";

export type QueueStatusChangedPayload = {
  session_id: string;
  entries?: QueuedMessage[] | null;
  count?: number;
  max?: number;
  merge_enabled?: boolean;
};

export type TaskStatusSummaryUpdatedPayload = {
  task_id: string;
  workspace_id: string;
  status_summary: TaskStatusSummary;
};

export type BackendMessageMap = OfficeBackendMessageMap &
  import("@/lib/types/http").WalkthroughBackendMessageMap &
  import("@/lib/types/review").ReviewBackendMessageMap & {
    "kanban.update": BackendMessage<"kanban.update", KanbanUpdatePayload>;
    "task.created": BackendMessage<"task.created", TaskEventPayload>;
    "task.updated": BackendMessage<"task.updated", TaskEventPayload>;
    "task.deleted": BackendMessage<"task.deleted", TaskEventPayload>;
    "task.state_changed": BackendMessage<"task.state_changed", TaskEventPayload>;
    "task.status_summary.updated": BackendMessage<
      "task.status_summary.updated",
      TaskStatusSummaryUpdatedPayload
    >;
    "task.plan.created": BackendMessage<"task.plan.created", TaskPlanEventPayload>;
    "task.plan.updated": BackendMessage<"task.plan.updated", TaskPlanEventPayload>;
    "task.plan.deleted": BackendMessage<"task.plan.deleted", TaskPlanEventPayload>;
    "task.plan.revision.created": BackendMessage<
      "task.plan.revision.created",
      TaskPlanRevisionEventPayload
    >;
    "task.plan.reverted": BackendMessage<"task.plan.reverted", TaskPlanRevisionEventPayload>;
    "agent.updated": BackendMessage<"agent.updated", AgentUpdatePayload>;
    "agent.available.updated": BackendMessage<
      "agent.available.updated",
      AgentAvailableUpdatedPayload
    >;
    "agent.install.started": BackendMessage<"agent.install.started", AgentInstallJobPayload>;
    "agent.install.output": BackendMessage<"agent.install.output", AgentInstallOutputPayload>;
    "agent.install.finished": BackendMessage<"agent.install.finished", AgentInstallJobPayload>;
    "agent.update.started": BackendMessage<"agent.update.started", AgentUpdateJobPayload>;
    "agent.update.output": BackendMessage<"agent.update.output", AgentUpdateOutputPayload>;
    "agent.update.finished": BackendMessage<"agent.update.finished", AgentUpdateJobPayload>;
    "terminal.output": BackendMessage<"terminal.output", TerminalOutputPayload>;
    "diff.update": BackendMessage<"diff.update", DiffUpdatePayload>;
    "session.git.event": BackendMessage<"session.git.event", GitEventPayload>;
    "system.job.update": BackendMessage<"system.job.update", import("./system").SystemJob>;
    "system.metrics.updated": BackendMessage<"system.metrics.updated", SystemMetricsSnapshot>;
    [SYSTEM_AGENT_RUNTIME_STATUS_CHANGED]: BackendMessage<
      typeof SYSTEM_AGENT_RUNTIME_STATUS_CHANGED,
      AgentRuntimeAvailability
    >;
    "system.logs.capture_requested": BackendMessage<
      "system.logs.capture_requested",
      CaptureRequest
    >;
    "system.update_available": BackendMessage<"system.update_available", UpdateAvailablePayload>;
    "workspace.created": BackendMessage<"workspace.created", WorkspacePayload>;
    "workspace.updated": BackendMessage<"workspace.updated", WorkspacePayload>;
    "workspace.deleted": BackendMessage<"workspace.deleted", WorkspacePayload>;
    "workflow.created": BackendMessage<"workflow.created", WorkflowPayload>;
    "workflow.updated": BackendMessage<"workflow.updated", WorkflowPayload>;
    "workflow.deleted": BackendMessage<"workflow.deleted", WorkflowPayload>;
    "workflow.step.created": BackendMessage<"workflow.step.created", WorkflowStepEventPayload>;
    "workflow.step.updated": BackendMessage<"workflow.step.updated", WorkflowStepEventPayload>;
    "workflow.step.deleted": BackendMessage<"workflow.step.deleted", WorkflowStepEventPayload>;
    "session.message.added": BackendMessage<"session.message.added", MessageAddedPayload>;
    "session.message.updated": BackendMessage<"session.message.updated", MessageAddedPayload>;
    "session.message.deleted": BackendMessage<"session.message.deleted", MessageAddedPayload>;
    "session.state_changed": BackendMessage<
      "session.state_changed",
      TaskSessionStateChangedPayload
    >;
    "session.turn_finished": BackendMessage<
      "session.turn_finished",
      TaskSessionNotificationPayload
    >;
    "session.activity_changed": BackendMessage<
      "session.activity_changed",
      TaskSessionActivityChangedPayload
    >;
    "session.cancellation_changed": BackendMessage<
      "session.cancellation_changed",
      TaskSessionCancellationChangedPayload
    >;
    "session.clarification_requested": BackendMessage<
      "session.clarification_requested",
      TaskSessionNotificationPayload
    >;
    "office.inbox_item": BackendMessage<"office.inbox_item", OfficeInboxItemNotificationPayload>;
    "session.agentctl_starting": BackendMessage<
      "session.agentctl_starting",
      TaskSessionAgentctlPayload
    >;
    "session.agentctl_ready": BackendMessage<"session.agentctl_ready", TaskSessionAgentctlPayload>;
    "session.agentctl_error": BackendMessage<"session.agentctl_error", TaskSessionAgentctlPayload>;
    "session.workspace_sources.updated": BackendMessage<
      "session.workspace_sources.updated",
      {
        task_id: string;
        session_id: string;
        workspace_path: string;
        adopted_session_ids?: string[];
      }
    >;
    "session.turn.started": BackendMessage<"session.turn.started", TurnEventPayload>;
    "session.turn.completed": BackendMessage<"session.turn.completed", TurnEventPayload>;
    "session.available_commands": BackendMessage<
      "session.available_commands",
      AvailableCommandsPayload
    >;
    "session.mode_changed": BackendMessage<"session.mode_changed", SessionModeChangedPayload>;
    "session.agent_capabilities": BackendMessage<
      "session.agent_capabilities",
      AgentCapabilitiesPayload
    >;
    "session.models_updated": BackendMessage<"session.models_updated", SessionModelsPayload>;
    "session.mcp_status_updated": BackendMessage<
      "session.mcp_status_updated",
      SessionMCPStatusPayload
    >;
    "session.info_updated": BackendMessage<"session.info_updated", SessionInfoPayload>;
    "session.todos_updated": BackendMessage<"session.todos_updated", SessionTodosPayload>;
    "session.prompt_usage": BackendMessage<"session.prompt_usage", SessionPromptUsagePayload>;
    "session.poll_mode_changed": BackendMessage<
      "session.poll_mode_changed",
      { session_id: string; poll_mode: string }
    >;
    "executor.created": BackendMessage<"executor.created", ExecutorPayload>;
    "executor.updated": BackendMessage<"executor.updated", ExecutorPayload>;
    "executor.deleted": BackendMessage<"executor.deleted", ExecutorPayload>;
    "executor.profile.created": BackendMessage<"executor.profile.created", ExecutorProfilePayload>;
    "executor.profile.updated": BackendMessage<"executor.profile.updated", ExecutorProfilePayload>;
    "executor.profile.deleted": BackendMessage<"executor.profile.deleted", { id: string }>;
    "executor.prepare.progress": BackendMessage<
      "executor.prepare.progress",
      PrepareProgressPayload
    >;
    "executor.prepare.completed": BackendMessage<
      "executor.prepare.completed",
      PrepareCompletedPayload
    >;
    "environment.created": BackendMessage<"environment.created", EnvironmentPayload>;
    "environment.updated": BackendMessage<"environment.updated", EnvironmentPayload>;
    "environment.deleted": BackendMessage<"environment.deleted", EnvironmentPayload>;
    "agent.profile.deleted": BackendMessage<"agent.profile.deleted", AgentProfileDeletedPayload>;
    "agent.profile.created": BackendMessage<"agent.profile.created", AgentProfileChangedPayload>;
    "agent.profile.updated": BackendMessage<"agent.profile.updated", AgentProfileChangedPayload>;
    "user.settings.updated": BackendMessage<"user.settings.updated", UserSettingsUpdatedPayload>;
    "session.workspace.file.changes": BackendMessage<
      "session.workspace.file.changes",
      FileChangeNotificationPayload
    >;
    "session.shell.output": BackendMessage<"session.shell.output", ShellOutputPayload>;
    "session.process.output": BackendMessage<"session.process.output", ProcessOutputPayload>;
    "session.process.status": BackendMessage<"session.process.status", ProcessStatusPayload>;
    "secrets.created": BackendMessage<"secrets.created", SecretListItem>;
    "secrets.updated": BackendMessage<"secrets.updated", SecretListItem>;
    "secrets.deleted": BackendMessage<"secrets.deleted", { id: string }>;
    "message.queue.status_changed": BackendMessage<
      "message.queue.status_changed",
      QueueStatusChangedPayload
    >;
    "github.task_pr.updated": BackendMessage<"github.task_pr.updated", TaskPR>;
    "github.task_pr.deleted": BackendMessage<"github.task_pr.deleted", TaskPRDeletedEvent>;
    "github.task_ci_options.updated": BackendMessage<
      "github.task_ci_options.updated",
      TaskCIAutomationOptions
    >;
    "github.rate_limit.updated": BackendMessage<"github.rate_limit.updated", GitHubRateLimitUpdate>;
    "gitlab.task_mr.updated": BackendMessage<
      "gitlab.task_mr.updated",
      TaskMR & { workspace_id: string }
    >;
    "gitlab.task_mr_options.updated": BackendMessage<
      "gitlab.task_mr_options.updated",
      TaskMRAutomationOptions
    >;
    "run.event.appended": BackendMessage<"run.event.appended", RunEventAppendedPayload>;
  };

// Workspace file types (extracted to reduce file size)
export * from "./workspace-files";
