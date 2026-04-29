import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  LinearConfig,
  LinearIssue,
  LinearSearchFilter,
  LinearSearchResult,
  LinearTeam,
  LinearWorkflowState,
  SetLinearConfigRequest,
  TestLinearConnectionResult,
} from "@/lib/types/linear";

// getLinearConfig returns null when the backend responds 204 (no config yet).
export async function getLinearConfig(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<LinearConfig | null> {
  const res = await fetchJson<LinearConfig | undefined>(
    `/api/v1/linear/config?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
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

export async function deleteLinearConfig(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/linear/config?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "DELETE" } },
  );
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

export async function listLinearTeams(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ teams: LinearTeam[] }>(
    `/api/v1/linear/teams?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function listLinearStates(
  workspaceId: string,
  teamKey: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ states: LinearWorkflowState[] }>(
    `/api/v1/linear/states?workspace_id=${encodeURIComponent(workspaceId)}&team_key=${encodeURIComponent(teamKey)}`,
    options,
  );
}

export async function getLinearIssue(
  workspaceId: string,
  identifier: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<LinearIssue>(
    `/api/v1/linear/issues/${encodeURIComponent(identifier)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function searchLinearIssues(
  workspaceId: string,
  params: LinearSearchFilter & { pageToken?: string; maxResults?: number },
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ workspace_id: workspaceId });
  if (params.query) search.set("query", params.query);
  if (params.teamKey) search.set("team_key", params.teamKey);
  if (params.stateIds?.length) search.set("state_ids", params.stateIds.join(","));
  if (params.assigned) search.set("assigned", params.assigned);
  if (params.pageToken) search.set("page_token", params.pageToken);
  if (params.maxResults) search.set("max_results", String(params.maxResults));
  return fetchJson<LinearSearchResult>(`/api/v1/linear/issues?${search.toString()}`, options);
}

export async function setLinearIssueState(
  workspaceId: string,
  issueID: string,
  stateID: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ transitioned: boolean }>(
    `/api/v1/linear/issues/${encodeURIComponent(issueID)}/state?workspace_id=${encodeURIComponent(workspaceId)}`,
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
