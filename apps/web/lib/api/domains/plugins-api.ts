import {
  ApiError,
  bootTokenHeaders,
  fetchJson,
  mutationInit,
  type ApiRequestOptions,
} from "../client";
import { getBackendConfig } from "@/lib/config";
import type { PluginRecord, SyncResult } from "@/lib/types/plugins";

const BASE = "/api/plugins";

// listPlugins fetches every registered plugin (GET /api/plugins).
export async function listPlugins(options?: ApiRequestOptions) {
  const res = await fetchJson<{ plugins?: PluginRecord[] }>(BASE, options);
  return res.plugins ?? [];
}

// getPlugin fetches a single plugin record by id. Rejects with a 404
// ApiError when the id is unknown.
export async function getPlugin(id: string, options?: ApiRequestOptions) {
  return fetchJson<PluginRecord>(`${BASE}/${encodeURIComponent(id)}`, options);
}

// InstallResult mirrors the backend's InstallResponse
// (internal/plugins/dto.go): the install endpoint always wraps the stored
// record under "plugin" (plus an optional "warning" when the package
// installed but its initial spawn/handshake failed — Plugin.Status is left
// as "error" in that case). Callers must surface `warning` — a plugin can
// install successfully yet still fail to come up.
export type InstallResult = {
  plugin: PluginRecord;
  warning?: string;
};

// installPluginFromUrl asks the backend to download, verify, and install a
// plugin package from a remote URL (POST /api/plugins/install, JSON body).
// Returns the stored plugin record plus an optional partial-install warning.
// Throws ApiError on 400 (invalid package/manifest — message names the
// problem, e.g. bad checksum or missing platform executable) or 409 (that
// version is already installed).
export async function installPluginFromUrl(
  url: string,
  options?: ApiRequestOptions,
): Promise<InstallResult> {
  return fetchJson<InstallResult>(`${BASE}/install`, {
    ...options,
    init: mutationInit("POST", options, JSON.stringify({ url })),
  });
}

// installPluginUpload uploads a plugin package (.tar.gz) directly as
// multipart/form-data under the "package" field (POST /api/plugins/install).
// Bypasses fetchJson, which always forces a JSON Content-Type header — that
// would stop the browser from setting the multipart boundary itself. Same
// return shape and error semantics as installPluginFromUrl.
export async function installPluginUpload(
  file: File,
  options?: ApiRequestOptions,
): Promise<InstallResult> {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const formData = new FormData();
  formData.append("package", file);

  // Spread caller init *first* so method/body always win, matching the
  // convention in lib/api/domains/voice-api.ts. The operator-token header is
  // merged last (after caller headers) so this multipart install — a CORS
  // "simple" request and the C1 CSRF-RCE vector — carries it. Content-Type is
  // deliberately never set here, so the browser sets the multipart boundary;
  // headers stays absent entirely when there is neither a caller header nor a
  // token to send.
  const headers = { ...(options?.init?.headers ?? {}), ...bootTokenHeaders() };
  const response = await fetch(`${baseUrl}${BASE}/install`, {
    ...options?.init,
    method: "POST",
    body: formData,
    ...(Object.keys(headers).length > 0 ? { headers } : {}),
  });

  if (!response.ok) await throwInstallError(response);
  return (await response.json()) as InstallResult;
}

async function throwInstallError(response: Response): Promise<never> {
  let body: unknown = null;
  let message = `Plugin install failed: ${response.status} ${response.statusText}`;
  try {
    body = await response.json();
  } catch {
    // body remains null
  }
  if (body && typeof body === "object" && "error" in body) {
    const errVal = (body as { error?: unknown }).error;
    if (typeof errVal === "string") message = errVal;
  }
  throw new ApiError(message, response.status, body);
}

// getPluginConfig fetches a plugin's stored operator config
// (GET /api/plugins/:id/config). Secret values (per the manifest's
// config_schema) arrive masked — the backend never returns them in
// cleartext on this surface.
export async function getPluginConfig(id: string, options?: ApiRequestOptions) {
  const res = await fetchJson<{ config?: Record<string, unknown> }>(
    `${BASE}/${encodeURIComponent(id)}/config`,
    options,
  );
  return res.config ?? {};
}

// updatePluginConfig replaces a plugin's operator-editable config
// (PATCH /api/plugins/:id). Secret fields submitted as the mask placeholder
// keep their stored value server-side.
export async function updatePluginConfig(
  id: string,
  config: Record<string, unknown>,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ updated: boolean }>(`${BASE}/${encodeURIComponent(id)}`, {
    ...options,
    init: mutationInit("PATCH", options, JSON.stringify({ config })),
  });
}

// enablePlugin transitions a plugin to active (POST /api/plugins/:id/enable).
export async function enablePlugin(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ enabled: boolean }>(`${BASE}/${encodeURIComponent(id)}/enable`, {
    ...options,
    init: mutationInit("POST", options),
  });
}

// disablePlugin transitions a plugin to disabled (POST /api/plugins/:id/disable).
export async function disablePlugin(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ disabled: boolean }>(`${BASE}/${encodeURIComponent(id)}/disable`, {
    ...options,
    init: mutationInit("POST", options),
  });
}

// uninstallPlugin removes a plugin's registration (DELETE /api/plugins/:id).
export async function uninstallPlugin(id: string, options?: ApiRequestOptions) {
  return fetchJson<{ deleted: boolean }>(`${BASE}/${encodeURIComponent(id)}`, {
    ...options,
    init: mutationInit("DELETE", options),
  });
}

// syncPlugins reconciles the plugin registry with the plugins directory on
// disk (POST /api/plugins/sync): registers unrecorded directory sideloads
// as disabled, installs and deletes dropped *.tar.gz packages, and marks
// records with a missing install path as errored. Returns the SyncResult
// summarizing what changed this run.
export async function syncPlugins(options?: ApiRequestOptions): Promise<SyncResult> {
  return fetchJson<SyncResult>(`${BASE}/sync`, {
    ...options,
    init: mutationInit("POST", options),
  });
}
