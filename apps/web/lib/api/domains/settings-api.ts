import { fetchJson, type ApiRequestOptions } from '../client';
import type {
  ListAgentsResponse,
  ListAgentDiscoveryResponse,
  ListAvailableAgentsResponse,
  AgentProfileMcpConfig,
  ListEnvironmentsResponse,
  ListExecutorsResponse,
  NotificationProvidersResponse,
  NotificationProvider,
  EditorsResponse,
  EditorOption,
  CustomPrompt,
  PromptsResponse,
  UserSettingsResponse,
  DynamicModelsResponse,
} from '@/lib/types/http';

// User settings
export async function fetchUserSettings(options?: ApiRequestOptions) {
  return fetchJson<UserSettingsResponse>('/api/v1/user/settings', options);
}

export async function updateUserSettings(
  payload: {
    workspace_id?: string;
    board_id?: string;
    repository_ids?: string[];
    preferred_shell?: string;
    default_editor_id?: string;
    enable_preview_on_click?: boolean;
    chat_submit_key?: 'enter' | 'cmd_enter';
  },
  options?: ApiRequestOptions
) {
  return fetchJson<UserSettingsResponse>('/api/v1/user/settings', {
    ...options,
    init: { method: 'PATCH', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

// Executors
export async function listExecutors(options?: ApiRequestOptions): Promise<ListExecutorsResponse> {
  return fetchJson<ListExecutorsResponse>('/api/v1/executors', options);
}

// Environments
export async function listEnvironments(options?: ApiRequestOptions): Promise<ListEnvironmentsResponse> {
  return fetchJson<ListEnvironmentsResponse>('/api/v1/environments', options);
}

// Agents
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

export type CommandPreviewRequest = {
  model: string;
  permission_settings: Record<string, boolean>;
  cli_passthrough: boolean;
};

export type CommandPreviewResponse = {
  supported: boolean;
  command: string[];
  command_string: string;
};

export async function previewAgentCommand(
  agentName: string,
  payload: CommandPreviewRequest,
  options?: ApiRequestOptions
): Promise<CommandPreviewResponse> {
  return fetchJson<CommandPreviewResponse>(`/api/v1/agent-command-preview/${agentName}`, {
    ...options,
    init: { method: 'POST', body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function fetchDynamicModels(
  agentName: string,
  options?: ApiRequestOptions & { refresh?: boolean }
): Promise<DynamicModelsResponse> {
  const refresh = options?.refresh ?? false;
  const url = `/api/v1/agent-models/${agentName}${refresh ? '?refresh=true' : ''}`;
  return fetchJson<DynamicModelsResponse>(url, options);
}

// Editors
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

// Prompts
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

// Notification providers
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
