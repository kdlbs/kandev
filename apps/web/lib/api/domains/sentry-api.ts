import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  CreateSentryIssueWatchRequest,
  SentryConfig,
  SentryIssue,
  SentryIssueWatch,
  SentryOrganization,
  SentryProject,
  SentrySearchFilter,
  SentrySearchResult,
  SetSentryConfigRequest,
  TestSentryConnectionResult,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";

// fetchSentryConfig returns undefined when the backend responds 204 (no config yet).
export async function fetchSentryConfig(
  options?: ApiRequestOptions,
): Promise<SentryConfig | undefined> {
  return fetchJson<SentryConfig | undefined>(`/api/v1/sentry/config`, options);
}

export async function saveSentryConfig(
  payload: SetSentryConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryConfig>(`/api/v1/sentry/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
  });
}

export async function deleteSentryConfig(options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`/api/v1/sentry/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

export async function testSentryConnection(
  secret?: string,
  url?: string,
  options?: ApiRequestOptions,
) {
  const payload: { secret?: string; url?: string } = {};
  if (secret) payload.secret = secret;
  if (url) payload.url = url;
  return fetchJson<TestSentryConnectionResult>(`/api/v1/sentry/config/test`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      body: JSON.stringify(payload),
    },
  });
}

export async function listSentryOrganizations(options?: ApiRequestOptions) {
  return fetchJson<{ organizations: SentryOrganization[] }>(
    `/api/v1/sentry/organizations`,
    options,
  );
}

export async function listSentryProjects(options?: ApiRequestOptions) {
  return fetchJson<{ projects: SentryProject[] }>(`/api/v1/sentry/projects`, options);
}

function appendFilter(search: URLSearchParams, filter: SentrySearchFilter): void {
  search.set("orgSlug", filter.orgSlug);
  if (filter.projectSlug) search.set("projectSlug", filter.projectSlug);
  if (filter.environment) search.set("environment", filter.environment);
  if (filter.query) search.set("query", filter.query);
  if (filter.statsPeriod) search.set("statsPeriod", filter.statsPeriod);
  for (const level of filter.levels ?? []) search.append("level", level);
  for (const status of filter.statuses ?? []) search.append("status", status);
}

export async function searchSentryIssues(
  filter: SentrySearchFilter,
  cursor?: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams();
  appendFilter(search, filter);
  if (cursor) search.set("cursor", cursor);
  return fetchJson<SentrySearchResult>(`/api/v1/sentry/issues?${search.toString()}`, options);
}

export async function getSentryIssue(idOrShortId: string, options?: ApiRequestOptions) {
  return fetchJson<SentryIssue>(
    `/api/v1/sentry/issues/${encodeURIComponent(idOrShortId)}`,
    options,
  );
}

// --- Issue watches ---

// listSentryIssueWatches fetches watches across all workspaces when
// workspaceId is omitted, or scoped to one workspace when provided.
export async function listSentryIssueWatches(workspaceId?: string, options?: ApiRequestOptions) {
  const path = workspaceId
    ? `/api/v1/sentry/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`
    : `/api/v1/sentry/watches/issue`;
  const res = await fetchJson<{ watches: SentryIssueWatch[] }>(path, options);
  return res.watches ?? [];
}

export async function getSentryIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryIssueWatch>(
    `/api/v1/sentry/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function createSentryIssueWatch(
  payload: CreateSentryIssueWatchRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryIssueWatch>(`/api/v1/sentry/watches/issue`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function updateSentryIssueWatch(
  id: string,
  workspaceId: string,
  payload: UpdateSentryIssueWatchRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryIssueWatch>(
    `/api/v1/sentry/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "PATCH", body: JSON.stringify(payload) },
    },
  );
}

export async function deleteSentryIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/sentry/watches/issue/${encodeURIComponent(id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "DELETE" },
    },
  );
}

export async function triggerSentryIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ published: number }>(
    `/api/v1/sentry/watches/issue/${encodeURIComponent(id)}/trigger?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}

// previewResetSentryIssueWatch returns how many tasks would be deleted if
// the watch were reset. Used by the confirmation dialog.
export async function previewResetSentryIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskCount: number }>(
    `/api/v1/sentry/watches/issue/${encodeURIComponent(id)}/reset/preview?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

// resetSentryIssueWatch deletes every task previously created by the watch
// (including archived), wipes its dedup table, and nulls last_polled_at so
// the next poll re-imports every currently-matching issue.
export async function resetSentryIssueWatch(
  id: string,
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ tasksDeleted: number }>(
    `/api/v1/sentry/watches/issue/${encodeURIComponent(id)}/reset?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}
