import { describe, expect, it } from "vitest";
import {
  mcpCatalogUsageHref,
  mcpOwnerDestinationHref,
  type MCPOwnerDestination,
} from "./management-links";

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2
// Routes carry owner identity only; assignment owns its separate definition hint.
describe("MCP owner destinations", () => {
  it.each<[MCPOwnerDestination, string]>([
    [
      { scope: "profile", workspaceId: "w/1", ownerId: "p/1", agentId: "a/1" },
      "/settings/agents/a%2F1/profiles/p%2F1?workspaceId=w%2F1#mcp",
    ],
    [
      { scope: "repository", workspaceId: "w/1", ownerId: "r/1" },
      "/settings/workspaces/w%2F1/repositories?repositoryId=r%2F1#mcp",
    ],
    [{ scope: "task", workspaceId: "w", ownerId: "t/1" }, "/t/t%2F1"],
    [
      { scope: "task_session", workspaceId: "w", ownerId: "s/1", taskId: "t/1" },
      "/t/t%2F1?sessionId=s%2F1",
    ],
  ])("builds a scoped destination without selection intent: %o", (owner, href) => {
    expect(mcpOwnerDestinationHref(owner)).toBe(href);
  });
  it("refuses incomplete or malformed owner identity", () => {
    for (const owner of [
      { scope: "profile", workspaceId: "w", ownerId: "p", agentId: "" },
      { scope: "task_session", workspaceId: "w", ownerId: "s", taskId: "" },
      { scope: "task", workspaceId: "", ownerId: "t" },
      { scope: "repository", workspaceId: "w", ownerId: "\n" },
      { scope: "arbitrary", workspaceId: "w", ownerId: "t" },
    ])
      expect(mcpOwnerDestinationHref(owner as MCPOwnerDestination)).toBeNull();
  });
  it("uses existing catalog usage with encoded raw IDs", () => {
    expect(mcpCatalogUsageHref("w/1", "d%1")).toBe(
      "/settings/workspaces/w%2F1/mcp-servers?serverId=d%251&view=usage",
    );
    expect(mcpCatalogUsageHref("", "d")).toBeNull();
  });
});

import { mcpUsageOwnerHref, readMCPContextHint } from "./management-links";
import type { MCPOwnerUsage } from "@/lib/types/http-mcp";

describe("management context admission", () => {
  it("rejects duplicate, empty and malformed hints without guessing context", () => {
    for (const query of [
      "workspaceId=",
      "workspaceId=a&workspaceId=b",
      "workspaceId=%00",
      "workspaceId=%EF%BF%BD",
    ]) {
      expect(readMCPContextHint(new URLSearchParams(query), "workspaceId")).toEqual({
        present: true,
        value: null,
      });
    }
    expect(readMCPContextHint(new URLSearchParams(), "workspaceId")).toEqual({
      present: false,
      value: null,
    });
    expect(readMCPContextHint(new URLSearchParams("workspaceId=w%2F1"), "workspaceId")).toEqual({
      present: true,
      value: "w/1",
    });
  });
  it("requires destination identity to agree with the authorized usage row", () => {
    const row: MCPOwnerUsage = {
      scope: "profile",
      workspace_id: "w",
      owner_id: "p",
      agent_id: "a",
      owner_state: "available",
      destination: { kind: "profile", workspace_id: "w", owner_id: "p", agent_id: "a" },
    };
    expect(mcpUsageOwnerHref(row, "w")).toBe("/settings/agents/a/profiles/p?workspaceId=w#mcp");
    expect(mcpUsageOwnerHref(row, "other")).toBeNull();
    for (const destination of [
      null,
      { ...row.destination!, owner_id: "other" },
      { ...row.destination!, kind: "repository" as const },
      { ...row.destination!, agent_id: "other" },
    ]) {
      expect(mcpUsageOwnerHref({ ...row, destination }, "w")).toBeNull();
    }
  });
});
