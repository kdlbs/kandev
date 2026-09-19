import type { ForegroundActivity, TaskPendingAction, TaskSessionState } from "./http";
import type { TaskLaunchRecoveryAction } from "./task-launch-error";

export type AgentErrorCause = {
  operation?: string;
  code?: string;
  detail?: string;
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
  /** Automatic session launch waiting for admission, independent of the selected session. */
  launch_queue?: TaskStatusSummaryLaunchQueue | null;
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
    aggregate_state?: string;
    state?: string;
    number?: number;
    url?: string;
  } | null;
};
