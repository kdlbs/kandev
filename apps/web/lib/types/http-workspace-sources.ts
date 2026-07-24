import type { RepositoryId, SessionId, TaskId } from "./ids";

export type TaskRepository = {
  id: string;
  task_id: TaskId;
  repository_id: RepositoryId;
  base_branch: string;
  /**
   * Optional branch to fetch and check out after worktree creation
   * (e.g. a PR head branch). Empty when no specific branch is requested.
   */
  checkout_branch?: string;
  position: number;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type WorkspaceFolder = {
  id: string;
  task_id: TaskId;
  local_path: string;
  display_name: string;
  position: number;
  created_at?: string;
  updated_at?: string;
};

export type WorkspaceRepositorySourceRequest = {
  kind: "repository";
  repository_id?: string;
  local_path?: string;
  remote_url?: string;
  provider?: "github" | "gitlab" | "azure_devops";
  provider_repo_id?: string;
  provider_owner?: string;
  provider_name?: string;
  base_branch: string;
  checkout_branch?: string;
};

export type WorkspaceFolderSourceRequest = {
  kind: "folder";
  local_path: string;
  display_name?: string;
};

export type WorkspaceSourceRequest =
  | WorkspaceRepositorySourceRequest
  | WorkspaceFolderSourceRequest;

export type AttachTaskWorkspaceSourcesRequest = { sources: WorkspaceSourceRequest[] };

export type AttachTaskWorkspaceSourcesResponse = {
  task_id: TaskId;
  repositories: TaskRepository[];
  workspace_folders: WorkspaceFolder[];
  workspace_path: string;
  /** Authoritative idle sessions whose runtime adopted the new sources. */
  adopted_session_ids?: SessionId[];
  session_ids: SessionId[];
};
