import { fetchJson, type ApiRequestOptions } from "../client";

export type Share = {
  id: string;
  url: string;
  created_at: string;
  revoked_at?: string;
  snapshot_size_bytes: number;
};

export type ListSharesResponse = {
  shares: Share[];
};

// SnapshotPreview is the redacted snapshot returned by ?dry_run=true. The
// shape mirrors share.Snapshot in the Go backend; only the fields the
// preview UI renders today are typed here. Everything else is left as
// unknown so the contract is the backend's responsibility to keep stable.
export type SnapshotPreviewMessage = {
  role: "user" | "assistant" | "system";
  ts: string;
  blocks: Array<{
    kind: "text" | "tool_call" | "tool_result" | "diff";
    text?: string;
    tool_name?: string;
    args?: unknown;
    output?: string;
    truncated?: boolean;
    path?: string;
    unified_diff?: string;
  }>;
};

export type SnapshotPreview = {
  version: number;
  exported_at: string;
  task: { title: string; workflow_step?: string };
  session: {
    agent_type?: string;
    model?: string;
    executor_type?: string;
    started_at: string;
    completed_at?: string;
  };
  messages: SnapshotPreviewMessage[];
  redaction: { applied_rules: string[] };
};

export async function createShare(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<Share> {
  return fetchJson<Share>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/shares`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

export async function previewShare(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<SnapshotPreview> {
  return fetchJson<SnapshotPreview>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/shares?dry_run=true`,
    {
      ...options,
      init: { method: "POST", ...(options?.init ?? {}) },
    },
  );
}

export async function listShares(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<ListSharesResponse> {
  return fetchJson<ListSharesResponse>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/shares`,
    options,
  );
}

export async function revokeShare(shareId: string, options?: ApiRequestOptions): Promise<void> {
  await fetchJson<void>(`/api/v1/shares/${encodeURIComponent(shareId)}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}
