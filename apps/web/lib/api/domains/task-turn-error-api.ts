import { fetchJson, type ApiRequestOptions } from "../client";

export type DismissTaskTurnErrorResponse = {
  dismissed: boolean;
};

/**
 * Acknowledges exactly the observed task turn-error stamp. The inline Chat
 * alert and the task context/overflow menu share this operation. A stale stamp
 * returns 409 and preserves the newer attention.
 */
export async function dismissTaskTurnError(
  taskId: string,
  stamp: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<DismissTaskTurnErrorResponse>(`/api/v1/tasks/${taskId}/turn-error/dismiss`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify({ stamp }) },
  });
}
