import { fetchJson, type ApiRequestOptions } from "../client";
import { getBackendConfig } from "@/lib/config";
import type { ProcessInfo } from "@/lib/types/http";

export type HtmlPreviewPublishResponse = {
  port: number;
  path: string;
  version: number;
};

export type HtmlPreviewPublishRequest = {
  repo?: string;
  path: string;
  content: string;
};

export async function publishHtmlPreview(
  sessionId: string,
  payload: HtmlPreviewPublishRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<HtmlPreviewPublishResponse>(
    `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/html-previews`,
    {
      ...options,
      init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
    },
  );
}

export function buildHtmlPreviewProxyUrl(
  sessionId: string,
  response: HtmlPreviewPublishResponse,
  options?: ApiRequestOptions,
): string {
  const baseUrl = (options?.baseUrl ?? getBackendConfig().apiBaseUrl).replace(/\/+$/, "");
  const path = response.path.startsWith("/") ? response.path : `/${response.path}`;
  const url = new URL(
    `${baseUrl}/port-proxy/${encodeURIComponent(sessionId)}/${response.port}${path}`,
  );
  url.searchParams.set("v", String(response.version));
  return url.toString();
}

// Process operations
export async function startProcess(
  sessionId: string,
  payload: { kind: string; script_name?: string; repo_id?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ process: ProcessInfo }>(`/api/v1/task-sessions/${sessionId}/processes/start`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function stopProcess(
  sessionId: string,
  payload: { process_id: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<null>(
    `/api/v1/task-sessions/${sessionId}/processes/${payload.process_id}/stop`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

export async function listSessionProcesses(sessionId: string, options?: ApiRequestOptions) {
  return fetchJson<ProcessInfo[]>(`/api/v1/task-sessions/${sessionId}/processes`, options);
}

export async function getSessionProcess(
  sessionId: string,
  processId: string,
  includeOutput = false,
  options?: ApiRequestOptions,
) {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/task-sessions/${sessionId}/processes/${processId}`);
  if (includeOutput) {
    url.searchParams.set("include_output", "true");
  }
  return fetchJson<ProcessInfo>(url.toString(), options);
}
