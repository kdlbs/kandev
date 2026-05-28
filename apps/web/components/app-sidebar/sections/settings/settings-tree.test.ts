import { describe, it, expect } from "vitest";
import { settingsGroupIdForPath } from "./settings-tree";

describe("settingsGroupIdForPath", () => {
  it("maps a group root to its id", () => {
    expect(settingsGroupIdForPath("/settings/executors")).toBe("executors");
    expect(settingsGroupIdForPath("/settings/agents")).toBe("agents");
    expect(settingsGroupIdForPath("/settings/general")).toBe("general");
    expect(settingsGroupIdForPath("/settings/integrations")).toBe("integrations");
    expect(settingsGroupIdForPath("/settings/system")).toBe("system");
    expect(settingsGroupIdForPath("/settings/workspace")).toBe("workspaces");
  });

  it("maps a nested path to its owning group", () => {
    expect(settingsGroupIdForPath("/settings/executors/profile-123")).toBe("executors");
    expect(settingsGroupIdForPath("/settings/workspace/ws-1/repositories")).toBe("workspaces");
    expect(settingsGroupIdForPath("/settings/system/logs")).toBe("system");
    // Editors/Secrets live under /settings/general so they belong to General.
    expect(settingsGroupIdForPath("/settings/general/editors")).toBe("general");
  });

  it("returns null for standalone leaves with no owning group", () => {
    expect(settingsGroupIdForPath("/settings/automations")).toBeNull();
    expect(settingsGroupIdForPath("/settings/prompts")).toBeNull();
    expect(settingsGroupIdForPath("/settings/utility-agents")).toBeNull();
    expect(settingsGroupIdForPath("/settings/external-mcp")).toBeNull();
    expect(settingsGroupIdForPath("/settings")).toBeNull();
  });
});
