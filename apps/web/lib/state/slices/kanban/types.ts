import type { TaskPendingAction, TaskState as TaskStatus } from "@/lib/types/http";

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
    workflowStepId: string;
    title: string;
    description?: string;
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
      position: number;
    }>;
    primarySessionId?: string | null;
    primarySessionState?: string | null;
    primarySessionPendingAction?: TaskPendingAction | null;
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
    isPRReview?: boolean;
    isIssueWatch?: boolean;
    issueUrl?: string;
    issueNumber?: number;
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

export type WorkflowsState = {
  items: Array<{
    id: string;
    workspaceId: string;
    name: string;
    description?: string | null;
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
};

export type KanbanSliceState = {
  kanban: KanbanState;
  kanbanMulti: KanbanMultiState;
  workflows: WorkflowsState;
  tasks: TaskState;
};

export type KanbanSliceActions = {
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
  setWorkflowSnapshot: (workflowId: string, data: WorkflowSnapshotData) => void;
  setKanbanMultiLoading: (loading: boolean) => void;
  clearKanbanMulti: () => void;
  updateMultiTask: (workflowId: string, task: KanbanState["tasks"][number]) => void;
  removeMultiTask: (workflowId: string, taskId: string) => void;
};

export type KanbanSlice = KanbanSliceState & KanbanSliceActions;
