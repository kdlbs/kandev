import { describe, expect, it } from "vitest";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";
import { mcpDefinitionBadgeKeys } from "./mcp-definition-badges";

const definition = { execution_mode: "remote", transport: "sse", source: "custom" } as const;

describe("inherited MCP setup metadata", () => {
  it.each([
    ["remote", "settings:mcpModeRemote"],
    ["managed_package", "settings:mcpModeManagedPackage"],
    ["existing_executable", "settings:mcpModeExistingExecutable"],
  ] as const)("describes %s without claiming it connected", (execution_mode, expected) => {
    expect(mcpDefinitionBadgeKeys({ ...definition, execution_mode }).mode).toBe(expected);
  });

  it.each([
    ["stdio", "settings:mcpTransportStdio"],
    ["sse", "settings:mcpTransportSse"],
    ["streamable_http", "settings:mcpTransportStreamable"],
    ["http", "settings:mcpTransportStreamable"],
  ] as const)("labels %s transport, including the HTTP alias", (transport, expected) => {
    expect(mcpDefinitionBadgeKeys({ ...definition, transport }).transport).toBe(expected);
  });

  it.each([
    ["custom", "settings:mcpSourceCustom"],
    ["curated", "settings:mcpSourceCurated"],
    ["registry", "settings:mcpSourceRegistry"],
    ["legacy_import", undefined],
  ] as const)("does not misattribute %s definitions", (source, expected) => {
    expect(mcpDefinitionBadgeKeys({ ...definition, source }).source).toBe(expected);
  });

  it("does not copy configuration, secret bindings or endpoint data into badges", () => {
    const configured: MCPServerDefinition = {
      ...definition,
      id: "definition",
      workspace_id: "workspace",
      display_name: "Documentation",
      runtime_name: "docs",
      normalized_runtime_name: "docs",
      enabled: true,
      configuration: { url: "https://example.test/private?token=secret" },
      secret_bindings: [{ input_name: "token", secret_id: "secret-id" }],
      revision: 1,
      created_at: "",
      updated_at: "",
    };
    expect(mcpDefinitionBadgeKeys(configured)).toEqual({
      mode: "settings:mcpModeRemote",
      transport: "settings:mcpTransportSse",
      source: "settings:mcpSourceCustom",
    });
  });
});
