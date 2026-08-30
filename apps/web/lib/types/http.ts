/* eslint-disable max-lines -- HTTP DTO definitions intentionally co-locate protocol shapes. */

import type { ExecutorType } from "./executor";
import type { ActiveSubagentCountFields, ForegroundActivity } from "./activity";
import type { UserSettings } from "./http-user-settings";
import type {
  RepositoryBranchPolicy,
  TaskRepository,
  WorkspaceFolder,
} from "./http-workspace-sources";
import type {
  AgentProfileId,
  RepositoryId,
  SessionId,
  TaskId,
  WorkflowId,
  WorkspaceId,
} from "./ids";
import type { OnEnterActionType, StepEvents } from "./workflow-actions";
import type { EntityReference } from "./entity-reference";
import type { TaskStatusSummary } from "./task-status-summary";

export type { TaskStatusSummary } from "./task-status-summary";

export type { ExecutorType } from "./executor";
export type { ActiveSubagentCountFields, ForegroundActivity } from "./activity";
export type {
  SavedLayout,
  SidebarViewApi,
  SidebarViewDraftApi,
  SidebarTaskPrefsApi,
  TaskCreateLastUsedApi,
  AppStatusBarOrderApi,
  LspStatusLocation,
  LastSeenDisplay,
  MCPTaskAgentProfileDefault,
  StartupPage,
  UserSettings,
  UserSettingsResponse,
  UserSettingsUpdatePayload,
} from "./http-user-settings";
export type {
  AgentProfileRecentUseApiRecord,
  AgentProfileRecentUseContext,
} from "./http-agent-profile-recent-use";
export type {
  AttachTaskWorkspaceSourcesRequest,
  AttachTaskWorkspaceSourcesResponse,
  RepositoryBranchPolicy,
  TaskRepository,
  WorkspaceFolder,
  WorkspaceFolderSourceRequest,
  WorkspaceRepositorySourceRequest,
  WorkspaceSourceRequest,
} from "./http-workspace-sources";
export * from "./ids";
export type {
  MoveToStepConfig,
  OnEnterAction,
  OnEnterActionType,
  ConfigureSessionActionConfig,
  ConfigureSessionOperation,
  ConfigureSessionRule,
  OnExitAction,
  OnExitActionType,
  OnTurnCompleteAction,
  OnTurnCompleteActionType,
  OnTurnStartAction,
  OnTurnStartActionType,
  StepEvents,
  TransitionConfig,
} from "./workflow-actions";

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

export type TaskPriority = "critical" | "high" | "medium" | "low";

// Workflow Review Status
export type WorkflowReviewStatus = "pending" | "approved" | "changes_requested" | "rejected";

// Reasons the backend tags on an auto-deleted task.deleted event.
export type TaskDeletionReason = "pr_approved_by_user" | "pr_merged_or_closed" | "issue_closed";

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
  id?: string;
  name: string;
  position: number;
  color?: string;
  prompt?: string;
  events?: StepEvents;
  is_start_step?: boolean;
  show_in_command_panel?: boolean;
  agent_profile_id?: AgentProfileId;
  execution_profile_id?: AgentProfileId;
  route_generation?: number;
  route_state?: string;
  route_reason?: string;
  downstream_acp_session_id?: string;
  auto_advance_requires_signal?: boolean;
  cancel_triggers_turn_complete?: boolean;
  wip_limit?: number;
  pull_from_step_id?: string | null;
};

// Workflow Step - instance of a step on a workflow
export type WorkflowStep = {
  id: string;
  workflow_id: WorkflowId;
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: StepEvents;
  allow_manual_move?: boolean;
  is_start_step?: boolean;
  show_in_command_panel?: boolean;
  auto_archive_after_hours?: number;
  agent_profile_id?: string;
  wip_limit?: number;
  pull_from_step_id?: string | null;
  /**
   * Phase 2 (ADR-0004) semantic UX hint. Backend code does not branch on this;
   * frontend uses it to choose presentation (review/approval styling, etc).
   */
  stage_type?: "work" | "review" | "approval" | "custom";
  /**
   * ADR 0015: gate on_turn_complete transitions on an explicit
   * `step_complete_kandev` MCP signal from the agent. When true, the
   * step's auto-advance only fires once the agent (or the manual
   * fallback button) signals completion. Default false preserves
   * legacy "any turn-end advances" behaviour.
   */
  auto_advance_requires_signal?: boolean;
  /**
   * When true, an explicit user cancellation runs this step's normal
   * on_turn_complete actions after the cancelled turn settles.
   */
  cancel_triggers_turn_complete?: boolean;
  created_at: string;
  updated_at: string;
};

// Session Step History - audit trail
export type SessionStepHistory = {
  id: string;
  session_id: SessionId;
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
  | "IDLE"
  | "WAITING_FOR_INPUT"
  | "COMPLETED"
  | "FAILED"
  | "CANCELLED";

export type TaskPendingAction = "clarification" | "permission";

export type TaskPendingActionRevision = {
  epoch: string;
  sequence: number;
};

/**
 * Fine-grained busy substate of a session (see ADR-0049). Distinguishes
 * a foreground turn that is actively generating from one that is idle, held open
 * only by spawned background work (a subagent task, a run-in-background shell, an
 * active Monitor). `generating` is meaningful while `state === "RUNNING"`;
 * `background` may outlive the foreground turn. Delivered live over the
 * `session.activity_changed` WS event and carried on `session.state_changed`;
 * absent/`null` is treated as "generating" for a RUNNING session.
 */
export type Workflow = {
  id: WorkflowId;
  workspace_id: WorkspaceId;
  name: string;
  description?: string | null;
  /** Optional workflow-level agent instructions prepended at every step entry. */
  prompt?: string;
  workflow_template_id?: string | null;
  agent_profile_id?: AgentProfileId;
  sort_order?: number;
  hidden?: boolean;
  /**
   * Phase 2 (ADR-0004) UX hint. Frontend uses this to pick a presentation
   * shell (kanban board, office task pane, etc). Backend does NOT branch on it.
   */
  style?: "kanban" | "office" | "custom";
  /**
   * Workflow provenance. `"github"` marks workflows synced from a configured
   * GitHub repo (see workflow sync); omitted/`"manual"` for user-created
   * workflows. `source_path` is the repo-relative file the workflow was
   * synced from and is omitted for manual workflows.
   */
  source?: string;
  source_path?: string;
  created_at: string;
  updated_at: string;
};

export type Workspace = {
  id: WorkspaceId;
  name: string;
  description?: string | null;
  owner_id: string;
  default_executor_id?: string | null;
  default_environment_id?: string | null;
  default_agent_profile_id?: AgentProfileId | null;
  default_config_agent_profile_id?: AgentProfileId | null;
  office_workflow_id?: WorkflowId;
  created_at: string;
  updated_at: string;
};

export type Repository = {
  id: RepositoryId;
  workspace_id: WorkspaceId;
  name: string;
  source_type: string;
  local_path: string;
  provider: string;
  provider_repo_id: string;
  provider_host?: string;
  provider_scope?: string;
  provider_owner: string;
  provider_name: string;
  /** Canonical credential-free clone URL for provider-backed repositories. */
  remote_url?: string;
  default_branch: string;
  scripts?: RepositoryScript[];
  worktree_branch_prefix: string;
  worktree_branch_template?: string;
  pull_before_worktree: boolean;
  setup_script: string;
  cleanup_script: string;
  dev_script: string;
  /**
   * Comma-separated gitignored files/globs seeded into each new worktree.
   * Append `:symlink` to an entry (e.g. `.env.local:symlink`) to link it back
   * to the main repo instead of copying it; `::symlink` escapes a literal
   * suffix. Remote executors always copy the bytes.
   */
  copy_files: string;
  secret_bindings?: RepositorySecretBinding[];
  created_at: string;
  updated_at: string;
};

export type RepositorySecretBinding = {
  key: string;
  secret_id: string;
};

/**
 * A named, reusable group of workspace repositories. Applying one fills the
 * task-creation repository picker in a single action.
 *
 * A set deliberately carries no branch: branch choice belongs to the task, and
 * the picker's existing per-row defaulting fills it after a set is applied.
 */
export type RepositorySet = {
  id: string;
  workspace_id: WorkspaceId;
  name: string;
  description: string;
  /** Membership in apply order. Always an array, never null. */
  repositories: RepositorySetItem[];
  created_at: string;
  updated_at: string;
};

export type RepositorySetItem = {
  repository_id: RepositoryId;
  position: number;
};

export type RepositoryScript = {
  id: string;
  repository_id: RepositoryId;
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
  session_id: SessionId;
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

/**
 * Returns the primary task repository (lowest Position, first by created_at on
 * tie). Returns undefined for repo-less tasks. Consumers that historically
 * picked `task.repositories?.[0]` should call this to get position-aware
 * selection consistent with the backend.
 */
export function primaryTaskRepository(
  repos: TaskRepository[] | undefined,
): TaskRepository | undefined {
  if (!repos || repos.length === 0) return undefined;
  let primary = repos[0];
  for (const r of repos) {
    if (r.position < primary.position) primary = r;
  }
  return primary;
}

export type Task = ActiveSubagentCountFields & {
  id: TaskId;
  workspace_id: WorkspaceId;
  workflow_id: WorkflowId;
  workflow_step_id: string;
  position: number;
  title: string;
  description: string;
  /** True when the task was created in autopilot mode. Immutable after creation. */
  autopilot?: boolean;
  state: TaskState;
  priority: TaskPriority;
  wip_admitted?: boolean;
  queued_for_step_id?: string;
  queued_at?: string | null;
  repositories?: TaskRepository[];
  workspace_folders?: WorkspaceFolder[];
  primary_session_id?: SessionId | null;
  primary_session_state?: TaskSessionState | null;
  primary_session_pending_action?: TaskPendingAction | null;
  task_pending_action?: TaskPendingAction | null;
  /** True when the task's session was mid-turn when the backend died and has
   *  not been resumed since (startup reconciliation marker). */
  interrupted?: boolean;
  /** True when a workflow step's auto_start_agent on_enter action failed to
   *  launch a run for this task. */
  auto_start_failed?: boolean;
  /**
   * Task-level MOST-ACTIVE-WINS activity across sessions. "generating" wins,
   * then "background"; null/absent means none is known. The count is the
   * corresponding sum of live subagents.
   */
  foreground_activity?: ForegroundActivity | null;
  session_count?: number | null;
  review_status?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primary_executor_id?: string | null;
  primary_executor_type?: ExecutorType | null;
  primary_executor_name?: string | null;
  primary_agent_name?: string | null;
  primary_working_directory?: string | null;
  is_remote_executor?: boolean;
  is_ephemeral?: boolean;
  parent_id?: TaskId;
  archived_at?: string | null;
  created_at: string;
  updated_at: string;
  metadata?: Record<string, unknown> | null;
  // Office extensions (mirror TaskDTO Go fields). Empty/undefined for kanban-origin tasks.
  origin?: TaskOrigin;
  project_id?: string;
  // Backend-computed "owned by office" flag: true when project_id is set
  // OR workflow_id matches the workspace's office_workflow_id. See
  // isFromOfficeProjection in the Go task repo for the canonical rule.
  is_from_office?: boolean;
  status_summary?: TaskStatusSummary | null;
  /** Explicitly clears a cached status summary. Omission keeps partial-response semantics. */
  status_summary_invalidated?: boolean;
};

// Task origin values mirror models.TaskOrigin* constants in the Go backend.
export type TaskOrigin =
  | "manual"
  | "agent_created"
  | "routine"
  | "onboarding"
  | "automation_run"
  | "automation_task";

// isFromOffice reads the backend-computed flag (predicate lives in SQL at
// apps/backend/internal/task/repository/sqlite/task.go). Use to gate
// office-only UI like the "Open in office view" topbar link.
export const isFromOffice = (task: Task | null | undefined): boolean => !!task?.is_from_office;

export type CreateTaskResponse = Task & {
  session_id?: string;
  agent_execution_id?: string;
  agent_profile_id?: AgentProfileId;
};

// Backend workflow step DTO (flat fields, as returned from API)
export type WorkflowStepDTO = {
  id: string;
  workflow_id: WorkflowId;
  name: string;
  position: number;
  color: string;
  prompt?: string;
  events?: StepEvents;
  allow_manual_move: boolean;
  is_start_step?: boolean;
  show_in_command_panel?: boolean;
  auto_archive_after_hours?: number;
  agent_profile_id?: AgentProfileId;
  stage_type?: "work" | "review" | "approval" | "custom";
  wip_limit?: number;
  pull_from_step_id?: string | null;
  created_at?: string;
  updated_at?: string;
};

// Response from moving a task - includes workflow step info for automation
export type MoveTaskResponse = {
  task: Task;
  workflow_step: WorkflowStepDTO;
};

/** A worktree associated with a task session (one per repo on multi-repo tasks). */
export type TaskSessionWorktree = {
  /** Session-worktree association ID. */
  id: string;
  session_id: SessionId;
  worktree_id: string;
  repository_id?: RepositoryId;
  branch_slug?: string;
  position: number;
  worktree_path?: string;
  worktree_branch?: string;
  created_at?: string;
};

export type TaskSession = ActiveSubagentCountFields & {
  id: SessionId;
  task_id: TaskId;
  /** Optional user-supplied label shown on the session tab. */
  name?: string;
  agent_profile_id?: AgentProfileId;
  /** Logical profile selected by the user; dynamic profiles resolve this to a concrete launch profile. */
  execution_profile_id?: AgentProfileId;
  /** Monotonic dynamic-route generation used for stale action rejection. */
  route_generation?: number;
  /** Durable dynamic-route state, such as starting, waiting, or action_required. */
  route_state?: string;
  /** Stable reason code for the current dynamic-route state. */
  route_reason?: string;
  /** Classified provider cause currently driving route recovery. */
  route_error_code?: string;
  route_error_class?: "transient" | "hard" | "unclassified" | string;
  route_catalogue_version?: string;
  route_retry_ordinal?: number;
  route_deadline?: string;
  route_pending_outcome?: "skip" | "stop" | string;
  /** Downstream ACP session ID for the currently selected concrete candidate. */
  downstream_acp_session_id?: string;
  container_id?: string;
  executor_id?: string;
  environment_id?: string;
  repository_id?: RepositoryId;
  base_branch?: string;
  base_commit_sha?: string;
  worktree_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  /** Effective task root containing every attached workspace source. */
  workspace_path?: string;
  worktrees?: TaskSessionWorktree[];
  task_environment_id?: string;
  state: TaskSessionState;
  /** Backend-owned runtime cancellation projection; API responses include it explicitly. */
  cancellation_pending?: boolean;
  /** Process-local cancellation transition generation used to reject stale snapshots. */
  cancellation_revision?: number;
  /** Fine-grained busy substate; background may outlive the foreground turn (ADR-0049). */
  foreground_activity?: ForegroundActivity | null;
  /**
   * True when a send right now would be delivered into the still-generating turn
   * (mid-turn steering) rather than blocked/queued. Live, derived from the
   * connected agent's negotiated capability plus the runtime flag; never
   * persisted. The composer uses it to promise delivery, not folding.
   */
  supports_steering?: boolean;
  /** Compact pending-input projection used when this session's messages are unloaded. */
  pending_action?: TaskPendingAction | null;
  /** Cross-channel logical clock for pending_action snapshots. */
  pending_action_revision?: TaskPendingActionRevision;
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
  review_status?: WorkflowReviewStatus;
  // Server-resolved tool_call count, populated by ListTaskSessions.
  command_count?: number;
  // Slack-style read cursor: the id of the newest message the frontend has
  // marked as read. Used to position the unread ("New") divider.
  last_read_message_id?: string;
};

export type {
  TaskSessionsResponse,
  TaskSessionResponse,
  MarkSessionReadResponse,
  ApproveSessionResponse,
} from "./http-session-responses";

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

export type ListRepositorySetsResponse = {
  repository_sets: RepositorySet[];
  total: number;
};

export type ListRepositoryBranchPoliciesResponse = {
  repository_branch_policies: RepositoryBranchPolicy[];
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
  /** @deprecated Compatibility field; manual validity is determined by `exists` and `is_git`. */
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
  current_branch?: string;
  // RFC3339 timestamp of the most recent `git fetch` for this repository,
  // when refresh was requested. Empty if no refresh has been performed.
  fetched_at?: string;
  // Human-readable error from the last fetch attempt for this request, if
  // one was attempted and failed. Empty otherwise.
  fetch_error?: string;
};

export type LocalRepositoryStatusResponse = {
  current_branch: string;
  dirty_files: string[];
};

export type ListWorkspacesResponse = {
  workspaces: Workspace[];
  total: number;
};

export type Executor = {
  id: string;
  name: string;
  type: ExecutorType;
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
  executor_type?: ExecutorType;
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
  | "script_execution"
  | "agent_plan";

export type MessageMetadata = Record<string, unknown> & {
  entity_references?: EntityReference[];
};

export type Message = {
  id: string;
  session_id: SessionId;
  task_id: TaskId;
  turn_id?: string;
  author_type: MessageAuthorType;
  author_id?: string;
  content: string;
  raw_content?: string;
  type: MessageType;
  metadata?: MessageMetadata;
  requests_input?: boolean;
  created_at: string;
  /** Authoritative per-message change signal; advances on every content/metadata update. */
  updated_at?: string;
  /** 1-based ordinal among ALL user messages of the session (ordered by
   * created_at ascending, ties by id); present only on user messages from an
   * indexed server payload, omitted on older payloads. */
  prompt_index?: number;
};

export type Turn = {
  id: string;
  session_id: SessionId;
  task_id: TaskId;
  started_at: string;
  completed_at?: string;
  execution_profile_id?: AgentProfileId;
  route_generation?: number;
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
  prompt?: string;
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
  wip_limit?: number;
  pull_from_step_position?: number;
};

export type ImportWorkflowsResult = { created: string[]; skipped: string[] };

// Helper function to check if a step has a specific on_enter action
export function stepHasOnEnterAction(
  step: { events?: StepEvents },
  actionType: OnEnterActionType,
): boolean {
  return step.events?.on_enter?.some((a) => a.type === actionType) ?? false;
}
