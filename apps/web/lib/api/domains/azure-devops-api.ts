import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AssociateAzureDevOpsPullRequestRequest,
  AzureDevOpsConfig,
  AzureDevOpsProject,
  AzureDevOpsPullRequestFeedback,
  AzureDevOpsPullRequestPage,
  AzureDevOpsRepository,
  AzureDevOpsTaskPullRequest,
  AzureDevOpsWorkItem,
  AzureDevOpsWorkItemSearchResult,
  SetAzureDevOpsConfigRequest,
  TestAzureDevOpsConnectionResult,
} from "@/lib/types/azure-devops";

const BASE = "/api/v1/azure-devops";

function withWorkspace(path: string, workspaceId: string): string {
  const search = new URLSearchParams();
  search.set("workspace_id", workspaceId);
  return `${path}${path.includes("?") ? "&" : "?"}${search}`;
}

function appendWorkspace(search: URLSearchParams, workspaceId: string): void {
  search.set("workspace_id", workspaceId);
}

export async function getAzureDevOpsConfig(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsConfig | null> {
  const result = await fetchJson<AzureDevOpsConfig | undefined>(
    withWorkspace(`${BASE}/config`, workspaceId),
    options,
  );
  return result ?? null;
}

export function setAzureDevOpsConfig(
  workspaceId: string,
  payload: SetAzureDevOpsConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsConfig>(withWorkspace(`${BASE}/config`, workspaceId), {
    ...options,
    init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
  });
}

export function deleteAzureDevOpsConfig(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(withWorkspace(`${BASE}/config`, workspaceId), {
    ...options,
    init: { ...options?.init, method: "DELETE" },
  });
}

export function testAzureDevOpsConnection(
  workspaceId: string,
  payload: SetAzureDevOpsConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TestAzureDevOpsConnectionResult>(
    withWorkspace(`${BASE}/config/test`, workspaceId),
    {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    },
  );
}

export function copyAzureDevOpsConfig(
  sourceWorkspaceId: string,
  targetWorkspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsConfig>(withWorkspace(`${BASE}/config/copy`, sourceWorkspaceId), {
    ...options,
    init: {
      ...options?.init,
      method: "POST",
      body: JSON.stringify({ targetWorkspaceId }),
    },
  });
}

export function listAzureDevOpsProjects(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ projects: AzureDevOpsProject[] }>(
    withWorkspace(`${BASE}/projects`, workspaceId),
    options,
  );
}

export function listAzureDevOpsRepositories(
  workspaceId: string,
  project: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project });
  appendWorkspace(search, workspaceId);
  return fetchJson<{ repositories: AzureDevOpsRepository[] }>(
    `${BASE}/repositories?${search}`,
    options,
  );
}

export function searchAzureDevOpsWorkItems(
  workspaceId: string,
  payload: { project: string; wiql: string; top?: number },
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsWorkItemSearchResult>(
    withWorkspace(`${BASE}/work-items/search`, workspaceId),
    {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    },
  );
}

export function getAzureDevOpsWorkItem(
  workspaceId: string,
  project: string,
  id: number,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project });
  appendWorkspace(search, workspaceId);
  return fetchJson<AzureDevOpsWorkItem>(`${BASE}/work-items/${id}?${search}`, options);
}

export type AzureDevOpsPullRequestFilters = {
  project: string;
  repository: string;
  status?: string;
  creator?: string;
  reviewer?: string;
  sourceBranch?: string;
  targetBranch?: string;
  skip?: number;
  top?: number;
};

export function listAzureDevOpsPullRequests(
  workspaceId: string,
  filters: AzureDevOpsPullRequestFilters,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({
    project: filters.project,
    repository: filters.repository,
  });
  if (filters.status) search.set("status", filters.status);
  if (filters.creator) search.set("creator", filters.creator);
  if (filters.reviewer) search.set("reviewer", filters.reviewer);
  if (filters.sourceBranch) search.set("source_branch", filters.sourceBranch);
  if (filters.targetBranch) search.set("target_branch", filters.targetBranch);
  if (filters.skip !== undefined) search.set("skip", String(filters.skip));
  if (filters.top !== undefined) search.set("top", String(filters.top));
  appendWorkspace(search, workspaceId);
  return fetchJson<AzureDevOpsPullRequestPage>(`${BASE}/pull-requests?${search}`, options);
}

function pullRequestPath(projectId: string, repositoryId: string, pullRequestId: number): string {
  return `${BASE}/pull-requests/${encodeURIComponent(projectId)}/${encodeURIComponent(repositoryId)}/${pullRequestId}`;
}

export function getAzureDevOpsPullRequest(
  workspaceId: string,
  projectId: string,
  repositoryId: string,
  pullRequestId: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsPullRequestFeedback["pullRequest"]>(
    withWorkspace(pullRequestPath(projectId, repositoryId, pullRequestId), workspaceId),
    options,
  );
}

export function getAzureDevOpsPullRequestFeedback(
  workspaceId: string,
  projectId: string,
  repositoryId: string,
  pullRequestId: number,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsPullRequestFeedback>(
    withWorkspace(
      `${pullRequestPath(projectId, repositoryId, pullRequestId)}/feedback`,
      workspaceId,
    ),
    options,
  );
}

export function listWorkspaceAzureDevOpsTaskPullRequests(
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskPrs: Record<string, AzureDevOpsTaskPullRequest[]> }>(
    `${BASE}/workspaces/${encodeURIComponent(workspaceId)}/task-prs`,
    options,
  );
}

function taskPullRequestMutation(
  action: "associate" | "sync",
  workspaceId: string,
  taskId: string,
  payload: AssociateAzureDevOpsPullRequestRequest,
  options?: ApiRequestOptions,
) {
  const suffix = action === "sync" ? "/sync" : "";
  return fetchJson<AzureDevOpsTaskPullRequest>(
    withWorkspace(
      `${BASE}/tasks/${encodeURIComponent(taskId)}/pull-requests${suffix}`,
      workspaceId,
    ),
    {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    },
  );
}

export function associateAzureDevOpsPullRequest(
  workspaceId: string,
  taskId: string,
  payload: AssociateAzureDevOpsPullRequestRequest,
  options?: ApiRequestOptions,
) {
  return taskPullRequestMutation("associate", workspaceId, taskId, payload, options);
}

export function syncAzureDevOpsTaskPullRequest(
  workspaceId: string,
  taskId: string,
  payload: AssociateAzureDevOpsPullRequestRequest,
  options?: ApiRequestOptions,
) {
  return taskPullRequestMutation("sync", workspaceId, taskId, payload, options);
}
