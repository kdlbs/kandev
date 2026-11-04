import type {
  MCPDefinitionSource,
  MCPExecutionMode,
  MCPServerDefinition,
  MCPTransport,
} from "@/lib/types/http-mcp";

const MODE_KEYS: Record<MCPExecutionMode, string> = {
  remote: "settings:mcpModeRemote",
  managed_package: "settings:mcpModeManagedPackage",
  existing_executable: "settings:mcpModeExistingExecutable",
};

const TRANSPORT_KEYS: Record<MCPTransport, string> = {
  stdio: "settings:mcpTransportStdio",
  sse: "settings:mcpTransportSse",
  http: "settings:mcpTransportStreamable",
  streamable_http: "settings:mcpTransportStreamable",
};

const SOURCE_KEYS: Record<MCPDefinitionSource, string | undefined> = {
  custom: "settings:mcpSourceCustom",
  curated: "settings:mcpSourceCurated",
  registry: "settings:mcpSourceRegistry",
  legacy_import: undefined,
};

export function mcpDefinitionBadgeKeys(
  definition: Pick<MCPServerDefinition, "execution_mode" | "transport" | "source">,
) {
  return {
    mode: MODE_KEYS[definition.execution_mode],
    transport: TRANSPORT_KEYS[definition.transport],
    source: SOURCE_KEYS[definition.source],
  };
}
