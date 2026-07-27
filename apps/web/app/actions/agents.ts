"use client";

import { getBackendConfig } from "@/lib/config";
import { fetchJson } from "@/lib/api/client";
import { readInterimSettingsInterlockToken } from "@/src/boot-payload";
import type {
  Agent,
  AgentProfile,
  AgentProfileMcpConfig,
  CLIFlag,
  ProfileEnvVar,
  McpServerDef,
  ListAgentsResponse,
  ListAgentDiscoveryResponse,
} from "@/lib/types/http";
import type { PermissionKey } from "@/lib/agent-permissions";
import { normalizeAgentProfile } from "@/lib/api/domains/agent-profile-normalize";

type ProfilePermissions = Record<PermissionKey, boolean>;

const { apiBaseUrl } = getBackendConfig();
const interimSettingsInterlockHeader = "X-Kandev-Interim-Settings-Interlock";

function normalizeAgentInPlace(agent: Agent): Agent {
  return {
    ...agent,
    profiles: (agent.profiles ?? []).map((profile) => normalizeAgentProfile(profile)),
  };
}

function agentSettingsRequest<T>(url: string, init?: RequestInit): Promise<T> {
  // Pin the URL origin to the configured backend so a tainted path segment
  // (agent ID from a form, profile ID from a route param) cannot redirect
  // the request to a different host. Closes the CodeQL SSRF finding.
  const parsed = new URL(url);
  const allowed = new URL(apiBaseUrl);
  if (parsed.origin !== allowed.origin) {
    throw new Error(`Refusing to fetch outside configured backend origin: ${parsed.origin}`);
  }
  return fetchJson<T>(parsed.toString(), { cache: "no-store", init });
}

export async function listAgentDiscoveryAction(): Promise<ListAgentDiscoveryResponse> {
  return agentSettingsRequest<ListAgentDiscoveryResponse>(`${apiBaseUrl}/api/v1/agents/discovery`);
}

export async function listAgentsAction(): Promise<ListAgentsResponse> {
  const res = await agentSettingsRequest<ListAgentsResponse>(`${apiBaseUrl}/api/v1/agents`);
  return { ...res, agents: (res.agents ?? []).map(normalizeAgentInPlace) };
}

export async function createAgentAction(payload: {
  name: string;
  workspace_id?: string | null;
  profiles?: Array<
    {
      name: string;
      model: string;
      mode?: string;
      cli_passthrough: boolean;
      cli_flags?: CLIFlag[];
      command_prefix?: string;
      env_vars?: ProfileEnvVar[];
    } & ProfilePermissions
  >;
}): Promise<Agent> {
  const res = await agentSettingsRequest<Agent>(`${apiBaseUrl}/api/v1/agents`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
  return normalizeAgentInPlace(res);
}

export async function updateAgentAction(
  id: string,
  payload: {
    workspace_id?: string | null;
    supports_mcp?: boolean;
    mcp_config_path?: string | null;
  },
): Promise<Agent> {
  const res = await agentSettingsRequest<Agent>(`${apiBaseUrl}/api/v1/agents/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
  return normalizeAgentInPlace(res);
}

export async function deleteAgentAction(id: string) {
  await agentSettingsRequest<void>(`${apiBaseUrl}/api/v1/agents/${id}`, { method: "DELETE" });
}

export async function createAgentProfileAction(
  agentId: string,
  payload: {
    name: string;
    model: string;
    mode?: string;
    config_options?: Record<string, string>;
    cli_passthrough: boolean;
    cli_flags?: CLIFlag[];
    command_prefix?: string;
    env_vars?: ProfileEnvVar[];
  } & ProfilePermissions,
): Promise<AgentProfile> {
  const raw = await agentSettingsRequest<unknown>(
    `${apiBaseUrl}/api/v1/agents/${agentId}/profiles`,
    {
      method: "POST",
      body: JSON.stringify(payload),
    },
  );
  return normalizeAgentProfile(raw);
}

export async function updateAgentProfileAction(
  id: string,
  payload: {
    name?: string;
    model?: string;
    mode?: string;
    config_options?: Record<string, string>;
    allow_indexing?: boolean;
    auto_approve?: boolean;
    cli_passthrough?: boolean;
    cli_flags?: CLIFlag[];
    command_prefix?: string;
    env_vars?: ProfileEnvVar[];
  },
): Promise<AgentProfile> {
  const raw = await agentSettingsRequest<unknown>(`${apiBaseUrl}/api/v1/agent-profiles/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
  return normalizeAgentProfile(raw);
}

import type {
  ActiveSessionInfo,
  RoutingTierReference,
  WatcherReference,
} from "@/lib/types/agent-profile-errors";

export type DeleteProfileResult =
  | { status: "ok" }
  | {
      status: "conflict";
      activeSessions: ActiveSessionInfo[];
      watchers: WatcherReference[];
      routingTiers: RoutingTierReference[];
    }
  | { status: "error"; message: string };

export async function deleteAgentProfileAction(
  id: string,
  force?: boolean,
): Promise<DeleteProfileResult> {
  const url = `${apiBaseUrl}/api/v1/agent-profiles/${id}${force ? "?force=true" : ""}`;
  const token = readInterimSettingsInterlockToken();
  const response = await fetch(url, {
    method: "DELETE",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { [interimSettingsInterlockHeader]: token } : {}),
    },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    // A 409 is active sessions, referencing watchers, routing tier mappings, or a mix.
    // Treat any non-empty list as the conflict signal — a watcher-only
    // conflict (the new self-heal path) must still pop the dialog.
    if (response.status === 409 && (body.active_sessions || body.watchers || body.routing_tiers)) {
      return {
        status: "conflict",
        activeSessions: body.active_sessions ?? [],
        watchers: body.watchers ?? [],
        routingTiers: body.routing_tiers ?? [],
      };
    }
    return {
      status: "error",
      message: body?.error || `Request failed: ${response.status} ${response.statusText}`,
    };
  }
  return { status: "ok" };
}

export async function getAgentProfileMcpConfigAction(
  profileId: string,
): Promise<AgentProfileMcpConfig> {
  return agentSettingsRequest<AgentProfileMcpConfig>(
    `${apiBaseUrl}/api/v1/agent-profiles/${profileId}/mcp-config`,
  );
}

export async function updateAgentProfileMcpConfigAction(
  profileId: string,
  payload: {
    enabled: boolean;
    mcpServers: Record<string, McpServerDef>;
    meta?: Record<string, unknown>;
  },
): Promise<AgentProfileMcpConfig> {
  return agentSettingsRequest<AgentProfileMcpConfig>(
    `${apiBaseUrl}/api/v1/agent-profiles/${profileId}/mcp-config`,
    {
      method: "POST",
      body: JSON.stringify(payload),
    },
  );
}

export type CommandPreviewRequest = {
  model: string;
  permission_settings: Record<string, boolean>;
  cli_passthrough: boolean;
  cli_flags: CLIFlag[];
  command_prefix?: string;
};

export type CommandPreviewResponse = {
  supported: boolean;
  command: string[];
  command_string: string;
};

export async function previewAgentCommandAction(
  agentName: string,
  payload: CommandPreviewRequest,
): Promise<CommandPreviewResponse> {
  return agentSettingsRequest<CommandPreviewResponse>(
    `${apiBaseUrl}/api/v1/agent-command-preview/${agentName}`,
    {
      method: "POST",
      body: JSON.stringify(payload),
    },
  );
}
