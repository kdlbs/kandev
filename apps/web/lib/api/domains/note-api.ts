import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskNote } from "@/lib/types/http";

const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

export async function getTaskNote(taskId: string): Promise<TaskNote | null> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  const response = await client.request("task.note.get", { task_id: taskId });
  if (!response || Object.keys(response).length === 0) return null;
  return response as TaskNote | null;
}

export async function updateTaskNote(
  taskId: string,
  content: string,
  updatedBy?: TaskNote["updated_by"],
): Promise<TaskNote> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  const payload: Record<string, string> = {
    task_id: taskId,
    content,
  };
  if (updatedBy) payload.updated_by = updatedBy;
  const response = await client.request("task.note.update", payload);
  return response as TaskNote;
}

export async function deleteTaskNote(taskId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  await client.request("task.note.delete", { task_id: taskId });
}
