import { fetchJson, type ApiRequestOptions } from "../client";
import { generateUUID } from "@/lib/utils";
import type {
  WorkloadRunObservation,
  BackgroundWorkActionRequest,
  BackgroundWorkActionResponse,
} from "@/lib/types/background-work";
import { getWebSocketClient } from "@/lib/ws/connection";

export type ListBackgroundWorkloadsResponse = {
  workloads: WorkloadRunObservation[];
};

export type GetBackgroundWorkloadResponse = {
  workload: WorkloadRunObservation;
};

export type BackgroundWorkloadUsageResponse = {
  work_id: string;
  tokens_in: number;
  tokens_out: number;
  tokens_total: number;
  cost_subcents: number;
  provenance: "reported" | "estimated" | "unavailable";
};

export async function listBackgroundWorkloads(
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<ListBackgroundWorkloadsResponse> {
  return fetchJson<ListBackgroundWorkloadsResponse>(
    `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/background-work`,
    options,
  );
}

export async function getBackgroundWorkload(
  sessionId: string,
  workId: string,
  options?: ApiRequestOptions,
): Promise<GetBackgroundWorkloadResponse> {
  return fetchJson<GetBackgroundWorkloadResponse>(
    `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/background-work/${encodeURIComponent(workId)}`,
    options,
  );
}

export async function executeBackgroundAction(
  sessionId: string,
  req: BackgroundWorkActionRequest,
  options?: ApiRequestOptions,
): Promise<BackgroundWorkActionResponse> {
  const opId = req.operation_id || generateUUID();
  const fullReq: BackgroundWorkActionRequest = { ...req, operation_id: opId };
  const wsClient = getWebSocketClient();
  if (wsClient && wsClient.getStatus() === "connected") {
    try {
      return await wsClient.request<BackgroundWorkActionResponse>(
        "session.background_work.action",
        { session_id: sessionId, ...fullReq },
        10000,
      );
    } catch (err) {
      // Do not transparently re-post over HTTP on WS failure/uncertainty (R10)
      return {
        success: false,
        work_id: req.work_id,
        run_id: req.run_id,
        action: req.action,
        // i18n-exempt: fallback diagnostic error string for failed websocket action
        error: err instanceof Error ? err.message : "WebSocket action failed",
        uncertain: true,
      };
    }
  }
  return fetchJson<BackgroundWorkActionResponse>(
    `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/background-work/${encodeURIComponent(req.work_id)}/action`,
    {
      ...options,
      init: {
        ...(options?.init ?? {}),
        method: "POST",
        body: JSON.stringify(fullReq),
      },
    },
  );
}

export async function getBackgroundWorkloadUsage(
  sessionId: string,
  workId: string,
  options?: ApiRequestOptions,
): Promise<BackgroundWorkloadUsageResponse> {
  return fetchJson<BackgroundWorkloadUsageResponse>(
    `/api/v1/task-sessions/${encodeURIComponent(sessionId)}/background-work/${encodeURIComponent(workId)}/usage`,
    options,
  );
}
