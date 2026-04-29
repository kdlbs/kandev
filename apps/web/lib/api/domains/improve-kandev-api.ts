import { fetchJson, type ApiRequestOptions } from "../client";
import type { LogEntry } from "@/lib/logger/buffer";

export type ImproveKandevBootstrapResponse = {
  repository_id: string;
  workflow_id: string;
  branch: string;
  bundle_dir: string;
  bundle_files: {
    metadata: string;
    backend_log: string;
    frontend_log: string;
  };
  github_login: string;
  has_write_access: boolean;
};

export async function bootstrapImproveKandev(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ImproveKandevBootstrapResponse> {
  return fetchJson<ImproveKandevBootstrapResponse>("/api/v1/system/improve-kandev/bootstrap", {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ workspace_id: workspaceId }),
      ...(options?.init ?? {}),
    },
  });
}

export type FrontendLogPayloadEntry = {
  timestamp: string;
  level: string;
  message: string;
  args?: unknown[];
  stack?: string;
};

export async function uploadFrontendLog(
  bundleDir: string,
  entries: LogEntry[],
  options?: ApiRequestOptions,
): Promise<{ path: string }> {
  const payload: FrontendLogPayloadEntry[] = entries.map((e) => ({
    timestamp: e.timestamp,
    level: e.level,
    message: e.message,
    args: e.args,
  }));
  return fetchJson<{ path: string }>("/api/v1/system/improve-kandev/bundle/frontend-log", {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ bundle_dir: bundleDir, entries: payload }),
      ...(options?.init ?? {}),
    },
  });
}
