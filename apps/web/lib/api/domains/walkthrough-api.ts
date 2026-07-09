import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskWalkthrough } from "@/lib/types/http";

const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/**
 * Get the agent-authored code walkthrough for a task, or null if none exists.
 * Used to backfill the store on mount — live `task.walkthrough.created` events
 * can fire before the page's WS subscription is established.
 */
export async function getTaskWalkthrough(taskId: string): Promise<TaskWalkthrough | null> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }

  const response = await client.request("task.walkthrough.get", { task_id: taskId });
  if (!response || typeof response !== "object" || !("id" in response)) {
    return null;
  }
  return response as TaskWalkthrough;
}

/** Delete the current persisted walkthrough for a task. */
export async function deleteTaskWalkthrough(taskId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }

  await client.request("task.walkthrough.delete", { task_id: taskId }, 10000);
}
