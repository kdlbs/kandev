import { fetchJson, type ApiRequestOptions } from '../client';
import { getBackendConfig } from '@/lib/config';
import type {
  WorkflowSnapshot,
  ListWorkflowsResponse,
  ListTasksResponse,
  CreateTaskResponse,
  Task,
  MoveTaskResponse,
} from '@/lib/types/http';

// Workflow operations
export async function listWorkflows(workspaceId: string, options?: ApiRequestOptions) {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/workflows`);
  url.searchParams.set('workspace_id', workspaceId);
  return fetchJson<ListWorkflowsResponse>(url.toString(), options);
}

export async function fetchWorkflowSnapshot(workflowId: string, options?: ApiRequestOptions) {
  return fetchJson<WorkflowSnapshot>(`/api/v1/workflows/${workflowId}/snapshot`, options);
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
      local_path?: string;
      name?: string;
      default_branch?: string;
    }>;
    state?: Task['state'];
    start_agent?: boolean;
    prepare_session?: boolean;
    agent_profile_id?: string;
    executor_id?: string;
    plan_mode?: boolean;
  },
  options?: ApiRequestOptions
) {
  return fetchJson<CreateTaskResponse>('/api/v1/tasks', {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateTask(
  taskId: string,
  payload: {
    title?: string;
    description?: string;
    position?: number;
    state?: Task['state'];
    repositories?: Array<{
      repository_id: string;
      base_branch?: string;
    }>;
  },
  options?: ApiRequestOptions
) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}`, {
    ...options,
    init: { method: 'PATCH', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`/api/v1/tasks/${taskId}`, {
    ...options,
    init: { method: 'DELETE', ...(options?.init ?? {}) },
  });
}

export async function moveTask(
  taskId: string,
  payload: { workflow_id: string; workflow_step_id: string; position: number },
  options?: ApiRequestOptions
) {
  return fetchJson<MoveTaskResponse>(`/api/v1/tasks/${taskId}/move`, {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function fetchTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}`, options);
}

export async function listTasksByWorkspace(
  workspaceId: string,
  params: { page?: number; pageSize?: number; query?: string } = {},
  options?: ApiRequestOptions
) {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/workspaces/${workspaceId}/tasks`);
  if (params.page) url.searchParams.set('page', String(params.page));
  if (params.pageSize) url.searchParams.set('page_size', String(params.pageSize));
  if (params.query) url.searchParams.set('query', params.query);
  return fetchJson<ListTasksResponse>(url.toString(), options);
}
