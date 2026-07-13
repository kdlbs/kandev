import { ApiError, fetchJson, type ApiRequestOptions } from "../client";
import type {
  CopySentryConfigRequest,
  CreateSentryConfigRequest,
  CreateSentryIssueWatchRequest,
  SentryConfig,
  SentryIssue,
  SentryIssueWatch,
  SentryOrganization,
  SentryProject,
  SentrySearchFilter,
  SentrySearchResult,
  TestSentryConnectionResult,
  UpdateSentryConfigRequest,
  UpdateSentryIssueWatchRequest,
} from "@/lib/types/sentry";

const BASE = "/api/v1/sentry";

// SENTRY_ERROR_CODES are the wire-level `code` discriminators the backend
// stamps on structured error bodies so the UI can react without matching on
// human-readable error text.
export const SENTRY_ERROR_CODES = {
  instanceRequired: "SENTRY_INSTANCE_REQUIRED",
  instanceNotFound: "SENTRY_INSTANCE_NOT_FOUND",
  instanceInUse: "SENTRY_INSTANCE_IN_USE",
  nameTaken: "SENTRY_INSTANCE_NAME_TAKEN",
  notConfigured: "SENTRY_NOT_CONFIGURED",
} as const;

// sentryErrorCode extracts the backend `code` from a failed request, or null
// when the rejection is not an ApiError or carries no code.
export function sentryErrorCode(err: unknown): string | null {
  if (!(err instanceof ApiError)) return null;
  const body = err.body;
  if (body && typeof body === "object" && "code" in body) {
    const code = body.code;
    return typeof code === "string" ? code : null;
  }
  return null;
}

// sentryInUseWatchCount returns the count of watches blocking a delete when the
// error is a 409 SENTRY_INSTANCE_IN_USE, else null.
export function sentryInUseWatchCount(err: unknown): number | null {
  if (!(err instanceof ApiError) || err.status !== 409) return null;
  if (sentryErrorCode(err) !== SENTRY_ERROR_CODES.instanceInUse) return null;
  const body = err.body;
  if (body && typeof body === "object" && "watchCount" in body) {
    const count = body.watchCount;
    return typeof count === "number" ? count : null;
  }
  return null;
}

// withParams appends non-empty query params to a path, respecting any existing
// query string.
function withParams(path: string, params: Record<string, string>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value) search.set(key, value);
  }
  const query = search.toString();
  if (!query) return path;
  return `${path}${path.includes("?") ? "&" : "?"}${query}`;
}

// --- Instances (per-workspace named Sentry configs) ---

// listSentryInstances returns every Sentry instance configured in a workspace.
export async function listSentryInstances(workspaceId: string, options?: ApiRequestOptions) {
  const res = await fetchJson<{ instances: SentryConfig[] }>(
    withParams(`${BASE}/instances`, { workspace_id: workspaceId }),
    options,
  );
  return res.instances ?? [];
}

// getSentryInstance fetches one instance by id, scoped to its workspace.
export async function getSentryInstance(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryConfig>(
    withParams(`${BASE}/instances/${encodeURIComponent(id)}`, { workspace_id: workspaceId }),
    options,
  );
}

// createSentryInstance adds a new named instance to a workspace.
export async function createSentryInstance(
  workspaceId: string,
  payload: CreateSentryConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryConfig>(withParams(`${BASE}/instances`, { workspace_id: workspaceId }), {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

// updateSentryInstance replaces an instance's name/url/auth (and, when a
// non-empty secret is supplied, its stored token).
export async function updateSentryInstance(
  workspaceId: string,
  id: string,
  payload: UpdateSentryConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryConfig>(
    withParams(`${BASE}/instances/${encodeURIComponent(id)}`, { workspace_id: workspaceId }),
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "PUT", body: JSON.stringify(payload) },
    },
  );
}

// deleteSentryInstance removes an instance. Rejects with a 409
// SENTRY_INSTANCE_IN_USE (carrying watchCount) when watches still bind to it.
export async function deleteSentryInstance(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ deleted: boolean }>(
    withParams(`${BASE}/instances/${encodeURIComponent(id)}`, { workspace_id: workspaceId }),
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "DELETE" },
    },
  );
}

// testSentryInstance pings Sentry with a saved instance's stored credentials.
export async function testSentryInstance(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<TestSentryConnectionResult>(
    withParams(`${BASE}/instances/${encodeURIComponent(id)}/test`, { workspace_id: workspaceId }),
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}

// testSentryConnection pings Sentry with ad-hoc credentials (before an instance
// is saved) to validate a token/URL pair.
export async function testSentryConnection(
  workspaceId: string,
  creds: { secret?: string; url?: string; authMethod?: string },
  options?: ApiRequestOptions,
) {
  const payload: { secret?: string; url?: string; authMethod?: string } = {};
  if (creds.secret) payload.secret = creds.secret;
  if (creds.url) payload.url = creds.url;
  if (creds.authMethod) payload.authMethod = creds.authMethod;
  return fetchJson<TestSentryConnectionResult>(
    withParams(`${BASE}/test-connection`, { workspace_id: workspaceId }),
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    },
  );
}

// copySentryInstances copies every instance (and its credential) from the
// source workspace (options.workspaceId) into the target workspace in the
// JSON body, returning the newly-created instances. The source workspace is
// authoritative for this route, so options.workspaceId is required —
// mirrors copyGitHubWorkspaceSettings's convention for consistency.
export async function copySentryInstances(
  targetWorkspaceId: string,
  options: { workspaceId: string } & ApiRequestOptions,
) {
  const { workspaceId: sourceWorkspaceId, ...requestOptions } = options;
  const payload: Pick<CopySentryConfigRequest, "targetWorkspaceId"> = { targetWorkspaceId };
  const res = await fetchJson<{ instances: SentryConfig[] }>(
    withParams(`${BASE}/config/copy`, { workspace_id: sourceWorkspaceId }),
    {
      ...requestOptions,
      init: { ...(requestOptions.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    },
  );
  return res.instances ?? [];
}

// --- Browse (org/project/issue lookups scoped to one instance) ---

export async function listSentryOrganizations(
  workspaceId: string,
  instanceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ organizations: SentryOrganization[] }>(
    withParams(`${BASE}/organizations`, { workspace_id: workspaceId, instanceId }),
    options,
  );
}

export async function listSentryProjects(
  workspaceId: string,
  instanceId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ projects: SentryProject[] }>(
    withParams(`${BASE}/projects`, { workspace_id: workspaceId, instanceId }),
    options,
  );
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
  workspaceId: string,
  instanceId: string,
  filter: SentrySearchFilter,
  cursor?: string,
  options?: ApiRequestOptions,
) {
  const search = new URLSearchParams();
  search.set("workspace_id", workspaceId);
  search.set("instanceId", instanceId);
  appendFilter(search, filter);
  if (cursor) search.set("cursor", cursor);
  return fetchJson<SentrySearchResult>(`${BASE}/issues?${search.toString()}`, options);
}

export async function getSentryIssue(
  workspaceId: string,
  instanceId: string,
  idOrShortId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<SentryIssue>(
    withParams(`${BASE}/issues/${encodeURIComponent(idOrShortId)}`, {
      workspace_id: workspaceId,
      instanceId,
    }),
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
  return fetchJson<SentryIssueWatch>(
    withParams(`${BASE}/watches/issue`, { workspace_id: payload.workspaceId }),
    {
      ...options,
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    },
  );
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
