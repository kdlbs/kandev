import type { ForegroundActivity, TaskPendingAction, TaskSessionState } from "./http";
import type { TaskLaunchRecoveryAction } from "./task-launch-error";

export type AgentErrorCause = {
  operation?: string;
  code?: string;
  detail?: string;
  reason?: string;
  requested_model?: string;
  effective_model?: string;
  attempted_model?: string;
  requested_mode?: string;
  effective_mode?: string;
  prompt_not_sent?: boolean;
};

export type TaskStatusSummaryActiveError = {
  scope?: "session" | "task";
  session_id?: string;
  task_repository_id?: string;
  stamp: string;
  occurred_at: string;
  preview: string;
  details?: string;
  category?: string;
  execution_id?: string;
  phase?: string;
  attempt_id?: string;
  causes?: AgentErrorCause[];
  recovery_actions?: TaskLaunchRecoveryAction[];
};

export type TaskStatusSummaryLaunchQueue = {
  session_id?: string;
  agent_profile_id?: string;
  workflow_step_id?: string;
  queued_at: string;
  reason: "session_capacity" | "ownership_unavailable" | "replay_error";
  retrying: boolean;
  capacity?: {
    in_use: number;
    limit: number;
    observed_at: string;
  };
};

export type TaskStatusSummaryCompletionGate = {
  revision: number;
  criteria_count: number;
  verified_count: number;
  blocker_count: number;
  blocked: boolean;
};

export type TaskStatusSummary = {
  revision: number;
  updated_at: string;
  /** Semantic task activity, separate from summary projection freshness. */
  last_activity_at?: string;
  primary_session?: {
    id: string;
    state: TaskSessionState;
  } | null;
  /** Task-wide RUNNING evidence. Undefined identifies a legacy summary. */
  has_running_session?: boolean;
  foreground_activity?: ForegroundActivity;
  active_subagent_count?: number;
  pending_action?: TaskPendingAction;
  /** Number of prompts currently en-queued for the task (all sessions). */
  queued_prompt_count?: number;
  /** Automatic session launch waiting for admission, independent of the selected session. */
  launch_queue?: TaskStatusSummaryLaunchQueue | null;
  completion_gate?: TaskStatusSummaryCompletionGate | null;
  active_error?: TaskStatusSummaryActiveError | null;
  /** Current task-owned failure, independent of the selected session. */
  task_error?: TaskStatusSummaryActiveError | null;
  git?: {
    additions?: number;
    deletions?: number;
    changed_files?: number;
    ahead?: number;
    behind?: number;
    comparison_unavailable?: boolean;
  } | null;
  pull_request?: {
    count?: number;
    open_count?: number;
    attention?: boolean;
    auto_fix_enabled?: boolean;
    auto_merge_enabled?: boolean;
    has_merge_conflicts?: boolean;
    workflow_approval_required?: boolean;
    workflow_approval_stale?: boolean;
    workflow_approval_pr_number?: number;
    workflow_approval_repository?: string;
    merge_conflict_pr_number?: number;
    merge_conflict_repository?: string;
    aggregate_state?: string;
    state?: string;
    number?: number;
    url?: string;
  } | null;
};
