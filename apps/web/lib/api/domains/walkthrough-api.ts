import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskWalkthrough } from "@/lib/types/http";

/**
 * Get the agent-authored code walkthrough for a task, or null if none exists.
 * Used to backfill the store on mount — live `task.walkthrough.created` events
 * can fire before the page's WS subscription is established.
 */
export async function getTaskWalkthrough(taskId: string): Promise<TaskWalkthrough | null> {
  const client = getWebSocketClient();
  if (!client) return null;

  const response = await client.request("task.walkthrough.get", { task_id: taskId });
  if (!response || typeof response !== "object" || !("id" in response)) {
    return null;
  }
  return response as TaskWalkthrough;
}
