import { fetchJson, type ApiRequestOptions } from "../client";

export type TaskEnvironment = {
  id: string;
  task_id: string;
  repository_id: string;
  executor_type: string;
  executor_id: string;
  executor_profile_id: string;
  agent_execution_id: string;
  control_port: number;
  status: string;
  worktree_id?: string;
  worktree_path?: string;
  worktree_branch?: string;
  workspace_path?: string;
  container_id?: string;
  sandbox_id?: string;
  created_at: string;
  updated_at?: string;
};

export type ResetTaskEnvironmentPayload = {
  push_branch?: boolean;
};

export async function fetchTaskEnvironment(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskEnvironment>(`/api/v1/tasks/${taskId}/environment`, options);
}

export async function resetTaskEnvironment(
  taskId: string,
  payload: ResetTaskEnvironmentPayload,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/tasks/${taskId}/environment/reset`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify(payload),
      ...(options?.init ?? {}),
    },
  });
}
