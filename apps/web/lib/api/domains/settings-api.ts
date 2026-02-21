import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  Agent,
  ListAgentsResponse,
  ListAgentDiscoveryResponse,
  ListAvailableAgentsResponse,
  AgentProfileMcpConfig,
  ListExecutorsResponse,
  ListExecutorProfilesResponse,
  ExecutorProfile,
  ProfileEnvVar,
  NotificationProvidersResponse,
  NotificationProvider,
  EditorsResponse,
  EditorOption,
  CustomPrompt,
  PromptsResponse,
  SavedLayout,
  UserSettingsResponse,
  DynamicModelsResponse,
} from "@/lib/types/http";

// User settings
export async function fetchUserSettings(options?: ApiRequestOptions) {
  return fetchJson<UserSettingsResponse>("/api/v1/user/settings", options);
}

export async function updateUserSettings(
  payload: {
    workspace_id?: string;
    workflow_filter_id?: string;
    kanban_view_mode?: string;
    repository_ids?: string[];
    preferred_shell?: string;
    default_editor_id?: string;
    enable_preview_on_click?: boolean;
    chat_submit_key?: "enter" | "cmd_enter";
    review_auto_mark_on_scroll?: boolean;
    lsp_auto_start_languages?: string[];
    lsp_auto_install_languages?: string[];
    lsp_server_configs?: Record<string, Record<string, unknown>>;
    saved_layouts?: SavedLayout[];
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<UserSettingsResponse>("/api/v1/user/settings", {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

// Executors
export async function listExecutors(options?: ApiRequestOptions): Promise<ListExecutorsResponse> {
  return fetchJson<ListExecutorsResponse>("/api/v1/executors", options);
}

// Executor profiles
export async function listExecutorProfiles(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<ListExecutorProfilesResponse> {
  return fetchJson<ListExecutorProfilesResponse>(
    `/api/v1/executors/${executorId}/profiles`,
    options,
  );
}

export async function listAllExecutorProfiles(
  options?: ApiRequestOptions,
): Promise<ListExecutorProfilesResponse> {
  return fetchJson<ListExecutorProfilesResponse>("/api/v1/executor-profiles", options);
}

export type ScriptPlaceholder = {
  key: string;
  description: string;
  example: string;
  executor_types: string[];
};

export async function listScriptPlaceholders(
  options?: ApiRequestOptions,
): Promise<{ placeholders: ScriptPlaceholder[] }> {
  return fetchJson<{ placeholders: ScriptPlaceholder[] }>("/api/v1/script-placeholders", options);
}

export async function fetchDefaultScripts(
  executorType: string,
  options?: ApiRequestOptions,
): Promise<{ prepare_script: string; cleanup_script: string }> {
  return fetchJson<{ prepare_script: string; cleanup_script: string }>(
    `/api/v1/executor-profiles/default-script?type=${encodeURIComponent(executorType)}`,
    options,
  );
}

export async function createExecutorProfile(
  executorId: string,
  payload: {
    name: string;
    mcp_policy?: string;
    config?: Record<string, string>;
    prepare_script?: string;
    cleanup_script?: string;
    env_vars?: ProfileEnvVar[];
  },
  options?: ApiRequestOptions,
): Promise<ExecutorProfile> {
  return fetchJson<ExecutorProfile>(`/api/v1/executors/${executorId}/profiles`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateExecutorProfile(
  executorId: string,
  profileId: string,
  payload: {
    name?: string;
    mcp_policy?: string;
    config?: Record<string, string>;
    prepare_script?: string;
    cleanup_script?: string;
    env_vars?: ProfileEnvVar[];
  },
  options?: ApiRequestOptions,
): Promise<ExecutorProfile> {
  return fetchJson<ExecutorProfile>(`/api/v1/executors/${executorId}/profiles/${profileId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteExecutorProfile(
  executorId: string,
  profileId: string,
  options?: ApiRequestOptions,
): Promise<{ success: boolean }> {
  return fetchJson<{ success: boolean }>(`/api/v1/executors/${executorId}/profiles/${profileId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// Agents
export async function listAgents(options?: ApiRequestOptions): Promise<ListAgentsResponse> {
  return fetchJson<ListAgentsResponse>("/api/v1/agents", options);
}

export async function listAgentDiscovery(
  options?: ApiRequestOptions,
): Promise<ListAgentDiscoveryResponse> {
  return fetchJson<ListAgentDiscoveryResponse>("/api/v1/agents/discovery", options);
}

export async function listAvailableAgents(
  options?: ApiRequestOptions,
): Promise<ListAvailableAgentsResponse> {
  return fetchJson<ListAvailableAgentsResponse>("/api/v1/agents/available", options);
}

export async function getAgentProfileMcpConfig(
  profileId: string,
  options?: ApiRequestOptions,
): Promise<AgentProfileMcpConfig> {
  return fetchJson<AgentProfileMcpConfig>(
    `/api/v1/agent-profiles/${profileId}/mcp-config`,
    options,
  );
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
  options?: ApiRequestOptions,
): Promise<CommandPreviewResponse> {
  return fetchJson<CommandPreviewResponse>(`/api/v1/agent-command-preview/${agentName}`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function createCustomTUIAgent(
  payload: { display_name: string; model?: string; command: string; description?: string },
  options?: ApiRequestOptions,
): Promise<Agent> {
  return fetchJson<Agent>("/api/v1/agents/tui", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function fetchDynamicModels(
  agentName: string,
  options?: ApiRequestOptions & { refresh?: boolean },
): Promise<DynamicModelsResponse> {
  const refresh = options?.refresh ?? false;
  const url = `/api/v1/agent-models/${agentName}${refresh ? "?refresh=true" : ""}`;
  return fetchJson<DynamicModelsResponse>(url, options);
}

// Editors
export async function listEditors(options?: ApiRequestOptions) {
  return fetchJson<EditorsResponse>("/api/v1/editors", options);
}

export async function createEditor(
  payload: {
    name: string;
    kind: string;
    config?: Record<string, unknown>;
    enabled?: boolean;
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<EditorOption>("/api/v1/editors", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
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
  options?: ApiRequestOptions,
) {
  return fetchJson<EditorOption>(`/api/v1/editors/${editorId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteEditor(editorId: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/editors/${editorId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// Prompts
export async function listPrompts(options?: ApiRequestOptions) {
  return fetchJson<PromptsResponse>("/api/v1/prompts", options);
}

export async function createPrompt(
  payload: { name: string; content: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<CustomPrompt>("/api/v1/prompts", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updatePrompt(
  promptId: string,
  payload: { name?: string; content?: string },
  options?: ApiRequestOptions,
) {
  return fetchJson<CustomPrompt>(`/api/v1/prompts/${promptId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deletePrompt(promptId: string, options?: ApiRequestOptions) {
  return fetchJson<{ success: boolean }>(`/api/v1/prompts/${promptId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// Notification providers
export async function listNotificationProviders(options?: ApiRequestOptions) {
  return fetchJson<NotificationProvidersResponse>("/api/v1/notification-providers", options);
}

export async function createNotificationProvider(
  payload: {
    name: string;
    type: NotificationProvider["type"];
    config?: NotificationProvider["config"];
    enabled?: boolean;
    events?: string[];
  },
  options?: ApiRequestOptions,
) {
  return fetchJson<NotificationProvider>("/api/v1/notification-providers", {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateNotificationProvider(
  providerId: string,
  payload: Partial<{
    name: string;
    type: NotificationProvider["type"];
    config: NotificationProvider["config"];
    enabled: boolean;
    events: string[];
  }>,
  options?: ApiRequestOptions,
) {
  return fetchJson<NotificationProvider>(`/api/v1/notification-providers/${providerId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function deleteNotificationProvider(providerId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`/api/v1/notification-providers/${providerId}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// Docker management

export type DockerContainer = {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  started_at?: string;
};

export function buildDockerImageUrl(payload: {
  dockerfile: string;
  tag: string;
  build_args?: Record<string, string>;
}): string {
  // Returns the URL for SSE streaming. Caller should use EventSource or fetch with streaming.
  const params = new URLSearchParams();
  params.set("dockerfile", payload.dockerfile);
  params.set("tag", payload.tag);
  if (payload.build_args) {
    params.set("build_args", JSON.stringify(payload.build_args));
  }
  return `/api/v1/docker/build?${params.toString()}`;
}

export async function buildDockerImage(
  payload: {
    dockerfile: string;
    tag: string;
    build_args?: Record<string, string>;
  },
  options?: ApiRequestOptions,
): Promise<Response> {
  const baseUrl = options?.baseUrl ?? "";
  return fetch(`${baseUrl}/api/v1/docker/build`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    ...(options?.init ?? {}),
  });
}

export async function listDockerContainers(
  filter?: { image?: string; labels?: Record<string, string> },
  options?: ApiRequestOptions,
): Promise<{ containers: DockerContainer[] }> {
  const params = new URLSearchParams();
  if (filter?.image) params.set("image", filter.image);
  if (filter?.labels) params.set("labels", JSON.stringify(filter.labels));
  const qs = params.toString();
  return fetchJson<{ containers: DockerContainer[] }>(
    `/api/v1/docker/containers${qs ? `?${qs}` : ""}`,
    options,
  );
}

export async function stopDockerContainer(
  id: string,
  options?: ApiRequestOptions,
): Promise<void> {
  await fetchJson<{ success: boolean }>(`/api/v1/docker/containers/${id}/stop`, {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function removeDockerContainer(
  id: string,
  options?: ApiRequestOptions,
): Promise<void> {
  await fetchJson<{ success: boolean }>(`/api/v1/docker/containers/${id}`, {
    ...options,
    init: { method: "DELETE", ...(options?.init ?? {}) },
  });
}

// Sprites network policies

export type NetworkPolicyRule = {
  domain: string;
  action: "allow" | "deny";
  include?: string;
};

export type NetworkPolicy = {
  rules: NetworkPolicyRule[];
};

export async function getSpritesNetworkPolicies(
  secretId: string,
  spriteName: string,
  options?: ApiRequestOptions,
): Promise<NetworkPolicy> {
  const params = new URLSearchParams({ secret_id: secretId, sprite_name: spriteName });
  return fetchJson<NetworkPolicy>(`/api/v1/sprites/network-policies?${params.toString()}`, options);
}

export async function updateSpritesNetworkPolicies(
  secretId: string,
  spriteName: string,
  policy: NetworkPolicy,
  options?: ApiRequestOptions,
): Promise<void> {
  const params = new URLSearchParams({ secret_id: secretId, sprite_name: spriteName });
  await fetchJson<void>(`/api/v1/sprites/network-policies?${params.toString()}`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(policy), ...(options?.init ?? {}) },
  });
}
