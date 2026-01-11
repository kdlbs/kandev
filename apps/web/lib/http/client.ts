import type { BoardSnapshot, ListBoardsResponse, ListWorkspacesResponse, Workspace } from '@/lib/types/http';

async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(options?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText}`);
  }
  return (await response.json()) as T;
}

export async function createWorkspace(baseUrl: string, payload: { name: string; description?: string }) {
  return fetchJson<Workspace>(`${baseUrl}/api/v1/workspaces`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function listWorkspaces(baseUrl: string) {
  return fetchJson<ListWorkspacesResponse>(`${baseUrl}/api/v1/workspaces`);
}

export async function listBoards(baseUrl: string, workspaceId: string) {
  const url = new URL(`${baseUrl}/api/v1/boards`);
  url.searchParams.set('workspace_id', workspaceId);
  return fetchJson<ListBoardsResponse>(url.toString());
}

export async function fetchBoardSnapshot(baseUrl: string, boardId: string) {
  return fetchJson<BoardSnapshot>(`${baseUrl}/api/v1/boards/${boardId}/snapshot`);
}
