'use server';

import { getBackendConfig } from '@/lib/config';
import type {
  Agent,
  AgentProfile,
  AgentProfileMcpConfig,
  McpServerDef,
  ListAgentsResponse,
  ListAgentDiscoveryResponse,
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

export async function listAgentDiscoveryAction(): Promise<ListAgentDiscoveryResponse> {
  return fetchJson<ListAgentDiscoveryResponse>(`${apiBaseUrl}/api/v1/agents/discovery`);
}

export async function listAgentsAction(): Promise<ListAgentsResponse> {
  return fetchJson<ListAgentsResponse>(`${apiBaseUrl}/api/v1/agents`);
}

export async function createAgentAction(payload: {
  name: string;
  workspace_id?: string | null;
  profiles?: Array<{
    name: string;
    model: string;
    auto_approve: boolean;
    dangerously_skip_permissions: boolean;
    plan: string;
  }>;
}): Promise<Agent> {
  return fetchJson<Agent>(`${apiBaseUrl}/api/v1/agents`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateAgentAction(
  id: string,
  payload: { workspace_id?: string | null; supports_mcp?: boolean; mcp_config_path?: string | null }
): Promise<Agent> {
  return fetchJson<Agent>(`${apiBaseUrl}/api/v1/agents/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteAgentAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/agents/${id}`, { method: 'DELETE' });
}

export async function createAgentProfileAction(
  agentId: string,
  payload: {
    name: string;
    model: string;
    auto_approve: boolean;
    dangerously_skip_permissions: boolean;
    plan: string;
  }
): Promise<AgentProfile> {
  return fetchJson<AgentProfile>(`${apiBaseUrl}/api/v1/agents/${agentId}/profiles`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}

export async function updateAgentProfileAction(
  id: string,
  payload: Partial<Pick<AgentProfile, 'name' | 'model' | 'auto_approve' | 'dangerously_skip_permissions' | 'plan'>>
): Promise<AgentProfile> {
  return fetchJson<AgentProfile>(`${apiBaseUrl}/api/v1/agent-profiles/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export async function deleteAgentProfileAction(id: string) {
  await fetchJson<void>(`${apiBaseUrl}/api/v1/agent-profiles/${id}`, { method: 'DELETE' });
}

export async function getAgentProfileMcpConfigAction(profileId: string): Promise<AgentProfileMcpConfig> {
  return fetchJson<AgentProfileMcpConfig>(`${apiBaseUrl}/api/v1/agent-profiles/${profileId}/mcp-config`);
}

export async function updateAgentProfileMcpConfigAction(
  profileId: string,
  payload: { enabled: boolean; mcpServers: Record<string, McpServerDef>; meta?: Record<string, unknown> }
): Promise<AgentProfileMcpConfig> {
  return fetchJson<AgentProfileMcpConfig>(`${apiBaseUrl}/api/v1/agent-profiles/${profileId}/mcp-config`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}
