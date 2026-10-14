import { fetchJson, type ApiRequestOptions } from "../client";

export type TaskManagementClaim = {
  task_id: string;
  workspace_id: string;
  owner_kind: "plugin" | "human" | "";
  owner_actor_id?: string;
  installation_id?: string;
  instance_key?: string;
  generation: number;
  resource_version: string;
  acquired_at?: string;
  updated_at: string;
  updated_by_actor?: string;
};

export type TaskManagementClaimHistory = {
  id: string;
  task_id: string;
  workspace_id: string;
  action: string;
  previous_owner_kind?: string;
  previous_owner_actor_id?: string;
  previous_installation_id?: string;
  previous_instance_key?: string;
  installation_id?: string;
  instance_key?: string;
  owner_kind?: string;
  owner_actor_id?: string;
  generation: number;
  actor_id: string;
  reason: string;
  resource_version: string;
  created_at: string;
};

export type TaskManagementClaimSnapshot = {
  claim: TaskManagementClaim | null;
  history: TaskManagementClaimHistory[];
};

export type ChangeTaskManagementClaimRequest = {
  action: "transfer" | "release";
  expected_task_resource_version: string;
  expected_claim_resource_version: string;
  reason: string;
};

export function getTaskManagementClaim(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskManagementClaimSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/management-claim`,
    options,
  );
}

export function changeTaskManagementClaim(
  taskId: string,
  request: ChangeTaskManagementClaimRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskManagementClaim>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/management-claim`,
    {
      ...options,
      init: {
        method: "POST",
        body: JSON.stringify(request),
        ...(options?.init ?? {}),
      },
    },
  );
}
