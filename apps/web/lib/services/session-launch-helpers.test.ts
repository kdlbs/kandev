import { describe, expect, it } from "vitest";
import { buildStartRequest } from "./session-launch-helpers";

describe("buildStartRequest", () => {
  it("omits profile_explicit unless the caller opts in", () => {
    const { request } = buildStartRequest("task-1", "profile-a");

    expect(request).not.toHaveProperty("profile_explicit");
  });

  it("serializes an explicit profile selection", () => {
    const { request } = buildStartRequest("task-1", "profile-b", { profileExplicit: true });

    expect(request).toHaveProperty("profile_explicit", true);
  });

  it("serializes a canonical task priority string", () => {
    const { request } = buildStartRequest("task-1", "agent-1", {
      priority: "critical",
    });

    expect(request.priority).toBe("critical");
  });
});
