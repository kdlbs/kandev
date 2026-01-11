import type {
  BoardSnapshot,
  ListBoardsResponse,
  ListRepositoriesResponse,
  ListWorkspacesResponse,
  RepositoryBranchesResponse,
  Task,
  Workspace,
} from '@/lib/types/http';

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

export async function listRepositories(baseUrl: string, workspaceId: string) {
  return fetchJson<ListRepositoriesResponse>(`${baseUrl}/api/v1/workspaces/${workspaceId}/repositories`);
}

export async function listRepositoryBranches(baseUrl: string, repositoryId: string) {
  return fetchJson<RepositoryBranchesResponse>(`${baseUrl}/api/v1/repositories/${repositoryId}/branches`);
}

export async function createTask(
  baseUrl: string,
  payload: {
    workspace_id: string;
    board_id: string;
    column_id: string;
    title: string;
    description?: string;
    position?: number;
    repository_url?: string;
    branch?: string;
    agent_type?: string;
    state?: Task['state'];
  }
) {
  return fetchJson<Task>(`${baseUrl}/api/v1/tasks`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateTask(
  baseUrl: string,
  taskId: string,
  payload: {
    title?: string;
    description?: string;
    position?: number;
    agent_type?: string;
    assigned_to?: string;
    state?: Task['state'];
  }
) {
  return fetchJson<Task>(`${baseUrl}/api/v1/tasks/${taskId}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteTask(baseUrl: string, taskId: string) {
  return fetchJson<void>(`${baseUrl}/api/v1/tasks/${taskId}`, {
    method: 'DELETE',
  });
}

export async function moveTask(
  baseUrl: string,
  taskId: string,
  payload: { board_id: string; column_id: string; position: number }
) {
  return fetchJson<Task>(`${baseUrl}/api/v1/tasks/${taskId}/move`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}
