import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AssociateAzureDevOpsPullRequestRequest,
  AzureDevOpsConfig,
  AzureDevOpsBoardReference,
  AzureDevOpsBoardSnapshot,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsBoardWorkItemUpdate,
  AzureDevOpsProject,
  AzureDevOpsPullRequestFeedback,
  AzureDevOpsPullRequestPage,
  AzureDevOpsRepository,
  AzureDevOpsTeam,
  AzureDevOpsSavedView,
  AzureDevOpsTaskPullRequest,
  AzureDevOpsTaskWorkItem,
  AzureDevOpsWorkItem,
  AzureDevOpsWorkItemAssignmentUpdate,
  AzureDevOpsWorkItemCommentPage,
  AzureDevOpsWorkItemDetail,
  AzureDevOpsWorkItemWatch,
  AzureDevOpsWorkItemWatchInput,
  AzureDevOpsPullRequestWatch,
  AzureDevOpsPullRequestWatchInput,
  AzureDevOpsWatchResetResult,
  AzureDevOpsWorkItemSearchResult,
  AzureDevOpsWorkspaceSettings,
  AssociateAzureDevOpsWorkItemRequest,
  SetAzureDevOpsConfigRequest,
  TestAzureDevOpsConnectionResult,
  UpdateAzureDevOpsWorkspaceSettingsRequest,
} from "@/lib/types/azure-devops";
import { invalidateIntegrationAvailabilityAfter } from "@/lib/integrations/integration-availability-events";

/* eslint-disable max-params */

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
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<AzureDevOpsConfig>(withWorkspace(`${BASE}/config`, workspaceId), {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    }),
  );
}

export function deleteAzureDevOpsConfig(workspaceId: string, options?: ApiRequestOptions) {
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<{ deleted: boolean }>(withWorkspace(`${BASE}/config`, workspaceId), {
      ...options,
      init: { ...options?.init, method: "DELETE" },
    }),
  );
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
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<AzureDevOpsConfig>(withWorkspace(`${BASE}/config/copy`, sourceWorkspaceId), {
      ...options,
      init: {
        ...options?.init,
        method: "POST",
        body: JSON.stringify({ targetWorkspaceId }),
      },
    }),
  );
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

export function listAzureDevOpsBranches(
  workspaceId: string,
  organization: string,
  project: string,
  repository: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ organization, project, repository });
  appendWorkspace(search, workspaceId);
  return fetchJson<{ branches: Array<{ name: string }> }>(`${BASE}/branches?${search}`, options);
}

export function getAzureDevOpsSavedViews(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ views: AzureDevOpsSavedView[] }>(
    withWorkspace(`${BASE}/views`, workspaceId),
    options,
  );
}

export function setAzureDevOpsSavedViews(
  workspaceId: string,
  views: AzureDevOpsSavedView[],
  options?: ApiRequestOptions,
) {
  return fetchJson<{ views: AzureDevOpsSavedView[] }>(withWorkspace(`${BASE}/views`, workspaceId), {
    ...options,
    init: { ...options?.init, method: "PUT", body: JSON.stringify({ views }) },
  });
}

export function getAzureDevOpsWorkspaceSettings(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<AzureDevOpsWorkspaceSettings>(
    withWorkspace(`${BASE}/workspace-settings`, workspaceId),
    options,
  );
}

export function updateAzureDevOpsWorkspaceSettings(
  workspaceId: string,
  payload: UpdateAzureDevOpsWorkspaceSettingsRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsWorkspaceSettings>(
    withWorkspace(`${BASE}/workspace-settings`, workspaceId),
    {
      ...options,
      init: { ...options?.init, method: "PATCH", body: JSON.stringify(payload) },
    },
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

export function getAzureDevOpsWorkItemDetail(
  workspaceId: string,
  project: string,
  id: number,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project });
  appendWorkspace(search, workspaceId);
  return fetchJson<AzureDevOpsWorkItemDetail>(`${BASE}/work-items/${id}?${search}`, options);
}

export function listAzureDevOpsWorkItemComments(
  workspaceId: string,
  project: string,
  id: number,
  continuationToken?: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project });
  appendWorkspace(search, workspaceId);
  if (continuationToken) search.set("continuation_token", continuationToken);
  return fetchJson<AzureDevOpsWorkItemCommentPage>(
    `${BASE}/work-items/${id}/comments?${search}`,
    options,
  );
}

export function updateAzureDevOpsWorkItemAssignment(
  workspaceId: string,
  project: string,
  id: number,
  payload: AzureDevOpsWorkItemAssignmentUpdate,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project });
  appendWorkspace(search, workspaceId);
  return fetchJson<AzureDevOpsWorkItem>(`${BASE}/work-items/${id}?${search}`, {
    ...options,
    init: { ...options?.init, method: "PATCH", body: JSON.stringify(payload) },
  });
}

export function listAzureDevOpsWorkItemWatches(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ watches: AzureDevOpsWorkItemWatch[] }>(
    withWorkspace(`${BASE}/watches/work-items`, workspaceId),
    options,
  );
}

export function createAzureDevOpsWorkItemWatch(
  workspaceId: string,
  payload: AzureDevOpsWorkItemWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsWorkItemWatch>(
    withWorkspace(`${BASE}/watches/work-items`, workspaceId),
    {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    },
  );
}

export function updateAzureDevOpsWorkItemWatch(
  workspaceId: string,
  id: string,
  payload: Partial<AzureDevOpsWorkItemWatchInput> & { enabled?: boolean },
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsWorkItemWatch>(
    withWorkspace(`${BASE}/watches/work-items/${encodeURIComponent(id)}`, workspaceId),
    {
      ...options,
      init: { ...options?.init, method: "PATCH", body: JSON.stringify(payload) },
    },
  );
}

export function deleteAzureDevOpsWorkItemWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(
    withWorkspace(`${BASE}/watches/work-items/${encodeURIComponent(id)}`, workspaceId),
    { ...options, init: { ...options?.init, method: "DELETE" } },
  );
}

export function triggerAzureDevOpsWorkItemWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ matched: number }>(
    withWorkspace(`${BASE}/watches/work-items/${encodeURIComponent(id)}/trigger`, workspaceId),
    { ...options, init: { ...options?.init, method: "POST" } },
  );
}

export function previewAzureDevOpsWorkItemWatchReset(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskCount: number }>(
    withWorkspace(
      `${BASE}/watches/work-items/${encodeURIComponent(id)}/reset/preview`,
      workspaceId,
    ),
    options,
  );
}

export function resetAzureDevOpsWorkItemWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsWatchResetResult>(
    withWorkspace(`${BASE}/watches/work-items/${encodeURIComponent(id)}/reset`, workspaceId),
    { ...options, init: { ...options?.init, method: "POST" } },
  );
}

export function listAzureDevOpsPullRequestWatches(
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ watches: AzureDevOpsPullRequestWatch[] }>(
    withWorkspace(`${BASE}/watches/pull-requests`, workspaceId),
    options,
  );
}

export function createAzureDevOpsPullRequestWatch(
  workspaceId: string,
  payload: AzureDevOpsPullRequestWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsPullRequestWatch>(
    withWorkspace(`${BASE}/watches/pull-requests`, workspaceId),
    { ...options, init: { ...options?.init, method: "POST", body: JSON.stringify(payload) } },
  );
}

export function updateAzureDevOpsPullRequestWatch(
  workspaceId: string,
  id: string,
  payload: Partial<AzureDevOpsPullRequestWatchInput> & { enabled?: boolean },
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsPullRequestWatch>(
    withWorkspace(`${BASE}/watches/pull-requests/${encodeURIComponent(id)}`, workspaceId),
    { ...options, init: { ...options?.init, method: "PATCH", body: JSON.stringify(payload) } },
  );
}

export function deleteAzureDevOpsPullRequestWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(
    withWorkspace(`${BASE}/watches/pull-requests/${encodeURIComponent(id)}`, workspaceId),
    { ...options, init: { ...options?.init, method: "DELETE" } },
  );
}

export function triggerAzureDevOpsPullRequestWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ matched: number }>(
    withWorkspace(`${BASE}/watches/pull-requests/${encodeURIComponent(id)}/trigger`, workspaceId),
    { ...options, init: { ...options?.init, method: "POST" } },
  );
}

export function previewAzureDevOpsPullRequestWatchReset(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskCount: number }>(
    withWorkspace(
      `${BASE}/watches/pull-requests/${encodeURIComponent(id)}/reset/preview`,
      workspaceId,
    ),
    options,
  );
}

export function resetAzureDevOpsPullRequestWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsWatchResetResult>(
    withWorkspace(`${BASE}/watches/pull-requests/${encodeURIComponent(id)}/reset`, workspaceId),
    { ...options, init: { ...options?.init, method: "POST" } },
  );
}

export function listAzureDevOpsTeams(
  workspaceId: string,
  project: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project });
  appendWorkspace(search, workspaceId);
  return fetchJson<{ teams: AzureDevOpsTeam[] }>(`${BASE}/teams?${search}`, options);
}

export function listAzureDevOpsBoards(
  workspaceId: string,
  project: string,
  team: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project, team });
  appendWorkspace(search, workspaceId);
  return fetchJson<{ boards: AzureDevOpsBoardReference[] }>(`${BASE}/boards?${search}`, options);
}

export function getAzureDevOpsBoardSnapshot(
  workspaceId: string,
  project: string,
  team: string,
  board: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project, team });
  appendWorkspace(search, workspaceId);
  return fetchJson<AzureDevOpsBoardSnapshot>(
    `${BASE}/boards/${encodeURIComponent(board)}?${search}`,
    options,
  );
}

export function updateAzureDevOpsBoardWorkItem(
  workspaceId: string,
  project: string,
  team: string,
  board: string,
  id: number,
  payload: AzureDevOpsBoardWorkItemUpdate,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ project, team });
  appendWorkspace(search, workspaceId);
  return fetchJson<AzureDevOpsBoardWorkItem>(
    `${BASE}/boards/${encodeURIComponent(board)}/work-items/${id}?${search}`,
    {
      ...options,
      init: { ...options?.init, method: "PATCH", body: JSON.stringify(payload) },
    },
  );
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

export function listWorkspaceAzureDevOpsTaskWorkItems(
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskWorkItems: Record<string, AzureDevOpsTaskWorkItem[]> }>(
    `${BASE}/workspaces/${encodeURIComponent(workspaceId)}/task-work-items`,
    options,
  );
}

export function associateAzureDevOpsWorkItem(
  workspaceId: string,
  taskId: string,
  payload: AssociateAzureDevOpsWorkItemRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<AzureDevOpsTaskWorkItem>(
    withWorkspace(`${BASE}/tasks/${encodeURIComponent(taskId)}/work-items`, workspaceId),
    {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    },
  );
}
