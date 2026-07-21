import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  CreateLinearIssueWatchInput,
  LinearConfig,
  LinearIssue,
  LinearIssueWatch,
  LinearLabel,
  LinearSearchFilter,
  LinearSearchResult,
  LinearTeam,
  LinearUser,
  LinearWorkflowState,
  SetLinearConfigRequest,
  TestLinearConnectionResult,
  UpdateLinearIssueWatchInput,
} from "@/lib/types/linear";
import { invalidateIntegrationAvailabilityAfter } from "@/lib/integrations/integration-availability-events";

type WorkspaceApiOptions = ApiRequestOptions & { workspaceId?: string };

function withWorkspace(path: string, options?: WorkspaceApiOptions): string {
  if (!options?.workspaceId) return path;
  const separator = path.includes("?") ? "&" : "?";
  return `${path}${separator}workspace_id=${encodeURIComponent(options.workspaceId)}`;
}

function requestOptions(options?: WorkspaceApiOptions): ApiRequestOptions | undefined {
  if (!options) return undefined;
  const { workspaceId: _workspaceId, ...rest } = options;
  return rest;
}

// getLinearConfig returns null when the backend responds 204 (no config yet).
export async function getLinearConfig(options?: WorkspaceApiOptions): Promise<LinearConfig | null> {
  const res = await fetchJson<LinearConfig | undefined>(
    withWorkspace(`/api/v1/linear/config`, options),
    requestOptions(options),
  );
  return res ?? null;
}

export async function setLinearConfig(
  payload: SetLinearConfigRequest,
  options?: WorkspaceApiOptions,
) {
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<LinearConfig>(withWorkspace(`/api/v1/linear/config`, options), {
      ...requestOptions(options),
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    }),
  );
}

export async function deleteLinearConfig(options?: WorkspaceApiOptions) {
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<{ deleted: boolean }>(withWorkspace(`/api/v1/linear/config`, options), {
      ...requestOptions(options),
      init: { ...(options?.init ?? {}), method: "DELETE" },
    }),
  );
}

export async function testLinearConnection(
  payload: SetLinearConfigRequest,
  options?: WorkspaceApiOptions,
) {
  return fetchJson<TestLinearConnectionResult>(
    withWorkspace(`/api/v1/linear/config/test`, options),
    {
      ...requestOptions(options),
      init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
    },
  );
}

// copyLinearConfig copies the Linear config + credential from the workspace in
// options (source) to targetWorkspaceId.
export async function copyLinearConfig(targetWorkspaceId: string, options?: WorkspaceApiOptions) {
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<LinearConfig>(withWorkspace(`/api/v1/linear/config/copy`, options), {
      ...requestOptions(options),
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        body: JSON.stringify({ targetWorkspaceId }),
      },
    }),
  );
}

export async function listLinearTeams(options?: WorkspaceApiOptions) {
  return fetchJson<{ teams: LinearTeam[] }>(
    withWorkspace(`/api/v1/linear/teams`, options),
    requestOptions(options),
  );
}

export async function listLinearStates(teamKey: string, options?: WorkspaceApiOptions) {
  return fetchJson<{ states: LinearWorkflowState[] }>(
    withWorkspace(`/api/v1/linear/states?team_key=${encodeURIComponent(teamKey)}`, options),
    requestOptions(options),
  );
}

export async function listLinearLabels(teamKey: string, options?: WorkspaceApiOptions) {
  return fetchJson<{ labels: LinearLabel[] }>(
    withWorkspace(`/api/v1/linear/labels?team_key=${encodeURIComponent(teamKey)}`, options),
    requestOptions(options),
  );
}

export async function listLinearUsers(teamKey: string, options?: WorkspaceApiOptions) {
  return fetchJson<{ users: LinearUser[] }>(
    withWorkspace(`/api/v1/linear/users?team_key=${encodeURIComponent(teamKey)}`, options),
    requestOptions(options),
  );
}

export async function getLinearIssue(identifier: string, options?: WorkspaceApiOptions) {
  return fetchJson<LinearIssue>(
    withWorkspace(`/api/v1/linear/issues/${encodeURIComponent(identifier)}`, options),
    requestOptions(options),
  );
}

function buildSearchIssuesQuery(
  params: LinearSearchFilter & { pageToken?: string; maxResults?: number },
): string {
  // Mapping table keeps the function flat — the complexity linter complains
  // about a long chain of ifs, but a `[wire, value]` array reads as a single
  // pass over the fields.
  const entries: [string, string | undefined][] = [
    ["query", params.query],
    ["team_key", params.teamKey],
    ["assigned", params.assigned],
    ["creator_id", params.creatorId],
    ["state_ids", params.stateIds?.length ? params.stateIds.join(",") : undefined],
    ["label_ids", params.labelIds?.length ? params.labelIds.join(",") : undefined],
    ["priorities", params.priorities?.length ? params.priorities.join(",") : undefined],
    ["estimate_min", params.estimateMin !== undefined ? String(params.estimateMin) : undefined],
    ["estimate_max", params.estimateMax !== undefined ? String(params.estimateMax) : undefined],
    ["page_token", params.pageToken],
    ["max_results", params.maxResults ? String(params.maxResults) : undefined],
  ];
  const search = new URLSearchParams();
  for (const [key, value] of entries) {
    if (value) search.set(key, value);
  }
  return search.toString();
}

export async function searchLinearIssues(
  params: LinearSearchFilter & { pageToken?: string; maxResults?: number },
  options?: WorkspaceApiOptions,
) {
  const qs = buildSearchIssuesQuery(params);
  return fetchJson<LinearSearchResult>(
    withWorkspace(`/api/v1/linear/issues${qs ? `?${qs}` : ""}`, options),
    requestOptions(options),
  );
}

export async function setLinearIssueState(
  issueID: string,
  stateID: string,
  options?: WorkspaceApiOptions,
) {
  return fetchJson<{ transitioned: boolean }>(
    withWorkspace(`/api/v1/linear/issues/${encodeURIComponent(issueID)}/state`, options),
    {
      ...requestOptions(options),
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

// previewResetLinearIssueWatch returns how many tasks would be deleted if
// the watch were reset. Used by the confirmation dialog.
export async function previewResetLinearIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ taskCount: number }>(
    `/api/v1/linear/watches/issue/${encodeURIComponent(id)}/reset/preview?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

// resetLinearIssueWatch deletes every task previously created by the watch
// (including archived), wipes its dedup table, and nulls last_polled_at so
// the next poll re-imports every currently-matching issue.
export async function resetLinearIssueWatch(
  workspaceId: string,
  id: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ tasksDeleted: number }>(
    `/api/v1/linear/watches/issue/${encodeURIComponent(id)}/reset?workspace_id=${encodeURIComponent(workspaceId)}`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}
