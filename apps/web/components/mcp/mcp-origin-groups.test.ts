import { describe, expect, it } from "vitest";
import type { MCPInheritedSelection, MCPSelectionOrigin } from "@/lib/types/http-mcp";
import { groupMCPOrigins } from "./mcp-origin-groups";

function origin(scope: MCPSelectionOrigin["scope"], owner_id: string = scope): MCPSelectionOrigin {
  return { scope, owner_id, workspace_id: "workspace" };
}

function selection(id: string, origins: MCPSelectionOrigin[]): MCPInheritedSelection {
  return {
    definition: {
      id,
      workspace_id: "workspace",
      display_name: id,
      runtime_name: id,
      normalized_runtime_name: id,
      enabled: true,
      execution_mode: "remote",
      transport: "sse",
      source: "custom",
      configuration: {},
      revision: 1,
      created_at: "",
      updated_at: "",
    },
    origins,
  };
}

describe("Customize origin sections", () => {
  // @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.20
  it("groups each server by its closest source while keeping every contributor", () => {
    const shared = selection("shared", [origin("profile"), origin("repository"), origin("task")]);
    const repository = selection("repository", [origin("profile"), origin("repository")]);
    const profile = selection("profile", [origin("profile")]);
    expect(groupMCPOrigins([profile, repository, shared])).toEqual([
      { scope: "task", items: [shared] },
      { scope: "repository", items: [repository] },
      { scope: "profile", items: [profile] },
    ]);
  });

  it.each([
    [["task"], "task"],
    [["repository"], "repository"],
    [["profile"], "profile"],
    [["profile", "task"], "task"],
    [["repository", "task"], "task"],
    [["profile", "repository"], "repository"],
    [["profile", "repository", "task"], "task"],
  ] as const)("places %j in %s without empty groups", (scopes, expected) => {
    const item = selection(
      "server",
      scopes.map((scope) => origin(scope)),
    );
    expect(groupMCPOrigins([item])).toEqual([{ scope: expected, items: [item] }]);
  });

  it("merges duplicate IDs and contributor tuples, retaining multiple repositories", () => {
    const repoA = origin("repository", "api");
    const repoB = origin("repository", "operations");
    const input = [
      selection("shared", [repoA, repoA, origin("profile")]),
      selection("shared", [repoB, repoA, origin("task")]),
    ];
    const before = structuredClone(input);
    expect(groupMCPOrigins(input)).toEqual([
      {
        scope: "task",
        items: [{ ...input[0], origins: [repoA, origin("profile"), repoB, origin("task")] }],
      },
    ]);
    expect(input).toEqual(before);
  });

  it("retains order and disabled inherited membership", () => {
    const first = selection("z-first", [origin("repository")]);
    const disabled = selection("a-disabled", [origin("repository")]);
    disabled.definition.enabled = false;
    expect(
      groupMCPOrigins([first, selection("profile", [origin("profile")]), disabled])[0],
    ).toEqual({ scope: "repository", items: [first, disabled] });
  });

  it("keeps unattributed and session-only origins visible without inventing task selection", () => {
    const unknown = selection("unknown", []);
    const session = selection("session", [origin("task_session")]);
    expect(groupMCPOrigins([unknown, session])).toEqual([
      { scope: "unknown", items: [unknown, session] },
    ]);
  });

  it("does not merge distinct workspaces' contributor identities", () => {
    const local = origin("repository");
    const foreign = { ...local, workspace_id: "other-workspace" };
    const item = selection("shared", [local, foreign]);
    expect(groupMCPOrigins([item])[0]?.items[0]?.origins).toEqual([local, foreign]);
  });

  it("omits all origin sections when there are no inherited servers", () => {
    expect(groupMCPOrigins([])).toEqual([]);
  });
});
