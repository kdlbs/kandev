import type { TaskDependencyRef } from "@/lib/state/slices/kanban/types";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import {
  type ForegroundActivity,
  type TaskPendingAction,
  type TaskPriority,
  type TaskState,
} from "@/lib/types/http";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";

export interface Task {
  id: string;
  title: string;
  workflowStepId: string;
  state?: TaskState;
  priority?: TaskPriority;
  description?: string;
  position?: number;
  repositoryId?: string;
  /** All repositories linked to the task; used to render a "+N" chip for multi-repo. */
  repositories?: Array<{
    id: string;
    repository_id: string;
    base_branch?: string;
    checkout_branch?: string;
    branch_policy_id?: string;
    branch_policy_name?: string;
    branch_policy_base_branch?: string;
    branch_policy_branch_template?: string;
    branch_policy_pull_request_target?: string;
    position: number;
  }>;
  sessionCount?: number | null;
  primarySessionId?: string | null;
  /**
   * Primary session's runtime state. Decoupled from `state` (the workflow
   * column). Used to suppress the running-spinner when the agent has already
   * finished — the workflow may leave the task in IN_PROGRESS for review.
   */
  primarySessionState?: string | null;
  primarySessionPendingAction?: TaskPendingAction | null;
  taskPendingAction?: TaskPendingAction | null;
  /**
   * Task-level MOST-ACTIVE-WINS activity aggregate;
   * undefined/null when no session is running. Drives the background-running
   * affordance on the card status icon.
   */
  foregroundActivity?: ForegroundActivity | null;
  /** True when the task's session was mid-turn when the backend died. */
  interrupted?: boolean;
  /** True when a workflow step's auto_start_agent on_enter action failed to
   *  launch a run for this task. */
  autoStartFailed?: boolean;
  /**
   * True when the task is waiting on the operator to notice, not on the
   * operator to act — a settled session with a positively-sampled
   * background process still live (spec:
   * docs/specs/disambiguate-waiting/spec.md). Outranked by pending-input
   * and any live foregroundActivity. The revision/epoch fields backing the
   * stale-update discard rule live on the store's KanbanState Task shape
   * (kanban/types.ts) and the wire payload, not here — this board-rendering
   * type only needs the resolved boolean.
   */
  parkedOnBackgroundWork?: boolean;
  /** True when this task inherits an archived parent's workspace and can no
   *  longer materialize or start. */
  workspaceOrphaned?: boolean;
  /** Live subagents summed across this task's sessions; drives the count chip. */
  activeSubagentCount?: number;
  reviewStatus?: "pending" | "approved" | "changes_requested" | "rejected" | null;
  primaryExecutorId?: string | null;
  primaryExecutorProfileId?: string | null;
  primaryExecutorType?: string | null;
  primaryExecutorName?: string | null;
  isRemoteExecutor?: boolean;
  /** Whether the executor profile can be switched right now. Never gap-filled on merge. */
  runnerEditable?: boolean;
  /** Machine-readable reason for `runnerEditable`. Never gap-filled on merge. */
  runnerIneligibleReason?: string;
  /** Human assignee (user id); the card renders their name read-only. */
  assigneeUserId?: string;
  parentTaskId?: string | null;
  workspaceMode?: "inherit_parent" | "new_workspace" | "shared_group";
  updatedAt?: string;
  createdAt?: string;
  wipAdmitted?: boolean;
  queuedForStepId?: string;
  queuedForStepTitle?: string;
  /** Derived dependency state — see TaskDependencyRef in the kanban slice. */
  blocked?: boolean;
  blockedReason?: string;
  dependsOn?: TaskDependencyRef[];
  blocks?: TaskDependencyRef[];
  startWhenUnblocked?: boolean;
  queuedAt?: string;
  issueUrl?: string;
  issueNumber?: number;
  statusSummary?: TaskStatusSummary | null;
}

export type RepositoryChip = {
  label: string;
  path?: string;
};

export interface WorkflowStep {
  id: string;
  title: string;
  color: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_start?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_exit?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
}

export type KanbanPresentation = PluginTaskMenuContext["presentation"];
