import type {
  MCPAttachmentHistory,
  MCPAttachmentServer,
  MCPAttachmentStatus,
} from "@/lib/state/slices/session-runtime/types";

export type MCPToolCatalogState = "loaded" | "not_loaded" | "unavailable";

const STATUS_LABEL_KEYS: Record<MCPAttachmentStatus, string> = {
  active: "task:mcpStatusActive",
  connected: "task:mcpStatusConnected",
  delivered: "task:mcpStatusDelivered",
  failed: "task:mcpStatusFailed",
  filtered: "task:mcpStatusFiltered",
  unavailable: "task:mcpStatusUnavailable",
  unknown: "task:mcpStatusUnknown",
};

export function buildMcpExplorerServers(
  configuredNames: string[],
  attachmentHistory?: MCPAttachmentHistory,
): MCPAttachmentServer[] {
  const observedServers = attachmentHistory?.current.servers;
  if (observedServers && observedServers.length > 0) return observedServers;
  return configuredNames.map((name) => ({ name, status: "unknown" }));
}

export function selectMcpServerName(
  servers: MCPAttachmentServer[],
  selectedName?: string | null,
): string | null {
  if (servers.length === 0) return null;
  if (selectedName && servers.some((server) => server.name === selectedName)) {
    return selectedName;
  }
  return servers.find((server) => server.name === "kandev")?.name ?? servers[0].name;
}

export function mcpStatusLabelKey(status: MCPAttachmentStatus): string {
  return STATUS_LABEL_KEYS[status] ?? STATUS_LABEL_KEYS.unknown;
}

export function isKandevMcpServer(server: MCPAttachmentServer): boolean {
  return server.name === "kandev" && server.source !== "profile";
}

export function getMcpCatalogState(server: MCPAttachmentServer): MCPToolCatalogState {
  if (!isKandevMcpServer(server)) return "unavailable";
  if (server.tools_listed_at || server.tools !== undefined) return "loaded";
  if (
    server.status === "failed" ||
    server.status === "filtered" ||
    server.status === "unavailable"
  ) {
    return "unavailable";
  }
  return "not_loaded";
}

export function getMcpToolCounts(server: MCPAttachmentServer) {
  const stored = server.tools?.length ?? 0;
  return {
    stored,
    total: server.tool_count ?? stored,
    truncated: server.tool_catalog_truncated === true,
  };
}
