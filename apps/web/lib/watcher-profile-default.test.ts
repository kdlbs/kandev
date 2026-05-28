import { describe, it, expect } from "vitest";
import { STEP_DEFAULT, resolveProfileId } from "./watcher-profile-default";

describe("resolveProfileId", () => {
  it("collapses the sentinel to an empty string so the payload signals 'use step default'", () => {
    expect(resolveProfileId(STEP_DEFAULT)).toBe("");
  });

  it("passes a real profile id through unchanged", () => {
    expect(resolveProfileId("agent-profile-123")).toBe("agent-profile-123");
    expect(resolveProfileId("exec-profile-abc")).toBe("exec-profile-abc");
  });
});
