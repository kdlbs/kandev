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

export async function listRepositoryBranches(repositoryId: string, options?: ApiRequestOptions) {
  return fetchJson<RepositoryBranchesResponse>(
    `/api/v1/repositories/${repositoryId}/branches`,
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
