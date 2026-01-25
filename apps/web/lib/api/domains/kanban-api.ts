import { fetchJson, type ApiRequestOptions } from '../client';
import { getBackendConfig } from '@/lib/config';
import type {
  BoardSnapshot,
  ListBoardsResponse,
  CreateTaskResponse,
  Task,
} from '@/lib/types/http';

// Board operations
export async function listBoards(workspaceId: string, options?: ApiRequestOptions) {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/boards`);
  url.searchParams.set('workspace_id', workspaceId);
  return fetchJson<ListBoardsResponse>(url.toString(), options);
}

export async function fetchBoardSnapshot(boardId: string, options?: ApiRequestOptions) {
  return fetchJson<BoardSnapshot>(`/api/v1/boards/${boardId}/snapshot`, options);
}

// Task operations
export async function createTask(
  payload: {
    workspace_id: string;
    board_id: string;
    column_id: string;
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
    agent_profile_id?: string;
    executor_id?: string;
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
  payload: { board_id: string; column_id: string; position: number },
  options?: ApiRequestOptions
) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}/move`, {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function fetchTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}`, options);
}
