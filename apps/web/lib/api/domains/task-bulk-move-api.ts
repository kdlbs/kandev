import { ApiError, fetchJson, type ApiRequestOptions } from "../client";

export async function bulkMoveSelectedTasks(
  payload: { task_ids: string[]; target_workflow_id: string; target_step_id: string },
  options?: ApiRequestOptions,
) {
  try {
    return await fetchJson<BulkMoveSelectedTasksResponse>("/api/v1/tasks/bulk-move", {
      ...options,
      init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
    });
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      const result = parseBulkMoveErrorResponse(error.body);
      if (result) throw new BulkMoveApiError(error, result);
    }
    throw error;
  }
}

export type BulkMoveSelectedTasksResponse = {
  moved_count: number;
  moved_task_ids: string[];
};

export type BulkMoveTaskRefusal = {
  task_id: string;
  task_title: string;
  code: string;
  message: string;
  pending_approvers?: Array<{ agent_profile_id: string; name: string }>;
};

export type BulkMoveErrorResponse = {
  code: string;
  moved_task_ids: string[];
  refusals: BulkMoveTaskRefusal[];
};

export class BulkMoveApiError extends ApiError {
  readonly result: BulkMoveErrorResponse;

  constructor(error: ApiError, result: BulkMoveErrorResponse) {
    super(error.message, error.status, result, error.retryAfterSeconds);
    this.result = result;
  }
}

export function getBulkMoveApiError(error: unknown): BulkMoveApiError | null {
  return error instanceof BulkMoveApiError ? error : null;
}

function parseBulkMoveErrorResponse(value: unknown): BulkMoveErrorResponse | null {
  if (!value || typeof value !== "object") return null;
  const body = value as Record<string, unknown>;
  if (
    typeof body.code !== "string" ||
    !isStringArray(body.moved_task_ids) ||
    !Array.isArray(body.refusals)
  ) {
    return null;
  }
  const refusals = body.refusals.map(parseBulkMoveTaskRefusal);
  if (refusals.some((refusal) => refusal === null)) return null;
  return {
    code: body.code,
    moved_task_ids: body.moved_task_ids,
    refusals: refusals as BulkMoveTaskRefusal[],
  };
}

function parseBulkMoveTaskRefusal(value: unknown): BulkMoveTaskRefusal | null {
  if (!value || typeof value !== "object") return null;
  const refusal = value as Record<string, unknown>;
  if (
    typeof refusal.task_id !== "string" ||
    typeof refusal.task_title !== "string" ||
    typeof refusal.code !== "string" ||
    typeof refusal.message !== "string"
  ) {
    return null;
  }
  const pendingApprovers = refusal.pending_approvers;
  if (
    pendingApprovers !== undefined &&
    (!Array.isArray(pendingApprovers) ||
      pendingApprovers.some(
        (approver) =>
          !approver ||
          typeof approver !== "object" ||
          typeof (approver as Record<string, unknown>).agent_profile_id !== "string" ||
          typeof (approver as Record<string, unknown>).name !== "string",
      ))
  ) {
    return null;
  }
  return {
    task_id: refusal.task_id,
    task_title: refusal.task_title,
    code: refusal.code,
    message: refusal.message,
    ...(pendingApprovers
      ? { pending_approvers: pendingApprovers as BulkMoveTaskRefusal["pending_approvers"] }
      : {}),
  };
}

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}
