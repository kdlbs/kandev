import { describe, expect, it } from "vitest";

import { getWorkspaceSettingsTabs, workspaceSettingsHref } from "./workspace-settings-tabs";

describe("getWorkspaceSettingsTabs", () => {
  it("omits coordinators when the coordinator flag is off", () => {
    const tabs = getWorkspaceSettingsTabs(true, false);
    expect(tabs.map((entry) => entry.tab)).not.toContain("coordinators");
  });

  it("includes coordinators, after secrets, when the coordinator flag is on", () => {
    const tabs = getWorkspaceSettingsTabs(true, true);
    const tabNames = tabs.map((entry) => entry.tab);
    expect(tabNames).toContain("coordinators");
    expect(tabNames.indexOf("coordinators")).toBe(tabNames.indexOf("secrets") + 1);
  });
});

describe("workspaceSettingsHref", () => {
  it("builds the coordinators tab href", () => {
    expect(workspaceSettingsHref("ws-1", "coordinators")).toBe(
      "/settings/workspaces/ws-1/coordinators",
    );
  });
});
