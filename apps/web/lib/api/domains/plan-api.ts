import { getWebSocketClient } from '@/lib/ws/connection';
import type { TaskPlan } from '@/lib/types/http';

/**
 * Get the task plan for a specific task.
 * Returns null if no plan exists.
 */
export async function getTaskPlan(taskId: string): Promise<TaskPlan | null> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error('WebSocket client not available');
  }
  const response = await client.request('task.plan.get', { task_id: taskId });

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
  title?: string
): Promise<TaskPlan> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error('WebSocket client not available');
  }
  const response = await client.request('task.plan.create', {
    task_id: taskId,
    title: title || 'Plan',
    content,
    created_by: 'user',
  });

  return response as TaskPlan;
}

/**
 * Update an existing task plan.
 */
export async function updateTaskPlan(
  taskId: string,
  content: string,
  title?: string
): Promise<TaskPlan> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error('WebSocket client not available');
  }
  const payload: Record<string, string> = {
    task_id: taskId,
    content,
    created_by: 'user',
  };

  if (title !== undefined) {
    payload.title = title;
  }

  const response = await client.request('task.plan.update', payload);

  return response as TaskPlan;
}

/**
 * Delete a task plan.
 */
export async function deleteTaskPlan(taskId: string): Promise<void> {
  const client = getWebSocketClient();
  if (!client) {
    throw new Error('WebSocket client not available');
  }
  await client.request('task.plan.delete', { task_id: taskId });
}

