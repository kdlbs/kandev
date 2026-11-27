import { describe, expect, it } from "vitest";
import {
  appendMCPAssignmentHint,
  readMCPAssignmentHint,
  removeMCPAssignmentHint,
  resolveMCPAssignmentHint,
} from "./assignment-hint";

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.6, AC-AGENTS-MCP-ASSIGNMENT-003.4
const definition = { id: "server-1", workspace_id: "workspace-a", enabled: true };
const input = {
  search: "?workspaceId=workspace-a&mcpServerId=server-1",
  workspaceId: "workspace-a",
  definitions: [definition],
  visibleDefinitionIds: [definition.id],
  ready: true,
  active: true,
};

describe("focus-only assignment hints", () => {
  it("preserves the existing owner, workspace, repeated query values and section", () => {
    const route =
      "/settings/agents/agent-a/profiles/profile-a?workspaceId=workspace-a&tag=a&tag=b#mcp";
    const hinted = appendMCPAssignmentHint(route, definition.id);
    expect(hinted).toBe(route.replace("#mcp", "&mcpServerId=server-1#mcp"));
    expect(removeMCPAssignmentHint(hinted)).toBe(route);
    expect(readMCPAssignmentHint(new URL(hinted, "https://example.test").search)).toEqual({
      present: true,
      definitionId: definition.id,
    });
  });

  it.each(["", "?query=docs"])("leaves an absent hint alone: %s", (search) => {
    expect(readMCPAssignmentHint(search)).toEqual({ present: false, definitionId: null });
    expect(resolveMCPAssignmentHint({ ...input, search })).toEqual({ status: "absent" });
  });

  it.each([
    "?mcpServerId=",
    "?mcpServerId=server-1&mcpServerId=server-1",
    "?mcpServerId=server-1&mcpServerId=other",
    "?mcpServerId=%20server-1",
    "?mcpServerId=../server-1",
    "?mcpServerId=%E0%A4%A",
  ])("consumes malformed or duplicate hints without a fallback: %s", (search) => {
    expect(readMCPAssignmentHint(search)).toEqual({ present: true, definitionId: null });
    expect(resolveMCPAssignmentHint({ ...input, search })).toEqual({
      status: "consume",
      definitionId: null,
    });
  });

  it("waits for explicit session Customize opening and loaded inventory", () => {
    expect(resolveMCPAssignmentHint({ ...input, active: false })).toEqual({ status: "pending" });
    expect(resolveMCPAssignmentHint({ ...input, ready: false })).toEqual({ status: "pending" });
    expect(resolveMCPAssignmentHint(input)).toEqual({
      status: "consume",
      definitionId: definition.id,
    });
  });

  it.each([
    { definitions: [] },
    { definitions: [{ ...definition, enabled: false }] },
    { definitions: [{ ...definition, workspace_id: "workspace-b" }] },
    { visibleDefinitionIds: [] },
  ])("never broadens inventory or search to highlight an unavailable row: %j", (change) => {
    expect(resolveMCPAssignmentHint({ ...input, ...change })).toEqual({
      status: "consume",
      definitionId: null,
    });
  });

  it("removes only the hint so reload cannot replay it", () => {
    const route = "/t/task-a?sessionId=session-a&mcpServerId=server-1&panel=files#details";
    const consumed = removeMCPAssignmentHint(route);
    expect(consumed).toBe("/t/task-a?sessionId=session-a&panel=files#details");
    expect(
      resolveMCPAssignmentHint({
        ...input,
        search: new URL(consumed, "https://example.test").search,
      }),
    ).toEqual({ status: "absent" });
  });
});
