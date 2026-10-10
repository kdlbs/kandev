import type { AgentMcpDiscoveryResponse } from "@/lib/types/agent-mcp-discovery";

export type CursorMcpSelectionRow = {
  id: string;
  name: string;
  pluginName: string | null;
  sourceKind: string;
  credentialsAvailable: boolean;
  selected: boolean;
  unavailable: boolean;
};

export type CursorMcpSelectionGroup = {
  id: string;
  pluginName: string | null;
  servers: CursorMcpSelectionRow[];
};

export function buildCursorMcpSelectionGroups(
  discovery: AgentMcpDiscoveryResponse,
  selectedServerIds: string[],
): CursorMcpSelectionGroup[] {
  const selected = new Set(selectedServerIds);
  const discoveredIds = new Set(discovery.servers.map((server) => server.id));
  const groups = new Map<string, CursorMcpSelectionGroup>();

  for (const server of discovery.servers) {
    const pluginName = server.plugin_name?.trim() || null;
    const groupId = pluginName ? `plugin:${pluginName}` : `source:${server.source_kind}`;
    const group = groups.get(groupId) ?? { id: groupId, pluginName, servers: [] };
    group.servers.push({
      id: server.id,
      name: server.name,
      pluginName,
      sourceKind: server.source_kind,
      credentialsAvailable: server.credentials_available,
      selected: selected.has(server.id),
      unavailable: false,
    });
    groups.set(groupId, group);
  }

  const unavailable = selectedServerIds
    .filter((id) => !discoveredIds.has(id))
    .map((id) => ({
      id,
      name: id,
      pluginName: null,
      sourceKind: "unknown",
      credentialsAvailable: false,
      selected: true,
      unavailable: true,
    }));
  if (unavailable.length > 0) {
    groups.set("unavailable-selections", {
      id: "unavailable-selections",
      pluginName: null,
      servers: unavailable,
    });
  }
  return [...groups.values()];
}
