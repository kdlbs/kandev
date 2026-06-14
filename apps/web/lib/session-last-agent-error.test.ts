import { describe, expect, it } from "vitest";

import { lastAgentErrorDismissKey, readLastAgentError } from "./session-last-agent-error";

const AGENT_ERROR_MESSAGE = "agent process exited";
const AGENT_EXECUTION_ID = "exec-1";
const OCCURRED_AT = "2026-06-14T12:00:00Z";

describe("readLastAgentError", () => {
  it("reads snake_case metadata and keeps occurredAt optional", () => {
    expect(
      readLastAgentError({
        last_agent_error: {
          message: AGENT_ERROR_MESSAGE,
          agent_execution_id: AGENT_EXECUTION_ID,
        },
      }),
    ).toEqual({
      message: AGENT_ERROR_MESSAGE,
      agentExecutionId: AGENT_EXECUTION_ID,
    });
  });

  it("reads camelCase metadata after a store round trip", () => {
    expect(
      readLastAgentError({
        last_agent_error: {
          message: AGENT_ERROR_MESSAGE,
          occurredAt: OCCURRED_AT,
          agentExecutionId: AGENT_EXECUTION_ID,
        },
      }),
    ).toEqual({
      message: AGENT_ERROR_MESSAGE,
      occurredAt: OCCURRED_AT,
      agentExecutionId: AGENT_EXECUTION_ID,
    });
  });
});

describe("lastAgentErrorDismissKey", () => {
  it("includes both timestamp and message in the dismissal stamp", () => {
    expect(
      lastAgentErrorDismissKey("session-1", {
        message: AGENT_ERROR_MESSAGE,
        occurredAt: OCCURRED_AT,
      }),
    ).toBe(`kandev:last-agent-error-dismissed:session-1:${OCCURRED_AT}:${AGENT_ERROR_MESSAGE}`);
  });
});
