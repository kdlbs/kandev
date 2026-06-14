import { describe, expect, it } from "vitest";

import { lastAgentErrorDismissKey, readLastAgentError } from "./session-last-agent-error";

describe("readLastAgentError", () => {
  it("reads snake_case metadata and keeps occurredAt optional", () => {
    expect(
      readLastAgentError({
        last_agent_error: {
          message: "agent process exited",
          agent_execution_id: "exec-1",
        },
      }),
    ).toEqual({
      message: "agent process exited",
      agentExecutionId: "exec-1",
    });
  });
});

describe("lastAgentErrorDismissKey", () => {
  it("includes both timestamp and message in the dismissal stamp", () => {
    expect(
      lastAgentErrorDismissKey("session-1", {
        message: "agent process exited",
        occurredAt: "2026-06-14T12:00:00Z",
      }),
    ).toBe("kandev:last-agent-error-dismissed:session-1:2026-06-14T12:00:00Z:agent process exited");
  });
});
