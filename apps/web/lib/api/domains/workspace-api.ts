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
