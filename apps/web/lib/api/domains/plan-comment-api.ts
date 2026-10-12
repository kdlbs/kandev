import { getWebSocketClient } from "@/lib/ws/connection";
import type { TaskPlanCommentSnapshot } from "@/lib/types/http";

// i18n-exempt: programmer precondition; UI callers render localized errors.
const WS_CLIENT_UNAVAILABLE = "WebSocket client not available";

type CreatePlanCommentInput = {
  taskId: string;
  planId: string;
  id: string;
  body: string;
  selectedText: string;
  anchorFrom: number;
  anchorTo: number;
};

type MutatePlanCommentInput = {
  taskId: string;
  planId: string;
  id: string;
  expectedVersion: number;
};

type UpdatePlanCommentInput = MutatePlanCommentInput & { body: string };

function requireClient() {
  const client = getWebSocketClient();
  if (!client) throw new Error(WS_CLIENT_UNAVAILABLE);
  return client;
}

export async function getTaskPlanComments(taskId: string): Promise<TaskPlanCommentSnapshot> {
  return (await requireClient().request("task.plan.comments.list", {
    task_id: taskId,
  })) as TaskPlanCommentSnapshot;
}

export async function createTaskPlanComment(
  input: CreatePlanCommentInput,
): Promise<TaskPlanCommentSnapshot> {
  return (await requireClient().request("task.plan.comments.create", {
    task_id: input.taskId,
    plan_id: input.planId,
    id: input.id,
    body: input.body,
    selected_text: input.selectedText,
    anchor_from: input.anchorFrom,
    anchor_to: input.anchorTo,
  })) as TaskPlanCommentSnapshot;
}

export async function updateTaskPlanComment(
  input: UpdatePlanCommentInput,
): Promise<TaskPlanCommentSnapshot> {
  return (await requireClient().request("task.plan.comments.update", {
    task_id: input.taskId,
    plan_id: input.planId,
    id: input.id,
    body: input.body,
    expected_version: input.expectedVersion,
  })) as TaskPlanCommentSnapshot;
}

export async function deleteTaskPlanComment(
  input: MutatePlanCommentInput,
): Promise<TaskPlanCommentSnapshot> {
  return (await requireClient().request("task.plan.comments.delete", {
    task_id: input.taskId,
    plan_id: input.planId,
    id: input.id,
    expected_version: input.expectedVersion,
  })) as TaskPlanCommentSnapshot;
}
