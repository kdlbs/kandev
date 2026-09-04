import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  OfficeConfigSyncConfig,
  OfficeConfigSyncForceSyncResponse,
  OfficeConfigSyncSetConfigRequest,
} from "@/lib/types/office-config-sync";

const BASE = "/api/v1/office";

// getOfficeConfigSyncConfig returns null when the backend responds 204 (no
// config yet for this workspace). fetchJson already maps 204 → undefined; we
// narrow it to null for callers.
export async function getOfficeConfigSyncConfig(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<OfficeConfigSyncConfig | null> {
  const res = await fetchJson<OfficeConfigSyncConfig | undefined>(
    `${BASE}/workspaces/${workspaceId}/config-sync/config`,
    options,
  );
  return res ?? null;
}

export function setOfficeConfigSyncConfig(
  workspaceId: string,
  payload: OfficeConfigSyncSetConfigRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<OfficeConfigSyncConfig>(`${BASE}/workspaces/${workspaceId}/config-sync/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
  });
}

export function deleteOfficeConfigSyncConfig(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`${BASE}/workspaces/${workspaceId}/config-sync/config`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "DELETE" },
  });
}

// forceOfficeConfigSync triggers an immediate sync. Rejects with an ApiError
// (404) when the workspace has no config; a failed sync attempt still
// resolves 200 with `error` set and `config.last_ok === false`.
export function forceOfficeConfigSync(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<OfficeConfigSyncForceSyncResponse>(
    `${BASE}/workspaces/${workspaceId}/config-sync/sync`,
    { ...options, init: { ...(options?.init ?? {}), method: "POST" } },
  );
}
