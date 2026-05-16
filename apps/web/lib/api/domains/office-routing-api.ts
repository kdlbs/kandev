import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AgentRouteData,
  AgentRoutePreview,
  AgentRoutingOverrides,
  ProviderHealth,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

export type RoutingConfigResponse = {
  config: WorkspaceRouting | null;
  known_providers: string[];
};

export function getWorkspaceRouting(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<RoutingConfigResponse>(`${BASE}/workspaces/${workspaceId}/routing`, options);
}

export function updateWorkspaceRouting(
  workspaceId: string,
  cfg: WorkspaceRouting,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/workspaces/${workspaceId}/routing`, {
    ...options,
    init: { method: "PUT", body: JSON.stringify(cfg), ...options?.init },
  });
}

export type RoutingRetryResponse = {
  status: string;
  retry_at?: string;
};

export function retryProvider(
  workspaceId: string,
  providerId: string,
  options?: ApiRequestOptions,
) {
  return fetchJson<RoutingRetryResponse>(`${BASE}/workspaces/${workspaceId}/routing/retry`, {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ provider_id: providerId }),
      ...options?.init,
    },
  });
}

export type ProviderHealthResponse = { health: ProviderHealth[] };

export function getProviderHealth(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<ProviderHealthResponse>(
    `${BASE}/workspaces/${workspaceId}/routing/health`,
    options,
  );
}

export type RoutingPreviewResponse = { agents: AgentRoutePreview[] };

export function getRoutingPreview(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<RoutingPreviewResponse>(
    `${BASE}/workspaces/${workspaceId}/routing/preview`,
    options,
  );
}

export function getAgentRoute(agentId: string, options?: ApiRequestOptions) {
  return fetchJson<AgentRouteData>(`${BASE}/agents/${agentId}/route`, options);
}

export function updateAgentRouting(
  agentId: string,
  ov: AgentRoutingOverrides,
  options?: ApiRequestOptions,
) {
  return fetchJson<{ ok: boolean }>(`${BASE}/agents/${agentId}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify({ routing: ov }), ...options?.init },
  });
}
