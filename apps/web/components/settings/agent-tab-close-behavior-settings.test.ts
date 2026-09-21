import { describe, expect, it } from "vitest";
import { shouldApplyAgentTabCloseBehavior } from "./agent-tab-close-behavior-settings";

describe("agent tab close behavior saves", () => {
  it("does not overwrite a newer preference received while saving", () => {
    expect(shouldApplyAgentTabCloseBehavior(2, 3, false)).toBe(false);
  });

  it("applies the submitted preference when it is still current", () => {
    expect(shouldApplyAgentTabCloseBehavior(3, 2, false)).toBe(true);
    expect(shouldApplyAgentTabCloseBehavior(undefined, undefined, true)).toBe(true);
  });
});
