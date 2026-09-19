import { describe, expect, it } from "vitest";

import { getWorkspaceSettingsTabs } from "./workspace-settings-tabs";

describe("getWorkspaceSettingsTabs", () => {
  it("hides coordinator settings when coordinator authority is disabled", () => {
    const tabs = getWorkspaceSettingsTabs(false).map(({ tab }) => tab);

    expect(tabs).not.toContain("coordinators");
  });

  it("shows coordinator settings when coordinator authority is enabled", () => {
    const tabs = getWorkspaceSettingsTabs(false, true).map(({ tab }) => tab);

    expect(tabs).toContain("coordinators");
  });
});
