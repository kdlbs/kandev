import { fetchJson, type ApiRequestOptions } from "../client";
import { getBackendConfig } from "@/lib/config";
import type {
  WorkflowSnapshot,
  ListWorkflowsResponse,
  ListTasksResponse,
  CreateTaskResponse,
  Task,
  MoveTaskResponse,
} from "@/lib/types/http";

// Workflow operations
export async function listWorkflows(
  workspaceId: string,
  options?: ApiRequestOptions & { includeHidden?: boolean },
) {
  const { includeHidden, ...requestOptions } = options ?? {};
  const baseUrl = requestOptions.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/workflows`);
  url.searchParams.set("workspace_id", workspaceId);
  if (includeHidden) {
    url.searchParams.set("include_hidden", "true");
  }
  return fetchJson<ListWorkflowsResponse>(url.toString(), requestOptions);
}

export async function fetchWorkflowSnapshot(workflowId: string, options?: ApiRequestOptions) {
  return fetchJson<WorkflowSnapshot>(`/api/v1/workflows/${workflowId}/snapshot`, options);
}

export async function reorderWorkflows(
  workspaceId: string,
  workflowIds: string[],
  options?: ApiRequestOptions,
) {
  return fetchJson<{ success: boolean }>(`/api/v1/workspaces/${workspaceId}/workflows/reorder`, {
    ...options,
    init: {
      method: "PUT",
      body: JSON.stringify({ workflow_ids: workflowIds }),
      ...(options?.init ?? {}),
    },
  });
}

// Task operations
export async function createTask(
  payload: {
    workspace_id: string;
    workflow_id: string;
    workflow_step_id?: string;
    title: string;
    description?: string;
    position?: number;
    repositories?: Array<{
      repository_id: string;
      base_branch?: string;
      checkout_branch?: string;
      pr_number?: number;
      local_path?: string;
      name?: string;
      default_branch?: string;
      github_url?: string;
      remote_url?: string;
      provider?: "github" | "gitlab" | "azure_devops";
      provider_repo_id?: string;
      provider_owner?: string;
      provider_name?: string;
      fresh_branch?: boolean;
      confirm_discard?: boolean;
      consented_dirty_files?: string[];
    }>;
    state?: Task["state"];
    start_agent?: boolean;
    prepare_session?: boolean;
    agent_profile_id?: string;
    executor_id?: string;
    executor_profile_id?: string;
    plan_mode?: boolean;
    attachments?: Array<{
      type: string;
      data: string;
      mime_type: string;
      name?: string;
      delivery_mode?: "prompt" | "path";
    }>;
    parent_id?: string;
    workspace_path?: string;
    priority?: string;
    project_id?: string;
    metadata?: Record<string, unknown>;
    /** Office task-handoffs phase 4/5 — workspace policy. */
    workspace_mode?: "inherit_parent" | "new_workspace" | "shared_group";
    workspace_group_id?: string;
    default_child_workspace?: "inherit_parent" | "new_workspace";
    default_child_ordering?: "sequential" | "parallel";
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<CreateTaskResponse>("/api/v1/tasks", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateTask(
  taskId: string,
  payload: {
    title?: string;
    description?: string;
    position?: number;
    state?: Task["state"];
    repositories?: Array<{
      repository_id: string;
      base_branch?: string;
    }>;
    /** Nest under another task. Empty string clears the parent (un-nest). */
    parent_id?: string;
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function detachTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}/detach`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function updateTaskRepositoryBaseBranch(
  taskId: string,
  taskRepositoryId: string,
  baseBranch: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<{
    id: string;
    task_id: string;
    repository_id: string;
    base_branch: string;
    checkout_branch?: string;
    position?: number;
  }>(`/api/v1/tasks/${taskId}/repositories/${taskRepositoryId}`, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PATCH",
      body: JSON.stringify({ base_branch: baseBranch }),
    },
  });
}

export async function deleteTask(
  taskId: string,
  params?: { cascade?: boolean },
  options?: ApiRequestOptions,
) {
  const query = params?.cascade ? "?cascade=true" : "";
  return fetchJson<void>(`/api/v1/tasks/${taskId}${query}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

export async function moveTask(
  taskId: string,
  payload: { workflow_id: string; workflow_step_id: string; position: number },
  options?: ApiRequestOptions,
) {
  return fetchJson<MoveTaskResponse>(`/api/v1/tasks/${taskId}/move`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function bulkMoveSelectedTasks(
  payload: { task_ids: string[]; target_workflow_id: string; target_step_id: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<{ moved_count: number }>("/api/v1/tasks/bulk-move", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function fetchTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}`, options);
}

export async function archiveTask(
  taskId: string,
  params?: { cascade?: boolean },
  options?: ApiRequestOptions,
) {
  const query = params?.cascade ? "?cascade=true" : "";
  return fetchJson<void>(`/api/v1/tasks/${taskId}/archive${query}`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export type BranchRecovery = {
  task_id: string;
  repository_id: string;
  branch: string;
  status: "local" | "remote" | "missing";
};

export type UnarchiveTaskResponse = {
  success: boolean;
  cascade_id: string;
  unarchived_ids: string[];
  skipped_ids: string[];
  affected_group_ids: string[];
  recovery: BranchRecovery[];
};

export async function unarchiveTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<UnarchiveTaskResponse>(`/api/v1/tasks/${taskId}/unarchive`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function getSubtaskCount(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<{ count: number }>(`/api/v1/tasks/${taskId}/subtask-count`, options);
}

export async function listTasksByWorkspace(
  workspaceId: string,
  params: {
    page?: number;
    pageSize?: number;
    query?: string;
    includeArchived?: boolean;
    workflowId?: string | null;
    repositoryId?: string | null;
    sort?: string;
  } = {},
  options?: ApiRequestOptions,
) {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/workspaces/${workspaceId}/tasks`);
  if (params.page) url.searchParams.set("page", String(params.page));
  if (params.pageSize) url.searchParams.set("page_size", String(params.pageSize));
  if (params.query) url.searchParams.set("query", params.query);
  if (params.includeArchived) url.searchParams.set("include_archived", "true");
  if (params.workflowId) url.searchParams.set("workflow_id", params.workflowId);
  if (params.repositoryId) url.searchParams.set("repository_id", params.repositoryId);
  if (params.sort) url.searchParams.set("sort", params.sort);
  return fetchJson<ListTasksResponse>(url.toString(), options);
}
