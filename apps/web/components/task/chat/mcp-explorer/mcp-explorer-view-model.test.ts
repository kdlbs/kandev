import { describe, expect, it } from "vitest";
import type {
  MCPAttachmentHistory,
  MCPAttachmentServer,
} from "@/lib/state/slices/session-runtime/types";
import {
  buildMcpExplorerServers,
  getMcpCatalogState,
  getMcpToolCounts,
  isKandevMcpServer,
  mcpStatusLabelKey,
  selectMcpServerName,
} from "./mcp-explorer-view-model";

function server(overrides: Partial<MCPAttachmentServer>): MCPAttachmentServer {
  return { name: "kandev", status: "unknown", ...overrides };
}

function history(servers: MCPAttachmentServer[]): MCPAttachmentHistory {
  return {
    version: 1,
    current: { attachment_attempt_id: "attempt-1", started_at: "2026-08-16T00:00:00Z", servers },
  };
}

describe("MCP explorer view model", () => {
  it("uses observed servers and falls back to configured names", () => {
    const configured = buildMcpExplorerServers(["kandev", "filesystem"]);
    expect(configured.map((item) => item.name)).toEqual(["kandev", "filesystem"]);

    const observed = buildMcpExplorerServers(
      ["kandev", "filesystem"],
      history([server({ name: "filesystem", source: "profile", status: "delivered" })]),
    );
    expect(observed.map((item) => item.name)).toEqual(["filesystem"]);
    expect(observed[0].source).toBe("profile");
  });

  it("selects kandev first and falls back when the selection disappears", () => {
    const servers = [server({ name: "filesystem" }), server({ name: "kandev" })];
    expect(selectMcpServerName(servers)).toBe("kandev");
    expect(selectMcpServerName(servers, "filesystem")).toBe("filesystem");
    expect(selectMcpServerName([servers[1]], "filesystem")).toBe("kandev");
    expect(selectMcpServerName([], "kandev")).toBeNull();
  });

  it("reports localized status keys and catalog availability", () => {
    expect(mcpStatusLabelKey("active")).toBe("task:mcpStatusActive");
    expect(mcpStatusLabelKey("unknown")).toBe("task:mcpStatusUnknown");

    const loaded = server({
      status: "active",
      tools_listed_at: "2026-08-16T00:01:00Z",
      tool_count: 129,
      tools: [{ name: "create_task_kandev", description: "Create a task" }],
      tool_catalog_truncated: true,
    });
    expect(getMcpCatalogState(loaded)).toBe("loaded");
    expect(getMcpToolCounts(loaded)).toEqual({ stored: 1, total: 129, truncated: true });
    expect(getMcpCatalogState(server({ status: "unknown" }))).toBe("not_loaded");
    expect(
      getMcpCatalogState(server({ name: "filesystem", source: "profile", status: "delivered" })),
    ).toBe("unavailable");
  });

  it("identifies only the backend-owned Kandev server", () => {
    expect(isKandevMcpServer(server({ name: "kandev", source: "kandev" }))).toBe(true);
    expect(isKandevMcpServer(server({ name: "kandev", source: "profile" }))).toBe(false);
    expect(isKandevMcpServer(server({ name: "filesystem", source: "profile" }))).toBe(false);
  });
});
