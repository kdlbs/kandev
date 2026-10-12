import { fetchJson, type ApiRequestOptions } from "../client";

export type TaskUsageTotalsDTO = {
  scope: "task" | "session";
  scope_id: string;
  tokens_in: number;
  tokens_cached_read: number;
  tokens_cached_write: number;
  tokens_out: number;
  tokens_thought: number;
  tokens_total: number;
  cost_subcents: number;
  event_count: number;
  estimated_event_count: number;
  unpriced_event_count: number;
  output_tokens_complete: boolean;
  first_event_at: string | null;
  last_event_at: string | null;
};

export function fetchTaskUsageTotals(
  taskId: string,
  sessionId?: string | null,
  options?: ApiRequestOptions,
): Promise<TaskUsageTotalsDTO> {
  const taskPath = `/api/v1/tasks/${encodeURIComponent(taskId)}`;
  const path = sessionId
    ? `${taskPath}/sessions/${encodeURIComponent(sessionId)}/usage`
    : `${taskPath}/usage`;
  return fetchJson<TaskUsageTotalsDTO>(path, options);
}
