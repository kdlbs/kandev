'use server';

import { getBackendConfig } from '@/lib/config';
import type {
  Board,
  Column,
  ListBoardsResponse,
  ListColumnsResponse,
  ListWorkspacesResponse,
  Workspace,
} from '@/lib/types/http';

const { apiBaseUrl } = getBackendConfig();

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...(options?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText}`);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

export async function listWorkspacesAction(): Promise<ListWorkspacesResponse> {
  return fetchJson<ListWorkspacesResponse>(`${apiBaseUrl}/api/v1/workspaces`);
}

export async function getWorkspaceAction(id: string): Promise<Workspace> {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces/${id}`);
}

export async function createWorkspaceAction(payload: { name: string; description?: string }) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateWorkspaceAction(id: string, payload: { name?: string; description?: string }) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteWorkspaceAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/workspaces/${id}`, { method: 'DELETE' });
}

export async function listBoardsAction(workspaceId: string): Promise<ListBoardsResponse> {
  const url = new URL(`${apiBaseUrl}/api/v1/boards`);
  url.searchParams.set('workspace_id', workspaceId);
  return fetchJson<ListBoardsResponse>(url.toString());
}

export async function createBoardAction(payload: { workspace_id: string; name: string; description?: string }) {
  return fetchJson<Board>(`${apiBaseUrl}/api/v1/boards`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateBoardAction(id: string, payload: { name?: string; description?: string }) {
  return fetchJson<Board>(`${apiBaseUrl}/api/v1/boards/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteBoardAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/boards/${id}`, { method: 'DELETE' });
}

export async function listColumnsAction(boardId: string): Promise<ListColumnsResponse> {
  return fetchJson<ListColumnsResponse>(`${apiBaseUrl}/api/v1/boards/${boardId}/columns`);
}

export async function createColumnAction(payload: {
  board_id: string;
  name: string;
  position: number;
  state: string;
  color: string;
}) {
  return fetchJson<Column>(`${apiBaseUrl}/api/v1/boards/${payload.board_id}/columns`, {
    method: 'POST',
    body: JSON.stringify({
      name: payload.name,
      position: payload.position,
      state: payload.state,
      color: payload.color,
    }),
  });
}

export async function updateColumnAction(id: string, payload: Partial<Pick<Column, 'name' | 'position' | 'state' | 'color'>>) {
  return fetchJson<Column>(`${apiBaseUrl}/api/v1/columns/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export async function deleteColumnAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/columns/${id}`, { method: 'DELETE' });
}
