import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  CreateJiraIssueWatchInput,
  JiraConfig,
  JiraIssueWatch,
  JiraProject,
  JiraSearchResult,
  JiraStatus,
  JiraTicket,
  SetJiraConfigRequest,
  TestJiraConnectionResult,
  UpdateJiraIssueWatchInput,
} from "@/lib/types/jira";

// getJiraConfig returns null when the backend responds 204 (no config yet).
// fetchJson already maps 204 → undefined; we narrow it to null for callers.
export async function getJiraConfig(options?: ApiRequestOptions): Promise<JiraConfig | null> {
  const res = await fetchJson<JiraConfig | undefined>(`/api/v1/jira/config`, options);
  return res ?? null;
}

export async function setJiraConfig(payload: SetJiraConfigRequest, options?: ApiRequestOptions) {
  return fetchJson<JiraConfig>(`/api/v1/jira/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deleteJiraConfig(options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/jira/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
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

export async function listJiraProjects(options?: ApiRequestOptions) {
  return fetchJson<{ projects: JiraProject[] }>(`/api/v1/jira/projects`, options);
}

export async function listJiraProjectStatuses(projectKey: string, options?: ApiRequestOptions) {
  return fetchJson<{ statuses: JiraStatus[] }>(
    `/api/v1/jira/projects/${encodeURIComponent(projectKey)}/statuses`,
    options,
  );
}

export async function getJiraTicket(ticketKey: string, options?: ApiRequestOptions) {
  return fetchJson<JiraTicket>(`/api/v1/jira/tickets/${encodeURIComponent(ticketKey)}`, options);
}

export async function searchJiraTickets(
  params: { jql?: string; pageToken?: string; maxResults?: number },
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams();
  if (params.jql) search.set("jql", params.jql);
  if (params.pageToken) search.set("page_token", params.pageToken);
  if (params.maxResults) search.set("max_results", String(params.maxResults));
  const qs = search.toString();
  return fetchJson<JiraSearchResult>(`/api/v1/jira/tickets${qs ? `?${qs}` : ""}`, options);
}

export async function transitionJiraTicket(
  ticketKey: string,
  transitionId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ transitioned: boolean }>(
    `/api/v1/jira/tickets/${encodeURIComponent(ticketKey)}/transitions`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify({ transitionId }) },
    },
  );
}

// --- Issue watches ---

// listJiraIssueWatches fetches watches across all workspaces when workspaceId
// is omitted, or scoped to one workspace when provided.
export async function listJiraIssueWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const path = workspaceId
    ? `/api/v1/jira/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`
    : `/api/v1/jira/watches/issue`;
  const res = await fetchJson<{ watches: JiraIssueWatch[] }>(path, options);
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

// All mutation/trigger endpoints require `workspace_id` so the backend can
// reject cross-workspace IDOR (a watch UUID from another workspace would
// otherwise be mutable by id alone). The list/create endpoints already scope
// by workspace; these now match.

export async function updateJiraIssueWatch(
  workspaceId: string,
  id: string,
  payload: UpdateJiraIssueWatchInput,
  options?: ApiRequestOptions,
) {
  return fetchJson<JiraIssueWatch>(
    `/api/v1/jira/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "PATCH", body: JSON.stringify(payload) },
    },
  );
}

export async function deleteJiraIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/jira/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "DELETE" } },
  );
}

export async function triggerJiraIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ newIssues: number }>(
    `/api/v1/jira/watches/issue/${encodeURIComponent(id)}/trigger?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}

// previewResetJiraIssueWatch returns how many tasks would be deleted if the
// watch were reset. Used by the confirmation dialog.
export async function previewResetJiraIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskCount: number }>(
    `/api/v1/jira/watches/issue/${encodeURIComponent(id)}/reset/preview?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

// resetJiraIssueWatch deletes every task previously created by the watch
// (including archived), wipes its dedup table, and nulls last_polled_at so
// the next poll re-imports every currently-matching ticket.
export async function resetJiraIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ tasksDeleted: number }>(
    `/api/v1/jira/watches/issue/${encodeURIComponent(id)}/reset?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}
