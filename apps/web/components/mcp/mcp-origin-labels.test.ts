import { describe, expect, it } from "vitest";
import type { MCPSelectionOrigin } from "@/lib/types/http-mcp";
import { formatMCPOriginLabel, type MCPOriginContext } from "./mcp-origin-labels";

const WORKSPACE = "workspace-a";
const OTHER_WORKSPACE = "workspace-b";
const context: MCPOriginContext = {
  workspaceId: WORKSPACE,
  profiles: [
    { id: "profile-a", label: "Team agent", workspace_id: WORKSPACE },
    { id: "global-profile", label: "Global agent" },
    { id: "profile-b", label: "Private agent", workspace_id: OTHER_WORKSPACE },
    { id: "same-name-profile", label: "Team agent", workspace_id: WORKSPACE },
  ],
  repositories: {
    [WORKSPACE]: [{ id: "repo-a", name: "Frontend", workspace_id: WORKSPACE }],
    [OTHER_WORKSPACE]: [
      { id: "repo-b", name: "Private repository", workspace_id: OTHER_WORKSPACE },
    ],
  },
  tasks: [
    { id: "task-a", title: "Fix selection", workspaceId: WORKSPACE, workflowId: "flow-a" },
    { id: "legacy-task", title: "Hydrated task", workflowId: "flow-a" },
    { id: "task-b", title: "Private task", workspaceId: OTHER_WORKSPACE, workflowId: "flow-b" },
  ],
  workflows: [{ id: "flow-a", workspaceId: WORKSPACE }],
  sessions: {
    "session-a": { name: "Research agent", task_id: "task-a" },
    "session-b": { name: "Private session", task_id: "task-b" },
    unnamed: { task_id: "task-a" },
  },
};

function origin(scope: MCPSelectionOrigin["scope"], owner_id: string): MCPSelectionOrigin {
  return { scope, owner_id, workspace_id: WORKSPACE };
}

describe("readable inherited MCP origins", () => {
  // @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.8
  it.each([
    ["profile", "profile-a", "Profile", "Profile: Team agent"],
    ["profile", "same-name-profile", "Profile", "Profile: Team agent"],
    ["profile", "global-profile", "Profile", "Profile: Global agent"],
    ["repository", "repo-a", "Repository", "Repository: Frontend"],
    ["task", "task-a", "Task", "Task: Fix selection"],
    ["task", "legacy-task", "Task", "Task: Hydrated task"],
    ["task_session", "session-a", "Session", "Session: Research agent"],
  ] as const)("resolves %s %s in its workspace", (scope, id, label, expected) => {
    expect(formatMCPOriginLabel(origin(scope, id), label, context)).toBe(expected);
  });

  it.each([
    ["profile", "profile-b"],
    ["repository", "repo-b"],
    ["task", "task-b"],
    ["task_session", "session-b"],
    ["task_session", "unnamed"],
    ["repository", "deleted-repo-uuid"],
  ] as const)("falls back to localized scope for unavailable %s %s", (scope, id) => {
    expect(formatMCPOriginLabel(origin(scope, id), "Origem traduzida", context)).toBe(
      "Origem traduzida",
    );
  });

  it("does not resolve another workspace's origin even for a global profile", () => {
    const foreign = { ...origin("profile", "global-profile"), workspace_id: OTHER_WORKSPACE };
    expect(formatMCPOriginLabel(foreign, "Profile", context)).toBe("Profile");
  });

  it("uses the latest hydrated name without a new selection or translation cache", () => {
    const renamed = {
      ...context,
      profiles: [{ id: "profile-a", label: "Renamed agent", workspace_id: WORKSPACE }],
    };
    expect(formatMCPOriginLabel(origin("profile", "profile-a"), "Perfil", renamed)).toBe(
      "Perfil: Renamed agent",
    );
  });
});
