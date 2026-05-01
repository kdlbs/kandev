import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  ListWorkspacesResponse,
  ListRepositoriesResponse,
  RepositoryBranchesResponse,
  ListRepositoryScriptsResponse,
  Workspace,
} from "@/lib/types/http";

// Workspace operations
export async function createWorkspace(
  payload: { name: string; description?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<Workspace>("/api/v1/workspaces", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function listWorkspaces(options?: ApiRequestOptions) {
  return fetchJson<ListWorkspacesResponse>("/api/v1/workspaces", options);
}

// Repository operations
export async function listRepositories(
  workspaceId: string,
  params?: { includeScripts?: boolean },
  options?: ApiRequestOptions,
) {
  const searchParams = new URLSearchParams();
  if (params?.includeScripts) {
    searchParams.set("include_scripts", "true");
  }
  const queryString = searchParams.toString();
  const url = `/api/v1/workspaces/${workspaceId}/repositories${queryString ? `?${queryString}` : ""}`;
  return fetchJson<ListRepositoriesResponse>(url, options);
}

/**
 * Lists git branches for a workspace repo. Pass exactly one of `repositoryId`
 * (an imported workspace repo) or `path` (an on-machine folder discovered
 * but not yet imported). The backend resolves either to an absolute path and
 * runs the same `listGitBranches`. Used by the chip row's per-repo branch
 * picker which needs to handle both shapes.
 */
export async function listBranches(
  workspaceId: string,
  source: { repositoryId: string } | { path: string },
  options?: ApiRequestOptions,
) {
  const params = new URLSearchParams();
  if ("repositoryId" in source) params.set("repository_id", source.repositoryId);
  else params.set("path", source.path);
  return fetchJson<RepositoryBranchesResponse>(
    `/api/v1/workspaces/${workspaceId}/branches?${params.toString()}`,
    options,
  );
}

/**
 * Lists git branches for an imported workspace repository, scoped by
 * repository id only. Supports `refresh=true` to force a `git fetch` before
 * returning the list (with the backend's per-repo cooldown applied). Used by
 * single-repo flows that already have the repo id and want to drive the
 * stale-while-revalidate UI in the dialog.
 */
export async function listRepositoryBranches(
  repositoryId: string,
  params?: { refresh?: boolean },
  options?: ApiRequestOptions,
) {
  const qs = params?.refresh ? "?refresh=true" : "";
  return fetchJson<RepositoryBranchesResponse>(
    `/api/v1/repositories/${repositoryId}/branches${qs}`,
    options,
  );
}

export async function listRepositoryScripts(repositoryId: string, options?: ApiRequestOptions) {
  return fetchJson<ListRepositoryScriptsResponse>(
    `/api/v1/repositories/${repositoryId}/scripts`,
    options,
  );
}

// Quick Chat operations
export type StartQuickChatRequest = {
  title?: string;
  repository_id?: string;
  agent_profile_id?: string;
  executor_id?: string;
  prompt?: string;
  local_path?: string;
  repository_name?: string;
  default_branch?: string;
  base_branch?: string;
};

export type StartQuickChatResponse = {
  task_id: string;
  session_id: string;
};

export async function startQuickChat(
  workspaceId: string,
  payload: StartQuickChatRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<StartQuickChatResponse>(`/api/v1/workspaces/${workspaceId}/quick-chat`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function listQuickChatSessions(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{
    tasks: Array<{
      id: string;
      title: string;
      workspace_id: string;
      primary_session_id?: string | null;
      metadata?: Record<string, unknown> | null;
    }>;
  }>(`/api/v1/workspaces/${workspaceId}/tasks?only_ephemeral=true&exclude_config=true`, options);
}

// Config Chat operations
export type StartConfigChatRequest = {
  agent_profile_id?: string;
  executor_id?: string;
  prompt?: string;
};

export type StartConfigChatResponse = {
  task_id: string;
  session_id: string;
};

export async function startConfigChat(
  workspaceId: string,
  payload: StartConfigChatRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<StartConfigChatResponse>(`/api/v1/workspaces/${workspaceId}/config-chat`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}
