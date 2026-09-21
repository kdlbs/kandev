import { describe, expect, it } from "vitest";
import { shouldApplyAgentTabCloseBehavior } from "./agent-tab-close-behavior-settings";

describe("agent tab close behavior saves", () => {
  it("does not overwrite a newer preference received while saving", () => {
    expect(shouldApplyAgentTabCloseBehavior("hide_panel", "delete_session")).toBe(false);
  });

  it("applies the submitted preference when it is still current", () => {
    expect(shouldApplyAgentTabCloseBehavior("hide_panel", "hide_panel")).toBe(true);
  });
});
