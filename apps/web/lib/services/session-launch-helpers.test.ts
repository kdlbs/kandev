import { describe, expect, it } from "vitest";
import { buildStartRequest } from "./session-launch-helpers";

describe("buildStartRequest", () => {
  it("serializes a canonical task priority string", () => {
    const { request } = buildStartRequest("task-1", "agent-1", {
      priority: "critical",
    });

    expect(request.priority).toBe("critical");
  });
});
