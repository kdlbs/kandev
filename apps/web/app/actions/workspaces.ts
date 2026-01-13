'use server';

import { getBackendConfig } from '@/lib/config';
import type {
  Board,
  Column,
  ListBoardsResponse,
  ListColumnsResponse,
  RepositoryDiscoveryResponse,
  ListRepositoriesResponse,
  ListRepositoryScriptsResponse,
  ListWorkspacesResponse,
  RepositoryPathValidationResponse,
  Repository,
  RepositoryScript,
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

export async function createWorkspaceAction(payload: {
  name: string;
  description?: string;
  default_executor_id?: string;
  default_environment_id?: string;
  default_agent_profile_id?: string;
}) {
  return fetchJson<Workspace>(`${apiBaseUrl}/api/v1/workspaces`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateWorkspaceAction(
  id: string,
  payload: {
    name?: string;
    description?: string;
    default_executor_id?: string;
    default_environment_id?: string;
    default_agent_profile_id?: string;
  }
) {
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

export async function listRepositoriesAction(workspaceId: string): Promise<ListRepositoriesResponse> {
  return fetchJson<ListRepositoriesResponse>(`${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories`);
}

export async function discoverRepositoriesAction(
  workspaceId: string,
  root?: string
): Promise<RepositoryDiscoveryResponse> {
  const params = root ? `?root=${encodeURIComponent(root)}` : '';
  return fetchJson<RepositoryDiscoveryResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/discover${params}`
  );
}

export async function validateRepositoryPathAction(
  workspaceId: string,
  path: string
): Promise<RepositoryPathValidationResponse> {
  const params = `?path=${encodeURIComponent(path)}`;
  return fetchJson<RepositoryPathValidationResponse>(
    `${apiBaseUrl}/api/v1/workspaces/${workspaceId}/repositories/validate${params}`
  );
}

export async function createRepositoryAction(payload: {
  workspace_id: string;
  name: string;
  source_type: string;
  local_path: string;
  provider: string;
  provider_repo_id: string;
  provider_owner: string;
  provider_name: string;
  default_branch: string;
  setup_script: string;
  cleanup_script: string;
}) {
  return fetchJson<Repository>(`${apiBaseUrl}/api/v1/workspaces/${payload.workspace_id}/repositories`, {
    method: 'POST',
    body: JSON.stringify({
      name: payload.name,
      source_type: payload.source_type,
      local_path: payload.local_path,
      provider: payload.provider,
      provider_repo_id: payload.provider_repo_id,
      provider_owner: payload.provider_owner,
      provider_name: payload.provider_name,
      default_branch: payload.default_branch,
      setup_script: payload.setup_script,
      cleanup_script: payload.cleanup_script,
    }),
  });
}

export async function updateRepositoryAction(id: string, payload: Partial<Repository>) {
  return fetchJson<Repository>(`${apiBaseUrl}/api/v1/repositories/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteRepositoryAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/repositories/${id}`, { method: 'DELETE' });
}

export async function listRepositoryScriptsAction(repositoryId: string): Promise<ListRepositoryScriptsResponse> {
  return fetchJson<ListRepositoryScriptsResponse>(`${apiBaseUrl}/api/v1/repositories/${repositoryId}/scripts`);
}

export async function createRepositoryScriptAction(payload: {
  repository_id: string;
  name: string;
  command: string;
  position: number;
}) {
  return fetchJson<RepositoryScript>(`${apiBaseUrl}/api/v1/repositories/${payload.repository_id}/scripts`, {
    method: 'POST',
    body: JSON.stringify({
      name: payload.name,
      command: payload.command,
      position: payload.position,
    }),
  });
}

export async function updateRepositoryScriptAction(id: string, payload: Partial<RepositoryScript>) {
  return fetchJson<RepositoryScript>(`${apiBaseUrl}/api/v1/scripts/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });
}

export async function deleteRepositoryScriptAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/scripts/${id}`, { method: 'DELETE' });
}
