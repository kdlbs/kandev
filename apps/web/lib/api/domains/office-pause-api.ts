import { fetchJson, type ApiRequestOptions } from "../client";
import type { WorkspacePauseRecord } from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

type RawWorkspacePause = {
  id: string;
  reason: string;
  created_by: string;
  created_by_kind: string;
  created_at: string;
};

function normalizePauseRecord(raw: RawWorkspacePause | null): WorkspacePauseRecord | null {
  if (!raw) return null;
  return {
    id: raw.id,
    reason: raw.reason,
    createdBy: raw.created_by,
    createdByKind: raw.created_by_kind,
    createdAt: raw.created_at,
  };
}

export type WorkspacePauseResponse = {
  workspaceId: string;
  paused: boolean;
  record: WorkspacePauseRecord | null;
  sweep?: WorkspacePauseSweep;
};

export type WorkspacePauseSweep = {
  runsCancelled: number;
  executionsCancelled: number;
  executionsNotRunning: number;
  failures: number;
};

type RawWorkspacePauseResponse = {
  workspace_id: string;
  paused: boolean;
  pause: RawWorkspacePause | null;
  sweep?: RawWorkspacePauseSweep;
};

type RawWorkspacePauseSweep = {
  runs_cancelled: number;
  executions_cancelled: number;
  executions_not_running: number;
  failures: number;
};

function normalizePauseResponse(raw: RawWorkspacePauseResponse): WorkspacePauseResponse {
  const response: WorkspacePauseResponse = {
    workspaceId: raw.workspace_id,
    paused: raw.paused,
    record: normalizePauseRecord(raw.pause),
  };
  if (raw.sweep) {
    response.sweep = {
      runsCancelled: raw.sweep.runs_cancelled,
      executionsCancelled: raw.sweep.executions_cancelled,
      executionsNotRunning: raw.sweep.executions_not_running,
      failures: raw.sweep.failures,
    };
  }
  return response;
}

export function getWorkspacePause(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<WorkspacePauseResponse> {
  return fetchJson<RawWorkspacePauseResponse>(
    `${BASE}/workspaces/${workspaceId}/pause`,
    options,
  ).then(normalizePauseResponse);
}

export function postWorkspacePause(
  workspaceId: string,
  reason: string,
  options?: ApiRequestOptions,
): Promise<WorkspacePauseResponse> {
  return fetchJson<RawWorkspacePauseResponse>(`${BASE}/workspaces/${workspaceId}/pause`, {
    ...options,
    init: { method: "POST", body: JSON.stringify({ reason }), ...options?.init },
  }).then(normalizePauseResponse);
}

export function postWorkspaceResume(
  workspaceId: string,
  reason: string,
  options?: ApiRequestOptions,
): Promise<WorkspacePauseResponse> {
  return fetchJson<RawWorkspacePauseResponse>(`${BASE}/workspaces/${workspaceId}/resume`, {
    ...options,
    init: { method: "POST", body: JSON.stringify({ reason }), ...options?.init },
  }).then(normalizePauseResponse);
}
