import type { ForegroundActivity, TaskPendingAction, TaskSessionState } from "./http";
import type { TaskLaunchRecoveryAction } from "./task-launch-error";

export type TaskStatusSummaryActiveError = {
  session_id?: string;
  task_repository_id?: string;
  stamp: string;
  occurred_at: string;
  preview: string;
  category?: string;
  recovery_actions?: TaskLaunchRecoveryAction[];
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
  foreground_activity?: ForegroundActivity;
  active_subagent_count?: number;
  pending_action?: TaskPendingAction;
  /** Number of prompts currently en-queued for the task (all sessions). */
  queued_prompt_count?: number;
  active_error?: TaskStatusSummaryActiveError | null;
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
    aggregate_state?: string;
    state?: string;
    number?: number;
    url?: string;
  } | null;
};
