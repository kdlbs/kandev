import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  MCPDefinitionInput,
  MCPDefinitionPatch,
  MCPMarketplaceEntry,
  MCPMarketplaceInstallInput,
  MCPMarketplaceSearchResponse,
  MCPServerDefinition,
  MCPSelectionResponse,
  MCPSelectionScope,
} from "@/lib/types/http-mcp";

const workspacePath = (workspaceId: string) =>
  `/api/v1/workspaces/${encodeURIComponent(workspaceId)}`;

const ownerPath = (scope: MCPSelectionScope, ownerId: string) => {
  const resources: Record<MCPSelectionScope, string> = {
    profile: "agent-profiles",
    repository: "repositories",
    task: "tasks",
    task_session: "task-sessions",
  };
  const resource = resources[scope];
  return `/api/v1/${resource}/${encodeURIComponent(ownerId)}/mcp-selections`;
};

export async function listMCPServers(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<MCPServerDefinition[]> {
  const response = await fetchJson<{ servers?: MCPServerDefinition[] }>(
    `${workspacePath(workspaceId)}/mcp-servers`,
    options,
  );
  return response.servers ?? [];
}

export async function createMCPServer(
  workspaceId: string,
  payload: MCPDefinitionInput,
  options?: ApiRequestOptions,
): Promise<MCPServerDefinition> {
  return fetchJson<MCPServerDefinition>(`${workspacePath(workspaceId)}/mcp-servers`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function updateMCPServer(
  workspaceId: string,
  serverId: string,
  payload: MCPDefinitionPatch,
  options?: ApiRequestOptions,
): Promise<MCPServerDefinition> {
  return fetchJson<MCPServerDefinition>(
    `${workspacePath(workspaceId)}/mcp-servers/${encodeURIComponent(serverId)}`,
    {
      ...options,
      init: { method: "PATCH", body: JSON.stringify(payload), ...(options?.init ?? {}) },
    },
  );
}

export async function deleteMCPServer(
  workspaceId: string,
  serverId: string,
  expectedRevision: number,
  confirm = false,
  options?: ApiRequestOptions,
): Promise<void> {
  await fetchJson<{ success: boolean }>(
    `${workspacePath(workspaceId)}/mcp-servers/${encodeURIComponent(serverId)}`,
    {
      ...options,
      init: {
        method: "DELETE",
        body: JSON.stringify({ expected_revision: expectedRevision, confirm }),
        ...(options?.init ?? {}),
      },
    },
  );
}

export async function searchMCPMarketplace(
  query = "",
  options?: ApiRequestOptions,
): Promise<MCPMarketplaceSearchResponse> {
  const params = new URLSearchParams();
  if (query.trim()) params.set("search", query.trim());
  const suffix = params.toString() ? `?${params.toString()}` : "";
  return fetchJson<MCPMarketplaceSearchResponse>(`/api/v1/mcp-marketplace${suffix}`, {
    ...options,
    cache: "no-store",
  });
}

export async function getMCPMarketplaceEntry(
  identity: string,
  options?: ApiRequestOptions,
): Promise<MCPMarketplaceEntry> {
  return fetchJson<MCPMarketplaceEntry>(
    `/api/v1/mcp-marketplace/entry?identity=${encodeURIComponent(identity)}`,
    options,
  );
}

export async function refreshMCPMarketplace(
  options?: ApiRequestOptions,
): Promise<{ refreshed: boolean }> {
  return fetchJson<{ refreshed: boolean }>("/api/v1/mcp-marketplace/refresh", {
    ...options,
    init: { method: "POST", ...(options?.init ?? {}) },
  });
}

export async function installMCPMarketplaceEntry(
  workspaceId: string,
  payload: MCPMarketplaceInstallInput,
  options?: ApiRequestOptions,
): Promise<MCPServerDefinition> {
  return fetchJson<MCPServerDefinition>(`${workspacePath(workspaceId)}/mcp-marketplace/install`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(payload), ...(options?.init ?? {}) },
  });
}

export async function getMCPSelections(
  scope: MCPSelectionScope,
  ownerId: string,
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<MCPSelectionResponse> {
  return fetchJson<MCPSelectionResponse>(
    `${ownerPath(scope, ownerId)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    options,
  );
}

export async function replaceMCPSelections(
  scope: MCPSelectionScope,
  ownerId: string,
  workspaceId: string,
  definitionIds: string[],
  options?: ApiRequestOptions,
): Promise<MCPSelectionResponse> {
  return fetchJson<MCPSelectionResponse>(ownerPath(scope, ownerId), {
    ...options,
    init: {
      method: "PUT",
      body: JSON.stringify({ workspace_id: workspaceId, definition_ids: definitionIds }),
      ...(options?.init ?? {}),
    },
  });
}
