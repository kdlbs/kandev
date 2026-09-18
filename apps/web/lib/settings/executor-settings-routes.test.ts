import { describe, expect, it } from "vitest";

import { executorProfileSettingsPath } from "./executor-settings-routes";

describe("executorProfileSettingsPath", () => {
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.1
  it("routes every profile to the canonical editor", () => {
    expect(executorProfileSettingsPath("profile/primary")).toBe(
      "/settings/executors/profile%2Fprimary",
    );
  });
});
