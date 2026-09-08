import { getWebSocketClient } from "@/lib/ws/connection";
import type { Task } from "@/lib/types/http";

// i18n-exempt: diagnostic thrown to the caller, never rendered.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/**
 * Switches a task's executor profile before anything has materialized
 * (REQ-TASKS-RUNNER-SWITCH-002). Rejects with a `WebSocketRequestError`
 * carrying the outcome's `code` and, for a mutability or compatibility
 * conflict, `details.error_code` naming the reason.
 */
export async function switchTaskRunner(
  taskId: string,
  executorProfileId: string,
): Promise<Task> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const response = await client.request("task.runner", {
    id: taskId,
    executor_profile_id: executorProfileId,
  });
  return response as Task;
}
