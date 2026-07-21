import { describe, it, expect } from "vitest";
import { settingsGroupIdForPath, settingsOpenGroupIdForPath } from "./settings-tree";

describe("settingsGroupIdForPath", () => {
  it("maps a group root to its id", () => {
    expect(settingsGroupIdForPath("/settings/executors")).toBe("executors");
    expect(settingsGroupIdForPath("/settings/agents")).toBe("agents");
    expect(settingsGroupIdForPath("/settings/general")).toBe("general");
    expect(settingsGroupIdForPath("/settings/system")).toBe("system");
    expect(settingsGroupIdForPath("/settings/workspace")).toBe("workspaces");
  });

  it("maps a nested path to its owning group", () => {
    expect(settingsGroupIdForPath("/settings/executors/profile-123")).toBe("executors");
    expect(settingsGroupIdForPath("/settings/workspace/ws-1/repositories")).toBe("workspaces");
    expect(settingsGroupIdForPath("/settings/workspace/ws-1/integrations/github")).toBe(
      "workspaces",
    );
    expect(settingsGroupIdForPath("/settings/system/logs")).toBe("system");
    expect(settingsGroupIdForPath("/settings/system/feature-toggles")).toBe("system");
    // General subpages stay under /settings/general so they belong to General.
    expect(settingsGroupIdForPath("/settings/general/appearance")).toBe("general");
    expect(settingsGroupIdForPath("/settings/general/terminal")).toBe("general");
    expect(settingsGroupIdForPath("/settings/general/keyboard-shortcuts")).toBe("general");
    expect(settingsGroupIdForPath("/settings/general/editors")).toBe("general");
  });

  it("returns null for standalone leaves with no owning group", () => {
    expect(settingsGroupIdForPath("/settings/automations")).toBeNull();
    expect(settingsGroupIdForPath("/settings/integrations")).toBeNull();
    expect(settingsGroupIdForPath("/settings/integrations/github")).toBeNull();
    expect(settingsGroupIdForPath("/settings/prompts")).toBeNull();
    expect(settingsGroupIdForPath("/settings/voice-mode")).toBeNull();
    expect(settingsGroupIdForPath("/settings/utility-agents")).toBeNull();
    expect(settingsGroupIdForPath("/settings/external-mcp")).toBeNull();
    expect(settingsGroupIdForPath("/settings")).toBeNull();
  });
});

describe("settingsOpenGroupIdForPath", () => {
  it("defaults the settings tree to Workspaces when no other group owns the route", () => {
    expect(settingsOpenGroupIdForPath("/")).toBe("workspaces");
    expect(settingsOpenGroupIdForPath("/settings")).toBe("workspaces");
    expect(settingsOpenGroupIdForPath("/settings/integrations/github")).toBe("workspaces");
  });

  it("keeps routed groups open when a route belongs to a different settings group", () => {
    expect(settingsOpenGroupIdForPath("/settings/general")).toBe("general");
    expect(settingsOpenGroupIdForPath("/settings/system/status")).toBe("system");
  });
});
