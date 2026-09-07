import type { BackendMessage } from "./backend-message";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { FileChangeNotificationPayload } from "./workspace-files";
import type { ForegroundActivity } from "./activity";
import type { TaskPendingAction, TaskPendingActionRevision } from "./http";
import type {
  AgentCapabilitiesPayload,
  SessionInfoPayload,
  SessionModelsPayload,
  SessionModelSelectionWarningPayload,
  SessionMCPStatusPayload,
  SessionPromptUsagePayload,
  SessionTodosPayload,
} from "./session-runtime-payloads";

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
  /** 1-based ordinal among ALL user messages of the session; present only on
   * user messages from an indexed server payload. */
  prompt_index?: number;
  /** Authoritative per-session input projection after this semantic message mutation. */
  pending_action?: TaskPendingAction | null;
  /** Logical clock shared with REST session snapshots. */
  pending_action_revision?: TaskPendingActionRevision;
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
  execution_profile_id?: string;
  route_generation?: number;
  route_state?: string;
  route_reason?: string;
  route_error_code?: string;
  route_error_class?: "transient" | "hard" | "unclassified" | string;
  route_catalogue_version?: string;
  route_retry_ordinal?: number;
  route_deadline?: string;
  route_pending_outcome?: "skip" | "stop" | string;
  downstream_acp_session_id?: string;
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

export type SessionPendingActionChangedPayload = {
  workspace_id: string;
  task_id: string;
  session_id: string;
  pending_action: TaskPendingAction | null;
  pending_action_revision: TaskPendingActionRevision;
};

export type TaskSessionNotificationPayload = {
  task_id: string;
  session_id: string;
  occurrence_id: string;
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

export type TurnEventPayload = {
  id: string;
  session_id: string;
  task_id: string;
  started_at: string;
  completed_at?: string;
  execution_profile_id?: string;
  route_generation?: number;
  metadata?: Record<string, unknown>;
  /** Whether the completed turn produced any agent output. Only set on turn.completed. */
  had_output?: boolean;
  created_at: string;
  updated_at: string;
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

export type ShellOutputPayload = {
  task_id: string;
  session_id: string;
  type: "output" | "exit";
  data?: string;
  code?: number;
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

export type QueueStatusChangedPayload = {
  session_id: string;
  entries?: QueuedMessage[] | null;
  count?: number;
  max?: number;
  merge_enabled?: boolean;
  auto_run?: boolean;
};

export type AvailableCommandPayload = {
  name: string;
  description?: string;
  input_hint?: string;
};

export type SessionBackendMessageMap = {
  "session.message.added": BackendMessage<"session.message.added", MessageAddedPayload>;
  "session.message.updated": BackendMessage<"session.message.updated", MessageAddedPayload>;
  "session.message.deleted": BackendMessage<"session.message.deleted", MessageAddedPayload>;
  "session.state_changed": BackendMessage<"session.state_changed", TaskSessionStateChangedPayload>;
  "session.turn_finished": BackendMessage<"session.turn_finished", TaskSessionNotificationPayload>;
  "session.activity_changed": BackendMessage<
    "session.activity_changed",
    TaskSessionActivityChangedPayload
  >;
  "session.cancellation_changed": BackendMessage<
    "session.cancellation_changed",
    TaskSessionCancellationChangedPayload
  >;
  "session.pending_action_changed": BackendMessage<
    "session.pending_action_changed",
    SessionPendingActionChangedPayload
  >;
  "session.clarification_requested": BackendMessage<
    "session.clarification_requested",
    TaskSessionNotificationPayload
  >;
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
  "session.model_fallback": BackendMessage<
    "session.model_fallback",
    {
      task_id: string;
      session_id: string;
      agent_id: string;
      fallback_model: string;
      timestamp: string;
    }
  >;
  "session.model_selection_warning": BackendMessage<
    "session.model_selection_warning",
    SessionModelSelectionWarningPayload
  >;
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
  "session.workspace.file.changes": BackendMessage<
    "session.workspace.file.changes",
    FileChangeNotificationPayload
  >;
  "session.shell.output": BackendMessage<"session.shell.output", ShellOutputPayload>;
  "session.process.output": BackendMessage<"session.process.output", ProcessOutputPayload>;
  "session.process.status": BackendMessage<"session.process.status", ProcessStatusPayload>;
  "message.queue.status_changed": BackendMessage<
    "message.queue.status_changed",
    QueueStatusChangedPayload
  >;
};
