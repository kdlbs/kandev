import type { FileInfo } from "@/lib/state/slices/session-runtime/types";

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
  branch_additions?: number;
  branch_deletions?: number;
  /**
   * Repository this status belongs to in multi-repo task workspaces.
   * Empty / undefined for single-repo workspaces. The frontend keys per-repo
   * git status off this name so the changes panel can show all repos at once.
   */
  repository_name?: string;
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
  /** Multi-repo: name of the repo this commit was made in. Empty for single-repo. */
  repository_name?: string;
};

// Git reset data
export type GitResetData = {
  previous_head: string;
  current_head: string;
  deleted_count: number;
};

// Git branch switch data
export type GitBranchSwitchData = {
  previous_branch: string;
  current_branch: string;
  current_head: string;
  base_commit: string;
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
  files?: Record<string, FileInfo>;
  triggered_by: string;
  created_at: string;
};

// Individual event variants
export type GitStatusUpdateEvent = GitEventBase & {
  type: "status_update";
  status: GitStatusData;
};

export type GitCommitCreatedEvent = GitEventBase & {
  type: "commit_created";
  commit: GitCommitData;
};

export type GitCommitsResetEvent = GitEventBase & {
  type: "commits_reset";
  reset: GitResetData;
};

export type GitBranchSwitchedEvent = GitEventBase & {
  type: "branch_switched";
  branch_switch: GitBranchSwitchData;
};

export type GitSnapshotCreatedEvent = GitEventBase & {
  type: "snapshot_created";
  snapshot: GitSnapshotData;
};

// Discriminated union
export type GitEventPayload =
  | GitStatusUpdateEvent
  | GitCommitCreatedEvent
  | GitCommitsResetEvent
  | GitBranchSwitchedEvent
  | GitSnapshotCreatedEvent;
