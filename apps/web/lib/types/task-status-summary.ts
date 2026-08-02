import type { ForegroundActivity, TaskPendingAction, TaskSessionState } from "./http";

export type TaskStatusSummary = {
  revision: number;
  updated_at: string;
  primary_session?: {
    id: string;
    state: TaskSessionState;
  } | null;
  foreground_activity?: ForegroundActivity;
  active_subagent_count?: number;
  pending_action?: TaskPendingAction;
  active_error?: {
    session_id: string;
    stamp: string;
    occurred_at: string;
    preview: string;
  } | null;
  git?: {
    additions?: number;
    deletions?: number;
    changed_files?: number;
    ahead?: number;
    behind?: number;
  } | null;
  pull_request?: {
    count?: number;
    open_count?: number;
    attention?: boolean;
    aggregate_state?: string;
    state?: string;
    number?: number;
    url?: string;
  } | null;
};
