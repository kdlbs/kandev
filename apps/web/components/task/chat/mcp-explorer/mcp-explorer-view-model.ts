import type {
  MCPAttachmentHistory,
  MCPAttachmentServer,
  MCPAttachmentStatus,
  MCPToolSummary,
} from "@/lib/state/slices/session-runtime/types";

export type MCPToolCatalogState = "loaded" | "not_loaded" | "unavailable";
export type MCPToolArgument = {
  name: string;
  type: string;
  required: boolean;
  description?: string;
};

export type MCPToolSchemaState =
  | { kind: "none" }
  | { kind: "too_large" }
  | { kind: "schema"; arguments: MCPToolArgument[]; showJSON: boolean };

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

export function selectMcpToolName(
  server: MCPAttachmentServer | null,
  selectedName?: string | null,
): string | null {
  if (!server?.tools || !selectedName) return null;
  if (server.tools.some((tool) => tool.name === selectedName)) return selectedName;
  return null;
}

export function getMcpToolSchemaState(tool: MCPToolSummary): MCPToolSchemaState {
  if (tool.input_schema_truncated) return { kind: "too_large" };
  if (!isRecord(tool.input_schema)) return { kind: "none" };
  const properties = isRecord(tool.input_schema.properties) ? tool.input_schema.properties : {};
  const required = new Set(
    Array.isArray(tool.input_schema.required)
      ? tool.input_schema.required.filter((name): name is string => typeof name === "string")
      : [],
  );
  const args = Object.entries(properties)
    .map(([name, schema]) => mcpToolArgument(name, schema, required.has(name)))
    .sort((left, right) => left.name.localeCompare(right.name));
  if (args.length === 0 && !hasComplexSchema(tool.input_schema)) return { kind: "none" };
  return { kind: "schema", arguments: args, showJSON: hasComplexSchema(tool.input_schema) };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function mcpToolArgument(name: string, value: unknown, required: boolean): MCPToolArgument {
  const schema = isRecord(value) ? value : {};
  const type = mcpArgumentType(schema.type);
  const description = typeof schema.description === "string" ? schema.description : undefined;
  return { name, type, required, ...(description ? { description } : {}) };
}

function mcpArgumentType(value: unknown): string {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === "string").join(" | ");
  }
  return "any";
}

function hasComplexSchema(schema: Record<string, unknown>): boolean {
  const complexKeys = ["$defs", "definitions", "$ref", "allOf", "anyOf", "oneOf", "not", "items"];
  if (complexKeys.some((key) => key in schema)) return true;
  if (!isRecord(schema.properties)) return false;
  return Object.values(schema.properties).some(
    (value) =>
      isRecord(value) &&
      ("items" in value || "properties" in value || complexKeys.some((key) => key in value)),
  );
}
