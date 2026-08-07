import { describe, expect, it } from "vitest";
import { buildRunErrorsFromSessions } from "./chat-entries";
import type { TaskSession } from "@/app/office/tasks/[id]/types";

const URL = "https://opencode.ai/workspace/wrk_01KQM7K5CYT715264YKKFB17ZY/go";

function session(overrides: Partial<TaskSession>): TaskSession {
  return {
    id: "s1",
    agentName: "CEO",
    agentRole: "agent",
    state: "FAILED",
    isPrimary: false,
    startedAt: "2026-08-02T15:00:00Z",
    errorMessage: "usage limit reached",
    ...overrides,
  };
}

describe("buildRunErrorsFromSessions", () => {
  it("preserves the remediation URL from last_agent_error metadata", () => {
    const errors = buildRunErrorsFromSessions([
      session({
        metadata: {
          last_agent_error: {
            message: "usage limit reached",
            remediation_url: URL,
          },
        },
      }),
    ]);
    expect(errors).toHaveLength(1);
    expect(errors[0].remediationUrl).toBe(URL);
    expect(errors[0].rawPayload).toBe("usage limit reached");
  });

  it("reads the camelCase form after a store round trip", () => {
    const errors = buildRunErrorsFromSessions([
      session({ metadata: { last_agent_error: { message: "boom", remediationUrl: URL } } }),
    ]);
    expect(errors[0].remediationUrl).toBe(URL);
  });

  it("omits the URL when metadata or the field is absent", () => {
    for (const s of [
      session({ errorMessage: "boom" }),
      session({ metadata: { last_agent_error: { message: "boom" } } }),
      session({ metadata: { last_agent_error: { message: "boom", remediation_url: "" } } }),
    ]) {
      const errors = buildRunErrorsFromSessions([s]);
      expect(errors[0].remediationUrl).toBeUndefined();
    }
  });

  it("skips non-FAILED sessions", () => {
    const errors = buildRunErrorsFromSessions([
      session({ state: "RUNNING", metadata: { last_agent_error: { remediation_url: URL } } }),
    ]);
    expect(errors).toHaveLength(0);
  });
});
