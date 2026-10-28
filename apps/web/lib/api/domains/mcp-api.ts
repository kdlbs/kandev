import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  MCPCatalogUsagePage,
  MCPProfileOwner,
  MCPProfileOwnerPage,
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
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
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
      init: { ...(options?.init ?? {}), method: "PATCH", body: JSON.stringify(payload) },
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
        ...(options?.init ?? {}),
        method: "DELETE",
        body: JSON.stringify({ expected_revision: expectedRevision, confirm }),
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
    init: { ...(options?.init ?? {}), method: "POST" },
  });
}

export async function installMCPMarketplaceEntry(
  workspaceId: string,
  payload: MCPMarketplaceInstallInput,
  options?: ApiRequestOptions,
): Promise<MCPServerDefinition> {
  return fetchJson<MCPServerDefinition>(`${workspacePath(workspaceId)}/mcp-marketplace/install`, {
    ...options,
    init: { ...(options?.init ?? {}), method: "POST", body: JSON.stringify(payload) },
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

export type MCPSelectionWriteOptions = ApiRequestOptions & {
  expected_definition_ids?: string[];
  expected_revision?: number;
};

export async function replaceMCPSelections(
  scope: MCPSelectionScope,
  ownerId: string,
  workspaceId: string,
  definitionIds: string[],
  options?: MCPSelectionWriteOptions,
): Promise<MCPSelectionResponse> {
  return fetchJson<MCPSelectionResponse>(ownerPath(scope, ownerId), {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PUT",
      body: JSON.stringify({
        workspace_id: workspaceId,
        definition_ids: definitionIds,
        expected_definition_ids: options?.expected_definition_ids,
        expected_revision: options?.expected_revision,
      }),
    },
  });
}

export async function listMCPUsage(
  workspaceId: string,
  definitionId: string,
  cursor?: string,
  options?: ApiRequestOptions,
): Promise<MCPCatalogUsagePage> {
  const suffix = cursor ? `?${new URLSearchParams({ cursor })}` : "";
  return fetchJson<MCPCatalogUsagePage>(
    `${workspacePath(workspaceId)}/mcp-servers/${encodeURIComponent(definitionId)}/usage${suffix}`,
    { ...options, cache: "no-store" },
  );
}

export async function listMCPProfileOwners(
  workspaceId: string,
  cursor?: string,
  options?: ApiRequestOptions,
): Promise<MCPProfileOwnerPage> {
  const query = cursor ? `?${new URLSearchParams({ cursor })}` : "";
  return fetchJson(`${workspacePath(workspaceId)}/mcp-profile-owners${query}`, {
    ...options,
    cache: "no-store",
  });
}

export async function getMCPProfileOwner(
  workspaceId: string,
  profileId: string,
  options?: ApiRequestOptions,
): Promise<MCPProfileOwner> {
  return fetchJson(
    `${workspacePath(workspaceId)}/mcp-profile-owners/${encodeURIComponent(profileId)}`,
    {
      ...options,
      cache: "no-store",
    },
  );
}
