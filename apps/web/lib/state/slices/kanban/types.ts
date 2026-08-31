import type {
  ForegroundActivity,
  TaskPendingAction,
  TaskPriority,
  TaskState as TaskStatus,
} from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";

export type KanbanStepEvents = {
  on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_exit?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_comment?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_blocker_resolved?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_children_completed?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_approval_resolved?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_heartbeat?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_budget_alert?: Array<{ type: string; config?: Record<string, unknown> }>;
  on_agent_error?: Array<{ type: string; config?: Record<string, unknown> }>;
};

/**
 * One end of a dependency edge. Carries title and state so the dependency chip
 * and blocked badge render without fetching each related task.
 *
 * `status` is the resolution verdict and is only meaningful on `dependsOn`
 * entries: "resolved" (finished successfully), "failed" (FAILED/CANCELLED —
 * halts the chain), or "pending" (anything else, including archived).
 */
export type TaskDependencyRef = {
  id: string;
  title?: string;
  state?: TaskStatus;
  status?: "resolved" | "failed" | "pending";
};

export type KanbanState = {
  workflowId: string | null;
  steps: Array<{
    id: string;
    title: string;
    color: string;
    position: number;
    events?: KanbanStepEvents;
    allow_manual_move?: boolean;
    prompt?: string;
    is_start_step?: boolean;
    show_in_command_panel?: boolean;
    agent_profile_id?: string;
    /** Maximum concurrent tasks allowed in this step. 0 or undefined means unlimited. */
    wip_limit?: number;
    /** Optional upstream step used by automation to pull more work. */
    pull_from_step_id?: string | null;
    /**
     * Phase 2 (ADR-0004) semantic UX hint. Read by `<TaskMetaRail>` to
     * pick the right meta surface (review/approval shows multi-agent
     * decisions). Backend never branches on this field.
     */
    stage_type?: "work" | "review" | "approval" | "custom";
  }>;
  tasks: Array<{
    id: string;
    workspaceId?: string;
    // Required for workflow-backed kanban tasks: every producer (kanban.update
    // WS handler, snapshotToState, toKanbanTask) must populate it, or the
    // prevent-auto-start gate would resolve a task's step list against the
    // wrong workflow. Ephemeral tasks are filtered out before this point.
    workflowId: string;
    workflowStepId: string;
    title: string;
    description?: string;
    autopilot?: boolean;
    priority?: TaskPriority;
    position: number;
    state?: TaskStatus;
    /** Primary repository id (lowest position). Kept for backwards compat. */
    repositoryId?: string;
    /**
     * All repositories linked to the task, ordered by Position. Optional so
     * legacy SSR payloads still parse; multi-repo UI consumers should prefer
     * this over repositoryId.
     */
    repositories?: Array<{
      id: string;
      repository_id: string;
      base_branch: string;
      checkout_branch?: string;
      branch_policy_id?: string;
      branch_policy_name?: string;
      branch_policy_base_branch?: string;
      branch_policy_branch_template?: string;
      branch_policy_pull_request_target?: string;
      position: number;
    }>;
    workspaceFolders?: Array<{
      id: string;
      local_path: string;
      display_name: string;
      position: number;
    }>;
    primarySessionId?: string | null;
    primarySessionState?: string | null;
    primarySessionPendingAction?: TaskPendingAction | null;
    taskPendingAction?: TaskPendingAction | null;
    /**
     * Task-level MOST-ACTIVE-WINS activity aggregate;
     * undefined/null when no session is running. Drives the board card and task
     * list background-running affordance.
     */
    foregroundActivity?: ForegroundActivity | null;
    /** True when the task's session was mid-turn when the backend died. */
    interrupted?: boolean;
    /** True when a workflow step's auto_start_agent on_enter action failed to
     *  launch a run for this task. */
    autoStartFailed?: boolean;
    /** Live subagents across this task's sessions; drives the board count chip. */
    activeSubagentCount?: number;
    sessionCount?: number | null;
    reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
    primaryExecutorId?: string | null;
    primaryExecutorType?: string | null;
    primaryExecutorName?: string | null;
    isRemoteExecutor?: boolean;
    parentTaskId?: string | null;
    workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
    updatedAt?: string;
    createdAt?: string;
    wipAdmitted?: boolean;
    queuedForStepId?: string;
    queuedAt?: string;
    /**
     * Task dependencies. Derived on the backend per read (never stored), so
     * these arrive fresh on every boot payload and task.updated event.
     */
    blocked?: boolean;
    /** "pending" | "failed" | "unknown"; absent when not blocked. */
    blockedReason?: string;
    /** Direct predecessors — the tasks this one is waiting on. Not transitive. */
    dependsOn?: TaskDependencyRef[];
    /** Direct dependents — the tasks waiting on this one. Not transitive. */
    blocks?: TaskDependencyRef[];
    /** A launch intent is waiting on dependency resolution. */
    startWhenUnblocked?: boolean;
    isPRReview?: boolean;
    isIssueWatch?: boolean;
    metadata?: Record<string, unknown> | null;
    isArchived?: boolean;
    issueUrl?: string;
    issueNumber?: number;
    statusSummary?: TaskStatusSummary | null;
  }>;
  isLoading?: boolean;
};

export type WorkflowSnapshotData = {
  workflowId: string;
  workflowName: string;
  steps: KanbanState["steps"];
  tasks: KanbanState["tasks"];
  isPlaceholder?: boolean;
};

export type KanbanMultiState = {
  snapshots: Record<string, WorkflowSnapshotData>;
  isLoading: boolean;
};

export type SidebarArchivedTasksState = {
  itemsByWorkspaceId: Record<string, KanbanState["tasks"]>;
  loadedByWorkspaceId: Record<string, boolean>;
  loadingByWorkspaceId: Record<string, boolean>;
  errorByWorkspaceId: Record<string, string | null>;
  revisionByWorkspaceId: Record<string, number>;
};

export type WorkflowsState = {
  items: Array<{
    id: string;
    workspaceId: string;
    name: string;
    description?: string | null;
    prompt?: string;
    sortOrder?: number;
    agent_profile_id?: string;
    hidden?: boolean;
    /**
     * Phase 2 (ADR-0004) UX hint. Read by `<TaskMetaRail>` to choose the
     * right meta surface (kanban / office / multi-agent). Backend never
     * branches on this field.
     */
    style?: "kanban" | "office" | "custom";
  }>;
  activeId: string | null;
};

export type TaskState = {
  activeTaskId: string | null;
  activeSessionId: string | null;
  // pinnedSessionId tracks the session the USER explicitly selected.
  // Set by setActiveSession (user-initiated). Cleared when navigating to a
  // different task or when an automatic handoff explicitly takes over. WS
  // auto-adopt paths must not override a non-terminal pin for the active task.
  pinnedSessionId: string | null;
  // lastSessionByTaskId remembers the most-recent active session for each task.
  // Unlike pinnedSessionId (single global slot, cleared on task change), this
  // map survives task switches so navigating back to a task can restore the
  // user's last-selected session instead of always jumping to primary.
  lastSessionByTaskId: Record<string, string>;
  // resumeSkippedSessionIds records sessions whose open-time auto-resume was
  // skipped because the prevent-auto-start-on-open preference is enabled. A
  // Record (not a Set): the slice is Immer-managed and SSR-hydrated, so a
  // native Set would break mutation and serialization. The Start agent button
  // renders for these sessions until the agent confirms RUNNING.
  resumeSkippedSessionIds: Record<string, true>;
};

export type KanbanSliceState = {
  kanban: KanbanState;
  kanbanMulti: KanbanMultiState;
  sidebarArchivedTasks: SidebarArchivedTasksState;
  workflows: WorkflowsState;
  workspaceContextGeneration: number;
  tasks: TaskState;
};

export type KanbanSliceActions = {
  resetKanbanWorkspaceContext: () => void;
  setActiveWorkflow: (workflowId: string | null) => void;
  setWorkflows: (workflows: WorkflowsState["items"]) => void;
  reorderWorkflowItems: (workflowIds: string[]) => void;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  // setActiveSessionAuto updates the active session without creating or
  // clearing a user pin. Callers that intentionally override a pin must clear
  // it explicitly after checking no non-terminal manual pin should be preserved.
  setActiveSessionAuto: (taskId: string, sessionId: string) => void;
  clearActiveSession: () => void;
  // setResumeSkipped records/clears the resume-skipped marker for a session.
  // Recording is guarded at the call site (the session-resumption hook reads
  // the live session row with typed store access), so a stale status response
  // can never leave a Start button while the agent is actually running.
  setResumeSkipped: (sessionId: string, skipped: boolean) => void;
  setWorkflowSnapshot: (workflowId: string, data: WorkflowSnapshotData) => void;
  setKanbanMultiLoading: (loading: boolean) => void;
  clearKanbanMulti: () => void;
  updateMultiTask: (workflowId: string, task: KanbanState["tasks"][number]) => void;
  removeMultiTask: (workflowId: string, taskId: string) => void;
  setSidebarArchivedTasks: (
    workspaceId: string,
    tasks: KanbanState["tasks"],
    expectedRevision?: number,
  ) => boolean;
  setSidebarArchivedTasksLoading: (workspaceId: string, loading: boolean) => void;
  setSidebarArchivedTasksError: (workspaceId: string, error: string | null) => void;
  upsertSidebarArchivedTask: (workspaceId: string, task: KanbanState["tasks"][number]) => void;
  removeSidebarArchivedTask: (taskId: string, workspaceId?: string) => void;
};

export type KanbanSlice = KanbanSliceState & KanbanSliceActions;
