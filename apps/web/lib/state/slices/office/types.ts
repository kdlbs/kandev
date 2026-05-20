// --- Office entity types ---
//
// Per ADR 0005 Wave E the canonical `AgentProfile`, `AgentRole`, `AgentStatus`,
// `BillingType`, `UtilizationWindow`, and `ProviderUsage` types live in
// `lib/types/agent-profile.ts`. Office consumers see profiles via the
// office API (which always populates orchestration fields), so this slice
// re-exports `OfficeAgentProfile` as `AgentProfile` for import-path
// compatibility — keeping office orchestration fields non-optional within
// the office subtree.

export type {
  AgentRole,
  AgentStatus,
  BillingType,
  UtilizationWindow,
  ProviderUsage,
} from "@/lib/types/agent-profile";

export type { OfficeAgentProfile as AgentProfile } from "@/lib/types/agent-profile";

import type { OfficeAgentProfile as AgentProfile } from "@/lib/types/agent-profile";

export type SkillSourceType =
  | "inline"
  | "local_path"
  | "git"
  | "skills_sh"
  | "user_home"
  // System skills shipped bundled in the kandev binary. Synced into
  // office_skills at backend start. Read-only in the UI: users cannot
  // edit or delete them — they get refreshed in place on every kandev
  // upgrade.
  | "system";

export type Skill = {
  id: string;
  workspaceId: string;
  name: string;
  slug: string;
  description?: string;
  sourceType: SkillSourceType;
  sourceLocator?: string;
  content?: string;
  fileInventory?: string[];
  createdByAgentProfileId?: string;
  // True iff this row was upserted by the kandev startup system-skill
  // sync. UI hides edit/delete affordances and surfaces a `System`
  // badge.
  isSystem?: boolean;
  // The kandev release version that wrote the current content. Shown
  // in the system-skill footer so users can correlate a SKILL.md edit
  // with the release that delivered it.
  systemVersion?: string;
  // Agent roles that auto-attach this skill at agent-create time.
  // Empty / undefined → no auto-attach (the user toggles it explicitly).
  defaultForRoles?: string[];
  createdAt: string;
  updatedAt: string;
};

export type ProjectStatus = "active" | "completed" | "on_hold" | "archived";

export type TaskCounts = {
  total: number;
  in_progress: number;
  done: number;
  blocked: number;
};

export type Project = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string;
  status: ProjectStatus;
  leadAgentProfileId?: string;
  color?: string;
  budgetCents?: number;
  repositories?: string[];
  executorConfig?: Record<string, unknown>;
  taskCounts?: TaskCounts;
  createdAt: string;
  updatedAt: string;
};

export type ApprovalType =
  | "hire_agent"
  | "budget_increase"
  | "board_approval"
  | "task_review"
  | "skill_creation";
export type ApprovalStatus = "pending" | "approved" | "rejected";

export type Approval = {
  id: string;
  workspaceId: string;
  type: ApprovalType;
  requestedByAgentProfileId?: string;
  status: ApprovalStatus;
  payload?: Record<string, unknown>;
  decisionNote?: string;
  decidedBy?: string;
  decidedAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type ActivityEntry = {
  id: string;
  workspaceId: string;
  actorType: "user" | "agent" | "system";
  actorId: string;
  action: string;
  targetType?: string;
  targetId?: string;
  details?: Record<string, unknown>;
  runId?: string;
  sessionId?: string;
  createdAt: string;
};

export type CostBreakdownItem = {
  group_key: string;
  group_label?: string;
  total_subcents: number;
  count: number;
};

export type CostSummary = {
  totalSubcents: number;
  byAgent: Array<{ agentProfileId: string; name: string; costSubcents: number }>;
  byProject: Array<{ projectId: string; name: string; costSubcents: number }>;
  byModel: Array<{ model: string; costSubcents: number; tokensIn: number; tokensOut: number }>;
};

export type BudgetPolicy = {
  id: string;
  workspaceId: string;
  scopeType: "agent" | "project" | "workspace";
  scopeId: string;
  limitSubcents: number;
  period: "monthly" | "total";
  alertThresholdPct: number;
  actionOnExceed: "notify_only" | "pause_agent" | "block_new_tasks";
  createdAt: string;
  updatedAt: string;
};

export type RoutineStatus = "active" | "paused" | "archived";

export type Routine = {
  id: string;
  workspaceId: string;
  name: string;
  description?: string;
  taskTemplate: Record<string, unknown>;
  assigneeAgentProfileId?: string;
  status: RoutineStatus;
  concurrencyPolicy: string;
  catchUpPolicy?: string;
  catchUpMax?: number;
  variables?: Record<string, unknown>;
  lastRunAt?: string;
  createdAt: string;
  updatedAt: string;
};

export type RoutineTriggerKind = "cron" | "webhook";
export type RoutineRunStatus =
  | "received"
  | "task_created"
  | "skipped"
  | "coalesced"
  | "failed"
  | "done"
  | "cancelled";

export type RoutineTrigger = {
  id: string;
  routineId: string;
  kind: RoutineTriggerKind;
  cronExpression?: string;
  timezone?: string;
  publicId?: string;
  signingMode?: string;
  nextRunAt?: string;
  lastFiredAt?: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type RoutineRun = {
  id: string;
  routineId: string;
  triggerId?: string;
  source: string;
  status: RoutineRunStatus;
  triggerPayload?: string;
  linkedTaskId?: string;
  coalescedIntoRunId?: string;
  dispatchFingerprint?: string;
  startedAt?: string;
  completedAt?: string;
  createdAt: string;
};

export type InboxItemType =
  | "approval"
  | "budget_alert"
  | "agent_error"
  | "agent_run_failed"
  | "agent_paused_after_failures"
  | "task_review"
  | "task_review_request"
  | "provider_degraded";

export type InboxItem = {
  id: string;
  type: InboxItemType;
  title: string;
  description?: string;
  status: string;
  entity_id?: string;
  entity_type?: string;
  createdAt: string;
  payload?: Record<string, unknown>;
};

export type Run = {
  id: string;
  agent_profile_id: string;
  reason: string;
  status: "queued" | "claimed" | "finished" | "failed" | "cancelled";
  cancel_reason?: string;
  /** Verbatim error payload set when status === "failed". */
  error_message?: string;
  requested_at: string;
};

export type OfficeTaskStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "blocked"
  | "done"
  | "cancelled";

export type OfficeTaskPriority = "critical" | "high" | "medium" | "low" | "none";

export type TaskLabel = {
  name: string;
  color: string;
};

export type OfficeTask = {
  id: string;
  workspaceId: string;
  identifier: string;
  title: string;
  description?: string;
  status: OfficeTaskStatus;
  priority: OfficeTaskPriority;
  parentId?: string;
  projectId?: string;
  assigneeAgentProfileId?: string;
  labels?: TaskLabel[] | string[];
  blockedBy?: string[];
  children?: OfficeTask[];
  executionPolicy?: string;
  executionState?: string;
  createdAt: string;
  updatedAt: string;
  // True when the task lives in a kandev-managed system workflow
  // (today: standing coordination; future: routine-fired). The Tasks
  // UI renders a "System" badge for these when the dev toggle reveals
  // them.
  isSystem?: boolean;
};

export type TaskFilterState = {
  statuses: OfficeTaskStatus[];
  priorities: OfficeTaskPriority[];
  assigneeIds: string[];
  projectIds: string[];
  search: string;
};

export type TaskSortField = "status" | "priority" | "title" | "created" | "updated";
export type TaskSortDir = "asc" | "desc";
export type TaskGroupBy = "status" | "priority" | "assignee" | "project" | "parent" | "none";
export type TaskViewMode = "list" | "board";

export type TasksState = {
  items: OfficeTask[];
  filters: TaskFilterState;
  viewMode: TaskViewMode;
  sortField: TaskSortField;
  sortDir: TaskSortDir;
  groupBy: TaskGroupBy;
  nestingEnabled: boolean;
  isLoading: boolean;
};

export type RunActivityDay = {
  date: string;
  succeeded: number;
  failed: number;
  other: number;
};

export type TaskBreakdown = {
  open: number;
  in_progress: number;
  blocked: number;
  done: number;
};

export type RecentTask = {
  id: string;
  identifier: string;
  title: string;
  status: string;
  assignee_agent_profile_id: string;
  updated_at: string;
};

export type LiveRun = {
  agentId: string;
  agentName: string;
  taskId: string;
  taskTitle: string;
  taskIdentifier: string;
  status: string;
  durationMs: number;
  startedAt: string;
  finishedAt?: string;
};

export type DashboardData = {
  agent_count: number;
  running_count: number;
  paused_count: number;
  error_count: number;
  tasks_in_progress: number;
  open_tasks: number;
  blocked_tasks: number;
  month_spend_subcents: number;
  pending_approvals: number;
  recent_activity: ActivityEntry[];
  task_count: number;
  skill_count: number;
  routine_count: number;
  run_activity: RunActivityDay[];
  task_breakdown: TaskBreakdown;
  recent_tasks: RecentTask[];
  /**
   * Per-agent card payload. Embedded in the dashboard response so the
   * dashboard renders in a single round-trip (Stream A + G of office
   * optimization). Always non-null; empty array for workspaces with no
   * agents. Type kept as `unknown[]` here to avoid importing from the
   * api domain — `AgentCardsPanel` narrows it via the api `AgentSummary`
   * type at the consumer.
   */
  agent_summaries: AgentSummary[];
};

/**
 * Slim per-agent card payload duplicated from `office-api` so the store
 * type doesn't depend on the api module. Mirrors the snake_case JSON shape
 * the backend serialises.
 */
export type AgentSummary = {
  agent_id: string;
  agent_name: string;
  agent_role: string;
  status: "live" | "finished" | "never_run";
  live_session?: SessionSummaryLite | null;
  last_session?: SessionSummaryLite | null;
  recent_sessions: SessionSummaryLite[];
  last_run_status?: "ok" | "failed";
  pause_reason?: string;
  consecutive_failures?: number;
};

export type SessionSummaryLite = {
  session_id: string;
  task_id: string;
  task_identifier: string;
  task_title: string;
  state: string;
  started_at: string;
  completed_at?: string | null;
  duration_seconds: number;
  command_count: number;
};

// --- Meta types (from backend /api/v1/office/meta) ---

export type StatusMeta = {
  id: string;
  label: string;
  order: number;
  color: string;
};

export type PriorityMeta = {
  id: string;
  label: string;
  order: number;
  color: string;
  value: number;
};

export type RoleMeta = {
  id: string;
  label: string;
  description: string;
  color: string;
};

export type ExecutorTypeMeta = {
  id: string;
  label: string;
  description: string;
};

export type SkillSourceTypeMeta = {
  id: string;
  label: string;
  readOnly: boolean;
  readOnlyReason?: string;
};

export type ProjectStatusMeta = {
  id: string;
  label: string;
  color: string;
};

export type AgentStatusMeta = {
  id: string;
  label: string;
  color: string;
};

export type RoutineRunStatusMeta = {
  id: string;
  label: string;
  color: string;
};

export type InboxItemTypeMeta = {
  id: string;
  label: string;
  icon: string;
};

export type PermissionMeta = {
  key: string;
  label: string;
  description: string;
  type: "bool" | "int";
};

export type OfficeMeta = {
  statuses: StatusMeta[];
  priorities: PriorityMeta[];
  roles: RoleMeta[];
  executorTypes: ExecutorTypeMeta[];
  skillSourceTypes: SkillSourceTypeMeta[];
  projectStatuses: ProjectStatusMeta[];
  agentStatuses: AgentStatusMeta[];
  routineRunStatuses: RoutineRunStatusMeta[];
  inboxItemTypes: InboxItemTypeMeta[];
  permissions: PermissionMeta[];
  permissionDefaults: Record<string, Record<string, unknown>>;
};

// --- Provider routing types ---

export type Tier = "frontier" | "balanced" | "economy";

export type RoutingErrorCode =
  | "auth_required"
  | "missing_credentials"
  | "subscription_required"
  | "quota_limited"
  | "rate_limited"
  | "provider_unavailable"
  | "model_unavailable"
  | "provider_not_configured"
  | "unknown_provider_error"
  | "agent_runtime_error"
  | "task_error"
  | "repo_error"
  | "permission_denied_by_user";

export type TierMap = {
  frontier?: string;
  balanced?: string;
  economy?: string;
};

export type ProviderProfile = {
  tier_map: TierMap;
  mode?: string;
  flags?: string[];
  env?: Record<string, string>;
};

// Wake reasons the workspace can map onto specific tiers. v1 keeps the
// surface narrow — only the three reasons that historically used the
// cheap-profile shortcut. Adding more requires both a backend constant
// and a UI copy update so the user gets context for each row.
export type WakeReason = "heartbeat" | "routine_trigger" | "budget_alert";

export type TierPerReason = Partial<Record<WakeReason, Tier>>;

export type WorkspaceRouting = {
  enabled: boolean;
  provider_order: string[];
  default_tier: Tier;
  provider_profiles: Record<string, ProviderProfile>;
  tier_per_reason?: TierPerReason;
};

export type AgentRoutingOverrides = {
  provider_order_source?: "inherit" | "override" | "";
  provider_order?: string[];
  tier_source?: "inherit" | "override" | "";
  tier?: Tier | "";
  tier_per_reason_source?: "inherit" | "override" | "";
  tier_per_reason?: TierPerReason;
};

export type ProviderHealthState = "healthy" | "degraded" | "user_action_required";
export type ProviderHealthScope = "provider" | "model" | "tier";

export type ProviderHealth = {
  workspace_id?: string;
  provider_id: string;
  scope: ProviderHealthScope;
  scope_value: string;
  state: ProviderHealthState;
  error_code?: RoutingErrorCode;
  retry_at?: string;
  backoff_step: number;
  last_failure?: string;
  last_success?: string;
  raw_excerpt?: string;
};

export type RouteAttemptOutcome =
  | "launched"
  | "failed_provider_unavailable"
  | "failed_other"
  | "skipped_degraded"
  | "skipped_user_action"
  | "skipped_missing_mapping"
  | "skipped_max_attempts";

export type RouteAttempt = {
  seq: number;
  provider_id: string;
  model?: string;
  tier: Tier | "";
  outcome: RouteAttemptOutcome;
  error_code?: RoutingErrorCode;
  error_confidence?: "high" | "medium" | "low";
  adapter_phase?: string;
  classifier_rule?: string;
  exit_code?: number;
  raw_excerpt?: string;
  reset_hint?: string;
  started_at: string;
  finished_at?: string;
};

export type ProviderModelPair = {
  provider_id: string;
  model: string;
  tier: Tier | "";
};

export type AgentRoutePreview = {
  agent_id: string;
  agent_name: string;
  tier_source: "inherit" | "override";
  effective_tier: Tier;
  // primary_* reflects configured intent — first entry in the
  // effective provider order, even when that provider is currently
  // skipped (degraded / missing mapping).
  primary_provider_id?: string;
  primary_model?: string;
  // current_* reflects the candidate the next launch would actually
  // pick; equal to primary when not degraded. Empty when every
  // candidate is skipped.
  current_provider_id?: string;
  current_model?: string;
  fallback_chain: ProviderModelPair[];
  missing: string[];
  degraded: boolean;
};

export type AgentRouteData = {
  preview: AgentRoutePreview;
  /**
   * Raw routing override blob persisted on the agent's settings. The
   * agent routing UI hydrates from this so toggles + tier override +
   * provider-order override reflect persisted values on first paint.
   */
  overrides: AgentRoutingOverrides;
  last_failure_code?: RoutingErrorCode;
  last_failure_run?: string;
};

export type RoutingState = {
  byWorkspace: Record<string, WorkspaceRouting | undefined>;
  knownProviders: string[];
  preview: { byWorkspace: Record<string, AgentRoutePreview[] | undefined> };
};

export type ProviderHealthSliceState = {
  byWorkspace: Record<string, ProviderHealth[]>;
};

export type RunAttemptsState = {
  byRunId: Record<string, RouteAttempt[]>;
};

export type AgentRoutingSliceState = {
  byAgentId: Record<string, AgentRouteData | undefined>;
};

// --- Slice state & actions ---

export type OfficeRefetchTrigger = {
  type: string;
  timestamp: number;
};

export type OfficeSliceState = {
  office: {
    agentProfiles: AgentProfile[];
    skills: Skill[];
    projects: Project[];
    approvals: Approval[];
    activity: ActivityEntry[];
    costSummary: CostSummary | null;
    budgetPolicies: BudgetPolicy[];
    routines: Routine[];
    inboxItems: InboxItem[];
    inboxCount: number;
    runs: Run[];
    dashboard: DashboardData | null;
    tasks: TasksState;
    meta: OfficeMeta | null;
    isLoading: boolean;
    refetchTrigger: OfficeRefetchTrigger | null;
    routing: RoutingState;
    providerHealth: ProviderHealthSliceState;
    runAttempts: RunAttemptsState;
    agentRouting: AgentRoutingSliceState;
  };
};

export type OfficeSliceActions = {
  setOfficeAgentProfiles: (agents: AgentProfile[]) => void;
  addOfficeAgentProfile: (agent: AgentProfile) => void;
  updateOfficeAgentProfile: (id: string, patch: Partial<AgentProfile>) => void;
  removeOfficeAgentProfile: (id: string) => void;
  setSkills: (skills: Skill[]) => void;
  addSkill: (skill: Skill) => void;
  updateSkill: (id: string, patch: Partial<Skill>) => void;
  removeSkill: (id: string) => void;
  setProjects: (projects: Project[]) => void;
  addProject: (project: Project) => void;
  updateProject: (id: string, patch: Partial<Project>) => void;
  removeProject: (id: string) => void;
  setApprovals: (approvals: Approval[]) => void;
  setActivity: (entries: ActivityEntry[]) => void;
  setCostSummary: (summary: CostSummary | null) => void;
  setBudgetPolicies: (policies: BudgetPolicy[]) => void;
  setRoutines: (routines: Routine[]) => void;
  setInboxItems: (items: InboxItem[]) => void;
  setInboxCount: (count: number) => void;
  setRuns: (runs: Run[]) => void;
  setDashboard: (data: DashboardData | null) => void;
  setTasks: (tasks: OfficeTask[]) => void;
  appendTasks: (tasks: OfficeTask[]) => void;
  patchTaskInStore: (taskId: string, patch: Partial<OfficeTask>) => void;
  setTaskFilters: (filters: Partial<TaskFilterState>) => void;
  setTaskViewMode: (mode: TaskViewMode) => void;
  setTaskSortField: (field: TaskSortField) => void;
  setTaskSortDir: (dir: TaskSortDir) => void;
  setTaskGroupBy: (groupBy: TaskGroupBy) => void;
  toggleNesting: () => void;
  setTasksLoading: (loading: boolean) => void;
  setMeta: (meta: OfficeMeta | null) => void;
  setOfficeLoading: (loading: boolean) => void;
  setOfficeRefetchTrigger: (type: string) => void;
  setWorkspaceRouting: (workspaceId: string, cfg: WorkspaceRouting | undefined) => void;
  setKnownProviders: (providers: string[]) => void;
  setRoutingPreview: (workspaceId: string, agents: AgentRoutePreview[]) => void;
  setProviderHealth: (workspaceId: string, health: ProviderHealth[]) => void;
  upsertProviderHealth: (workspaceId: string, row: ProviderHealth) => void;
  setRunAttempts: (runId: string, attempts: RouteAttempt[]) => void;
  appendRunAttempt: (runId: string, attempt: RouteAttempt) => void;
  setAgentRouting: (agentId: string, data: AgentRouteData | undefined) => void;
};

export type OfficeSlice = OfficeSliceState & OfficeSliceActions;
