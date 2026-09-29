import { fetchJson, type ApiRequestOptions } from "../client";
import type { SessionUsageTotals, UsageTurn, UsageTurnsPage } from "@/lib/types/conversation-usage";

export async function getSessionUsageTotals(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<SessionUsageTotals> {
  return fetchJson<SessionUsageTotals>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/usage`,
    options,
  );
}

export async function listSessionUsageTurns(
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<UsageTurnsPage> {
  return fetchJson<UsageTurnsPage>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/usage/turns?limit=1`,
    options,
  );
}

export async function getSessionUsageTurn(
  taskId: string,
  sessionId: string,
  turnId: string,
  options?: ApiRequestOptions,
): Promise<UsageTurn> {
  return fetchJson<UsageTurn>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/sessions/${encodeURIComponent(sessionId)}/usage/turns/${encodeURIComponent(turnId)}`,
    options,
  );
}
