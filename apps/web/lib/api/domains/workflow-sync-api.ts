import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  WorkflowSyncConfig,
  WorkflowSyncForceSyncResponse,
  WorkflowSyncSetConfigRequest,
} from "@/lib/types/workflow-sync";

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

// getWorkflowSyncConfig returns null when the backend responds 204 (no
// config yet for this workspace). fetchJson already maps 204 → undefined; we
// narrow it to null for callers.
export async function getWorkflowSyncConfig(
  options?: WorkspaceApiOptions,
): Promise<WorkflowSyncConfig | null> {
  const res = await fetchJson<WorkflowSyncConfig | undefined>(
    withWorkspace(`/api/v1/workflow-sync/config`, options),
    requestOptions(options),
  );
  return res ?? null;
}

export async function setWorkflowSyncConfig(
  payload: WorkflowSyncSetConfigRequest,
  options?: WorkspaceApiOptions,
) {
  return fetchJson<WorkflowSyncConfig>(withWorkspace(`/api/v1/workflow-sync/config`, options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export async function deleteWorkflowSyncConfig(options?: WorkspaceApiOptions) {
  return fetchJson<{ deleted: boolean }>(withWorkspace(`/api/v1/workflow-sync/config`, options), {
    ...requestOptions(options),
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

// forceWorkflowSync triggers an immediate sync. Rejects with an ApiError
// (404) when the workspace has no config; a failed sync attempt still
// resolves 200 with `error` set and `config.last_ok === false`.
export async function forceWorkflowSync(options?: WorkspaceApiOptions) {
  return fetchJson<WorkflowSyncForceSyncResponse>(
    withWorkspace(`/api/v1/workflow-sync/sync`, options),
    {
      ...requestOptions(options),
      init: { ...(options?.init ?? {}), method: "POST" },
    },
  );
}
