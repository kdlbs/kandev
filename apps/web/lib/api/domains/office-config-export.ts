import { fetchBlob, fetchJson, type ApiRequestOptions } from "../client";

const BASE = "/api/v1/office";

export type ConfigExportFile = {
  path: string;
  content: string;
};

export type ConfigExportManifest = {
  revision: string;
  files: ConfigExportFile[];
};

export function exportConfigManifest(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<ConfigExportManifest>(
    `${BASE}/workspaces/${workspaceId}/config/export/manifest`,
    options,
  );
}

export const exportConfigZipUrl = (workspaceId: string) =>
  `${BASE}/workspaces/${workspaceId}/config/export/zip`;

export function exportSelectedConfigZip(
  workspaceId: string,
  request: { revision: string; paths: string[] },
  options?: ApiRequestOptions,
) {
  return fetchBlob(`${BASE}/workspaces/${workspaceId}/config/export/zip`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify(request),
      ...options?.init,
    },
  });
}
