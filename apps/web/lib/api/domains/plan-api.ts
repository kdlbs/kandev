import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskPlan, TaskPlanRevision } from "@/lib/types/http";

const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

/**
 * Get the task plan for a specific task.
 * Returns null if no plan exists.
 */
export async function getTaskPlan(taskId: string): Promise<TaskPlan | null> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const response = await client.request("task.plan.get", { task_id: taskId });

  if (!response || Object.keys(response).length === 0) {
    return null;
  }

  return response as TaskPlan;
}

/**
 * Create a new task plan.
 */
export async function createTaskPlan(
  taskId: string,
  content: string,
  title?: string,
): Promise<TaskPlan> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const response = await client.request("task.plan.create", {
    task_id: taskId,
    title: title || "Plan",
    content,
    created_by: "user",
  });

  return response as TaskPlan;
}

/**
 * Update an existing task plan.
 */
export async function updateTaskPlan(
  taskId: string,
  content: string,
  title?: string,
): Promise<TaskPlan> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const payload: Record<string, string> = {
    task_id: taskId,
    content,
    created_by: "user",
  };

  if (title !== undefined) {
    payload.title = title;
  }

  const response = await client.request("task.plan.update", payload);

  return response as TaskPlan;
}

/**
 * Delete a task plan.
 */
export async function deleteTaskPlan(taskId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  await client.request("task.plan.delete", { task_id: taskId });
}

/**
 * List all revisions for a plan (metadata only, newest first).
 */
export async function listPlanRevisions(taskId: string): Promise<TaskPlanRevision[]> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const response = await client.request("task.plan.revisions.list", { task_id: taskId });
  const list = (response as { revisions?: TaskPlanRevision[] })?.revisions;
  return list ?? [];
}

/**
 * Fetch a single revision including its content (for diff/preview).
 * `taskId` is optional but recommended — the backend uses it to enforce
 * ownership (a connected client can only read revisions belonging to a
 * task it already holds a reference to).
 */
export async function getPlanRevision(
  revisionId: string,
  taskId?: string,
): Promise<TaskPlanRevision> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const payload: Record<string, string> = { revision_id: revisionId };
  if (taskId) payload.task_id = taskId;
  const response = await client.request("task.plan.revision.get", payload);
  return response as TaskPlanRevision;
}

/**
 * Revert the plan to a previous revision. Server appends a new revision with the target's content.
 */
export async function revertPlanRevision(
  taskId: string,
  revisionId: string,
  authorName?: string,
): Promise<TaskPlanRevision> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error(WS_CLIENT_UNAVAILABLE);
  }
  const payload: Record<string, string> = {
    task_id: taskId,
    revision_id: revisionId,
  };
  if (authorName) payload.author_name = authorName;
  const response = await client.request("task.plan.revert", payload);
  return response as TaskPlanRevision;
}
