export type BackendMessageType =
  | "kanban.update"
  | "task.created"
  | "task.updated"
  | "task.deleted"
  | "task.state_changed"
  | "task.plan.created"
  | "task.plan.updated"
  | "task.plan.deleted"
  | "agent.updated"
  | "agent.available.updated"
  | "terminal.output"
  | "diff.update"
  | "session.git.event"
  | "system.error"
  | "workspace.created"
  | "workspace.updated"
  | "workspace.deleted"
  | "workflow.created"
  | "workflow.updated"
  | "workflow.deleted"
  | "workflow.step.created"
  | "workflow.step.updated"
  | "workflow.step.deleted"
  | "session.message.added"
  | "session.message.updated"
  | "session.state_changed"
  | "session.waiting_for_input"
  | "session.agentctl_starting"
  | "session.agentctl_ready"
  | "session.agentctl_error"
  | "session.turn.started"
  | "session.turn.completed"
  | "session.available_commands"
  | "session.mode_changed"
  | "session.agent_capabilities"
  | "session.models_updated"
  | "session.todos_updated"
  | "session.prompt_usage"
  | "session.poll_mode_changed"
  | "executor.created"
  | "executor.updated"
  | "executor.deleted"
  | "executor.profile.created"
  | "executor.profile.updated"
  | "executor.profile.deleted"
  | "executor.prepare.progress"
  | "executor.prepare.completed"
  | "environment.created"
  | "environment.updated"
  | "environment.deleted"
  | "agent.profile.deleted"
  | "agent.profile.created"
  | "agent.profile.updated"
  | "user.settings.updated"
  | "session.workspace.file.changes"
  | "session.shell.output"
  | "session.process.output"
  | "session.process.status"
  | "secrets.created"
  | "secrets.updated"
  | "secrets.deleted"
  | "message.queue.status_changed"
  | "github.task_pr.updated";

export type BackendMessage<T extends BackendMessageType, P> = {
  id?: string;
  type: "request" | "response" | "notification" | "error";
  action: T;
  payload: P;
  timestamp?: string;
};

import type {
  AvailableAgent,
  SavedLayout,
  StepEvents,
  TaskState,
  ToolStatus,
} from "@/lib/types/http";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { GitEventPayload } from "@/lib/types/git-events";
import type { TaskPR } from "@/lib/types/github";
import type { FileChangeNotificationPayload } from "./workspace-files";
import type {
  AgentCapabilitiesPayload,
  SessionModelsPayload,
  SessionPromptUsagePayload,
  SessionTodosPayload,
} from "./session-runtime-payloads";

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
  workflow_id: string;
  workflow_step_id: string;
  title: string;
  description?: string;
  state?: TaskState;
  priority?: number;
  position?: number;
  repository_id?: string;
  primary_session_id?: string | null;
  session_count?: number | null;
  review_status?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  archived_at?: string | null;
  updated_at?: string;
  is_ephemeral: boolean;
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

export type SystemErrorPayload = {
  message: string;
  code?: string;
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
  agent_profile_id?: string;
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
};

export type TaskSessionStateChangedPayload = {
  task_id: string;
  session_id: string;
  old_state?: string;
  new_state?: string;
  agent_profile_id?: string;
  agent_profile_snapshot?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  is_passthrough?: boolean;
  error_message?: string;
  /** When true, the frontend should not show an error toast for this state change. */
  suppress_toast?: boolean;
  // Workflow-related fields (sent during workflow transitions)
  review_status?: string;
  // Task environment (for session→environment mapping)
  task_environment_id?: string;
};

export type TaskSessionWaitingForInputPayload = {
  task_id: string;
  session_id: string;
  title: string;
  body: string;
};

export type TaskSessionAgentctlPayload = {
  task_id: string;
  session_id: string;
  agent_execution_id?: string;
  error_message?: string;
  worktree_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
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

export type UserSettingsUpdatedPayload = {
  user_id: string;
  workspace_id: string;
  kanban_view_mode?: string;
  workflow_filter_id?: string;
  repository_ids: string[];
  initial_setup_complete?: boolean;
  preferred_shell?: string;
  default_editor_id?: string;
  enable_preview_on_click?: boolean;
  chat_submit_key?: string;
  review_auto_mark_on_scroll?: boolean;
  show_release_notification?: boolean;
  release_notes_last_seen_version?: string;
  lsp_auto_start_languages?: string[];
  lsp_auto_install_languages?: string[];
  saved_layouts?: SavedLayout[];
  default_utility_agent_id?: string;
  keyboard_shortcuts?: Record<string, { key: string; modifiers?: Record<string, boolean> }>;
  terminal_link_behavior?: string;
  updated_at?: string;
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
  type SessionTodosPayload,
} from "./session-runtime-payloads";

export type TaskPlanEventPayload = {
  id: string;
  task_id: string;
  title: string;
  content: string;
  created_by: "agent" | "user";
  created_at: string;
  updated_at: string;
};

export type QueuedMessagePayload = {
  content: string;
  model?: string;
  plan_mode?: boolean;
  task_id: string;
  user_id?: string;
  queued_at: string;
};

export type QueueStatusChangedPayload = {
  session_id: string;
  is_queued: boolean;
  message?: QueuedMessagePayload | null;
};

export type BackendMessageMap = {
  "kanban.update": BackendMessage<"kanban.update", KanbanUpdatePayload>;
  "task.created": BackendMessage<"task.created", TaskEventPayload>;
  "task.updated": BackendMessage<"task.updated", TaskEventPayload>;
  "task.deleted": BackendMessage<"task.deleted", TaskEventPayload>;
  "task.state_changed": BackendMessage<"task.state_changed", TaskEventPayload>;
  "task.plan.created": BackendMessage<"task.plan.created", TaskPlanEventPayload>;
  "task.plan.updated": BackendMessage<"task.plan.updated", TaskPlanEventPayload>;
  "task.plan.deleted": BackendMessage<"task.plan.deleted", TaskPlanEventPayload>;
  "agent.updated": BackendMessage<"agent.updated", AgentUpdatePayload>;
  "agent.available.updated": BackendMessage<
    "agent.available.updated",
    AgentAvailableUpdatedPayload
  >;
  "terminal.output": BackendMessage<"terminal.output", TerminalOutputPayload>;
  "diff.update": BackendMessage<"diff.update", DiffUpdatePayload>;
  "session.git.event": BackendMessage<"session.git.event", GitEventPayload>;
  "system.error": BackendMessage<"system.error", SystemErrorPayload>;
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
  "session.state_changed": BackendMessage<"session.state_changed", TaskSessionStateChangedPayload>;
  "session.waiting_for_input": BackendMessage<
    "session.waiting_for_input",
    TaskSessionWaitingForInputPayload
  >;
  "session.agentctl_starting": BackendMessage<
    "session.agentctl_starting",
    TaskSessionAgentctlPayload
  >;
  "session.agentctl_ready": BackendMessage<"session.agentctl_ready", TaskSessionAgentctlPayload>;
  "session.agentctl_error": BackendMessage<"session.agentctl_error", TaskSessionAgentctlPayload>;
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
  "executor.prepare.progress": BackendMessage<"executor.prepare.progress", PrepareProgressPayload>;
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
};

// Workspace file types (extracted to reduce file size)
export {
  type FileTreeNode,
  type FileTreeResponse,
  type FileContentResponse,
  type FileSearchResponse,
  type FileChangeEvent,
  type FileChangeNotificationPayload,
  type OpenFileTab,
  FILE_EXTENSION_COLORS,
} from "./workspace-files";
