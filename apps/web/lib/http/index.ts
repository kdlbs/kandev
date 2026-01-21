import { getBackendConfig } from '@/lib/config';
import type {
  BoardSnapshot,
  ListAgentsResponse,
  ListAgentDiscoveryResponse,
  ListAvailableAgentsResponse,
  AgentProfileMcpConfig,
  ListBoardsResponse,
  ListEnvironmentsResponse,
  ListExecutorsResponse,
  ListRepositoriesResponse,
  ListWorkspacesResponse,
  ListMessagesResponse,
  RepositoryBranchesResponse,
  NotificationProvidersResponse,
  NotificationProvider,
  EditorsResponse,
  EditorOption,
  CustomPrompt,
  PromptsResponse,
  CreateTaskResponse,
  Task,
  TaskSessionsResponse,
  TaskSessionResponse,
  UserSettingsResponse,
  Workspace,
} from '@/lib/types/http';

export type ApiRequestOptions = {
  baseUrl?: string;
  cache?: RequestCache;
  init?: RequestInit;
};

function resolveUrl(pathOrUrl: string, baseUrl: string) {
  if (pathOrUrl.startsWith('http://') || pathOrUrl.startsWith('https://')) {
    return pathOrUrl;
  }
  return `${baseUrl}${pathOrUrl}`;
}

async function fetchJson<T>(pathOrUrl: string, options?: ApiRequestOptions): Promise<T> {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = resolveUrl(pathOrUrl, baseUrl);
  const response = await fetch(url, {
    ...options?.init,
    cache: options?.cache,
    headers: {
      'Content-Type': 'application/json',
      ...(options?.init?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText} (${url})`);
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

export async function createWorkspace(
  payload: { name: string; description?: string },
  options?: ApiRequestOptions
) {
  return fetchJson<Workspace>('/api/v1/workspaces', {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function listWorkspaces(options?: ApiRequestOptions) {
  return fetchJson<ListWorkspacesResponse>('/api/v1/workspaces', options);
}

export async function listBoards(workspaceId: string, options?: ApiRequestOptions) {
  const baseUrl = options?.baseUrl ?? getBackendConfig().apiBaseUrl;
  const url = new URL(`${baseUrl}/api/v1/boards`);
  url.searchParams.set('workspace_id', workspaceId);
  return fetchJson<ListBoardsResponse>(url.toString(), options);
}

export async function fetchBoardSnapshot(boardId: string, options?: ApiRequestOptions) {
  return fetchJson<BoardSnapshot>(`/api/v1/boards/${boardId}/snapshot`, options);
}

export async function listRepositories(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<ListRepositoriesResponse>(`/api/v1/workspaces/${workspaceId}/repositories`, options);
}

export async function listRepositoryBranches(repositoryId: string, options?: ApiRequestOptions) {
  return fetchJson<RepositoryBranchesResponse>(`/api/v1/repositories/${repositoryId}/branches`, options);
}

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

export async function fetchUserSettings(options?: ApiRequestOptions) {
  return fetchJson<UserSettingsResponse>('/api/v1/user/settings', options);
}

export async function updateUserSettings(
  payload: {
    workspace_id: string;
    board_id: string;
    repository_ids: string[];
    preferred_shell?: string;
    default_editor_id?: string;
    enable_preview_on_click?: boolean;
  },
  options?: ApiRequestOptions
) {
  return fetchJson<UserSettingsResponse>('/api/v1/user/settings', {
    ...options,
    init: { method: 'PATCH', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function listEditors(options?: ApiRequestOptions) {
  return fetchJson<EditorsResponse>('/api/v1/editors', options);
}

export async function createEditor(
  payload: {
    name: string;
    kind: string;
    config?: Record<string, unknown>;
    enabled?: boolean;
  },
  options?: ApiRequestOptions
) {
  return fetchJson<EditorOption>('/api/v1/editors', {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateEditor(
  editorId: string,
  payload: {
    name?: string;
    kind?: string;
    config?: Record<string, unknown>;
    enabled?: boolean;
  },
  options?: ApiRequestOptions
) {
  return fetchJson<EditorOption>(`/api/v1/editors/${editorId}`, {
    ...options,
    init: { method: 'PATCH', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteEditor(editorId: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/editors/${editorId}`, {
    ...options,
    init: { method: 'DELETE', ...(options?.init ?? {}) },
  });
}

export async function listPrompts(options?: ApiRequestOptions) {
  return fetchJson<PromptsResponse>('/api/v1/prompts', options);
}

export async function createPrompt(
  payload: { name: string; content: string },
  options?: ApiRequestOptions
) {
  return fetchJson<CustomPrompt>('/api/v1/prompts', {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updatePrompt(
  promptId: string,
  payload: { name?: string; content?: string },
  options?: ApiRequestOptions
) {
  return fetchJson<CustomPrompt>(`/api/v1/prompts/${promptId}`, {
    ...options,
    init: { method: 'PATCH', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deletePrompt(promptId: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/prompts/${promptId}`, {
    ...options,
    init: { method: 'DELETE', ...(options?.init ?? {}) },
  });
}

export async function openSessionInEditor(
  sessionId: string,
  payload: Partial<{ editor_id: string; editor_type: string; file_path: string; line: number; column: number }>,
  options?: ApiRequestOptions
) {
  return fetchJson<{ url?: string }>(`/api/v1/task-sessions/${sessionId}/open-editor`, {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function listNotificationProviders(options?: ApiRequestOptions) {
  return fetchJson<NotificationProvidersResponse>('/api/v1/notification-providers', options);
}

export async function createNotificationProvider(
  payload: {
    name: string;
    type: NotificationProvider['type'];
    config?: NotificationProvider['config'];
    enabled?: boolean;
    events?: string[];
  },
  options?: ApiRequestOptions
) {
  return fetchJson<NotificationProvider>('/api/v1/notification-providers', {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateNotificationProvider(
  providerId: string,
  payload: Partial<{
    name: string;
    type: NotificationProvider['type'];
    config: NotificationProvider['config'];
    enabled: boolean;
    events: string[];
  }>,
  options?: ApiRequestOptions
) {
  return fetchJson<NotificationProvider>(`/api/v1/notification-providers/${providerId}`, {
    ...options,
    init: { method: 'PATCH', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteNotificationProvider(providerId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`/api/v1/notification-providers/${providerId}`, {
    ...options,
    init: { method: 'DELETE', ...(options?.init ?? {}) },
  });
}

export async function fetchTask(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<Task>(`/api/v1/tasks/${taskId}`, options);
}

export async function listTaskSessions(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskSessionsResponse>(`/api/v1/tasks/${taskId}/sessions`, options);
}

export async function fetchTaskSession(taskSessionId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskSessionResponse>(`/api/v1/task-sessions/${taskSessionId}`, options);
}

export async function listTaskSessionMessages(
  taskSessionId: string,
  params?: { limit?: number; before?: string; after?: string; sort?: 'asc' | 'desc' },
  options?: ApiRequestOptions
) {
  const query = new URLSearchParams();
  if (params?.limit) query.set('limit', params.limit.toString());
  if (params?.before) query.set('before', params.before);
  if (params?.after) query.set('after', params.after);
  if (params?.sort) query.set('sort', params.sort);
  const suffix = query.toString();
  const url = `/api/v1/task-sessions/${taskSessionId}/messages${suffix ? `?${suffix}` : ''}`;
  return fetchJson<ListMessagesResponse>(url, options);
}

export async function listExecutors(options?: ApiRequestOptions): Promise<ListExecutorsResponse> {
  return fetchJson<ListExecutorsResponse>('/api/v1/executors', options);
}

export async function listEnvironments(options?: ApiRequestOptions): Promise<ListEnvironmentsResponse> {
  return fetchJson<ListEnvironmentsResponse>('/api/v1/environments', options);
}

export async function listAgents(options?: ApiRequestOptions): Promise<ListAgentsResponse> {
  return fetchJson<ListAgentsResponse>('/api/v1/agents', options);
}

export async function listAgentDiscovery(
  options?: ApiRequestOptions
): Promise<ListAgentDiscoveryResponse> {
  return fetchJson<ListAgentDiscoveryResponse>('/api/v1/agents/discovery', options);
}

export async function listAvailableAgents(
  options?: ApiRequestOptions
): Promise<ListAvailableAgentsResponse> {
  return fetchJson<ListAvailableAgentsResponse>('/api/v1/agents/available', options);
}

export async function getAgentProfileMcpConfig(
  profileId: string,
  options?: ApiRequestOptions
): Promise<AgentProfileMcpConfig> {
  return fetchJson<AgentProfileMcpConfig>(`/api/v1/agent-profiles/${profileId}/mcp-config`, options);
}
