import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  CreateJiraIssueWatchInput,
  JiraConfig,
  JiraIssueWatch,
  JiraProject,
  JiraSearchResult,
  JiraTicket,
  SetJiraConfigRequest,
  TestJiraConnectionResult,
  UpdateJiraIssueWatchInput,
} from "@/lib/types/jira";

// getJiraConfig returns null when the backend responds 204 (no config yet).
// fetchJson already maps 204 → undefined; we narrow it to null for callers.
export async function getJiraConfig(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<JiraConfig | null> {
  const res = await fetchJson<JiraConfig | undefined>(
    `/api/v1/jira/config?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
  return res ?? null;
}

export async function setJiraConfig(payload: SetJiraConfigRequest, options?: ApiRequestOptions) {
  return fetchJson<JiraConfig>(`/api/v1/jira/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deleteJiraConfig(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/jira/config?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "DELETE" } },
  );
}

export async function testJiraConnection(
  payload: SetJiraConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TestJiraConnectionResult>(`/api/v1/jira/config/test`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function listJiraProjects(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ projects: JiraProject[] }>(
    `/api/v1/jira/projects?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function getJiraTicket(
  workspaceId: string,
  ticketKey: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<JiraTicket>(
    `/api/v1/jira/tickets/${encodeURIComponent(ticketKey)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function searchJiraTickets(
  workspaceId: string,
  params: { jql?: string; pageToken?: string; maxResults?: number },
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams({ workspace_id: workspaceId });
  if (params.jql) search.set("jql", params.jql);
  if (params.pageToken) search.set("page_token", params.pageToken);
  if (params.maxResults) search.set("max_results", String(params.maxResults));
  return fetchJson<JiraSearchResult>(`/api/v1/jira/tickets?${search.toString()}`, options);
}

export async function transitionJiraTicket(
  workspaceId: string,
  ticketKey: string,
  transitionId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ transitioned: boolean }>(
    `/api/v1/jira/tickets/${encodeURIComponent(ticketKey)}/transitions?workspace_id=${encodeURIComponent(workspaceId)}`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify({ transitionId }) },
    },
  );
}

// --- Issue watches ---

export async function listJiraIssueWatches(workspaceId: string, options?: ApiRequestOptions) {
  const res = await fetchJson<{ watches: JiraIssueWatch[] }>(
    `/api/v1/jira/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
  return res.watches ?? [];
}

export async function createJiraIssueWatch(
  payload: CreateJiraIssueWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<JiraIssueWatch>(`/api/v1/jira/watches/issue`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function updateJiraIssueWatch(
  id: string,
  payload: UpdateJiraIssueWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<JiraIssueWatch>(`/api/v1/jira/watches/issue/${encodeURIComponent(id)}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "PATCH", body: JSON.stringify(payload) },
  });
}

export async function deleteJiraIssueWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/jira/watches/issue/${encodeURIComponent(id)}`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export async function triggerJiraIssueWatch(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ newIssues: number }>(
    `/api/v1/jira/watches/issue/${encodeURIComponent(id)}/trigger`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}
