import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  CreateLinearIssueWatchInput,
  LinearConfig,
  LinearIssue,
  LinearIssueWatch,
  LinearSearchFilter,
  LinearSearchResult,
  LinearTeam,
  LinearWorkflowState,
  SetLinearConfigRequest,
  TestLinearConnectionResult,
  UpdateLinearIssueWatchInput,
} from "@/lib/types/linear";

// getLinearConfig returns null when the backend responds 204 (no config yet).
export async function getLinearConfig(options?: ApiRequestOptions): Promise<LinearConfig | null> {
  const res = await fetchJson<LinearConfig | undefined>(`/api/v1/linear/config`, options);
  return res ?? null;
}

export async function setLinearConfig(
  payload: SetLinearConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<LinearConfig>(`/api/v1/linear/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deleteLinearConfig(options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/linear/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export async function testLinearConnection(
  payload: SetLinearConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TestLinearConnectionResult>(`/api/v1/linear/config/test`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function listLinearTeams(options?: ApiRequestOptions) {
  return fetchJson<{ teams: LinearTeam[] }>(`/api/v1/linear/teams`, options);
}

export async function listLinearStates(teamKey: string, options?: ApiRequestOptions) {
  return fetchJson<{ states: LinearWorkflowState[] }>(
    `/api/v1/linear/states?team_key=${encodeURIComponent(teamKey)}`,
    options,
  );
}

export async function getLinearIssue(identifier: string, options?: ApiRequestOptions) {
  return fetchJson<LinearIssue>(`/api/v1/linear/issues/${encodeURIComponent(identifier)}`, options);
}

export async function searchLinearIssues(
  params: LinearSearchFilter & { pageToken?: string; maxResults?: number },
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams();
  if (params.query) search.set("query", params.query);
  if (params.teamKey) search.set("team_key", params.teamKey);
  if (params.stateIds?.length) search.set("state_ids", params.stateIds.join(","));
  if (params.assigned) search.set("assigned", params.assigned);
  if (params.pageToken) search.set("page_token", params.pageToken);
  if (params.maxResults) search.set("max_results", String(params.maxResults));
  const qs = search.toString();
  return fetchJson<LinearSearchResult>(`/api/v1/linear/issues${qs ? `?${qs}` : ""}`, options);
}

export async function setLinearIssueState(
  issueID: string,
  stateID: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ transitioned: boolean }>(
    `/api/v1/linear/issues/${encodeURIComponent(issueID)}/state`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        body: JSON.stringify({ stateId: stateID }),
      },
    },
  );
}

// --- Issue watches ---

// listLinearIssueWatches fetches watches across all workspaces when
// workspaceId is omitted, or scoped to one workspace when provided.
export async function listLinearIssueWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const path = workspaceId
    ? `/api/v1/linear/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`
    : `/api/v1/linear/watches/issue`;
  const res = await fetchJson<{ watches: LinearIssueWatch[] }>(path, options);
  return res.watches ?? [];
}

export async function createLinearIssueWatch(
  payload: CreateLinearIssueWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<LinearIssueWatch>(`/api/v1/linear/watches/issue`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

// All mutation/trigger endpoints require `workspace_id` so the backend can
// reject cross-workspace IDOR.

export async function updateLinearIssueWatch(
  workspaceId: string,
  id: string,
  payload: UpdateLinearIssueWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<LinearIssueWatch>(
    `/api/v1/linear/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "PATCH", body: JSON.stringify(payload) },
    },
  );
}

export async function deleteLinearIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/linear/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "DELETE" } },
  );
}

export async function triggerLinearIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ newIssues: number }>(
    `/api/v1/linear/watches/issue/${encodeURIComponent(id)}/trigger?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}
