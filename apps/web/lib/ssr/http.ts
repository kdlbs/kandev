import type {
  BoardSnapshot,
  ListBoardsResponse,
  ListWorkspacesResponse,
  Task,
  UserSettingsResponse,
} from '@/lib/types/http';
import { getBackendConfig } from '@/lib/config';

const { apiBaseUrl: API_BASE_URL } = getBackendConfig();

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText}`);
  }
  return (await response.json()) as T;
}

export async function fetchWorkspaces(): Promise<ListWorkspacesResponse> {
  return fetchJson<ListWorkspacesResponse>(`${API_BASE_URL}/api/v1/workspaces`);
}

export async function fetchBoards(workspaceId?: string): Promise<ListBoardsResponse> {
  const url = new URL(`${API_BASE_URL}/api/v1/boards`);
  if (workspaceId) {
    url.searchParams.set('workspace_id', workspaceId);
  }
  return fetchJson<ListBoardsResponse>(url.toString());
}

export async function fetchBoardSnapshot(boardId: string): Promise<BoardSnapshot> {
  return fetchJson<BoardSnapshot>(`${API_BASE_URL}/api/v1/boards/${boardId}/snapshot`);
}

export async function fetchTask(taskId: string): Promise<Task> {
  return fetchJson<Task>(`${API_BASE_URL}/api/v1/tasks/${taskId}`);
}

export async function fetchUserSettings(): Promise<UserSettingsResponse> {
  return fetchJson<UserSettingsResponse>(`${API_BASE_URL}/api/v1/user/settings`);
}
