import { describe, expect, it } from "vitest";
import { isProfileDirty } from "./agent-profile-dirty";
import type { AgentProfile, PermissionSetting } from "@/lib/types/http";

const permissions: Record<string, PermissionSetting> = {};
const testServerId = "plugin-atlassian-jira";

function profile(overrides: Record<string, unknown> = {}): AgentProfile {
  return {
    id: "profile-1" as AgentProfile["id"],
    agentId: "agent-1",
    name: "Cursor profile",
    agentDisplayName: "Cursor",
    model: "model",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: "",
    updatedAt: "",
    ...overrides,
  } as AgentProfile;
}

describe("isProfileDirty Cursor MCP selection", () => {
  it("marks a changed selection mode dirty", () => {
    const saved = profile({ mcpSelectionMode: "inherit" });

    expect(
      isProfileDirty(profile({ ...saved, mcpSelectionMode: "selected" }), saved, permissions),
    ).toBe(true);
  });

  it("marks a changed selected-server list dirty", () => {
    const saved = profile({ mcpSelectedServers: [testServerId] });

    expect(
      isProfileDirty(
        profile({ ...saved, mcpSelectedServers: [testServerId, "filesystem"] }),
        saved,
        permissions,
      ),
    ).toBe(true);
  });

  it("does not treat a replaced array with the same IDs as dirty", () => {
    expect(
      isProfileDirty(
        profile({ mcpSelectedServers: [testServerId] }),
        profile({ mcpSelectedServers: [testServerId] }),
        permissions,
      ),
    ).toBe(false);
  });
});
