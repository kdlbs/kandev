import type { FileInfo } from '@/lib/state/slices/session-runtime/types';

// Base payload with discriminator
type GitEventBase = {
  session_id: string;
  task_id?: string;
  agent_id?: string;
  timestamp: string;
};

// Git status data
export type GitStatusData = {
  branch: string;
  remote_branch: string | null;
  head_commit?: string;
  base_commit?: string;
  modified: string[];
  added: string[];
  deleted: string[];
  untracked: string[];
  renamed: string[];
  ahead: number;
  behind: number;
  files: Record<string, FileInfo>;
};

// Git commit data
export type GitCommitData = {
  id: string;
  commit_sha: string;
  parent_sha: string;
  commit_message: string;
  author_name: string;
  author_email: string;
  files_changed: number;
  insertions: number;
  deletions: number;
  committed_at: string;
  created_at?: string;
};

// Git reset data
export type GitResetData = {
  previous_head: string;
  current_head: string;
  deleted_count: number;
};

// Git snapshot data
export type GitSnapshotData = {
  id: string;
  session_id: string;
  snapshot_type: string;
  branch: string;
  remote_branch: string;
  head_commit: string;
  base_commit: string;
  ahead: number;
  behind: number;
  files: Record<string, FileInfo>;
  triggered_by: string;
  created_at: string;
};

// Individual event variants
export type GitStatusUpdateEvent = GitEventBase & {
  type: 'status_update';
  status: GitStatusData;
};

export type GitCommitCreatedEvent = GitEventBase & {
  type: 'commit_created';
  commit: GitCommitData;
};

export type GitCommitsResetEvent = GitEventBase & {
  type: 'commits_reset';
  reset: GitResetData;
};

export type GitSnapshotCreatedEvent = GitEventBase & {
  type: 'snapshot_created';
  snapshot: GitSnapshotData;
};

// Discriminated union
export type GitEventPayload =
  | GitStatusUpdateEvent
  | GitCommitCreatedEvent
  | GitCommitsResetEvent
  | GitSnapshotCreatedEvent;
