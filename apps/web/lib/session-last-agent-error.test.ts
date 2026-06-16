import { beforeEach, describe, expect, it } from "vitest";

import {
  getStoredDismissedAgentErrors,
  lastAgentErrorStamp,
  readLastAgentError,
  setStoredDismissedAgentErrors,
} from "./session-last-agent-error";

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

describe("lastAgentErrorStamp", () => {
  it("combines occurredAt and message so a fresh error invalidates a prior dismissal", () => {
    expect(lastAgentErrorStamp({ message: AGENT_ERROR_MESSAGE, occurredAt: OCCURRED_AT })).toBe(
      `${OCCURRED_AT}:${AGENT_ERROR_MESSAGE}`,
    );
  });

  it("tolerates a missing occurredAt", () => {
    expect(lastAgentErrorStamp({ message: AGENT_ERROR_MESSAGE })).toBe(`:${AGENT_ERROR_MESSAGE}`);
  });
});

describe("setStoredDismissedAgentErrors", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("merges with existing entries so a concurrent tab's writes are not clobbered", () => {
    // Simulate a write from another tab landing first.
    setStoredDismissedAgentErrors({ "session-other-tab": "stamp-other" });

    // This tab then dismisses for a different session using a stale snapshot
    // (i.e. one that does not include `session-other-tab`).
    setStoredDismissedAgentErrors({ "session-this-tab": "stamp-this" });

    expect(getStoredDismissedAgentErrors()).toEqual({
      "session-other-tab": "stamp-other",
      "session-this-tab": "stamp-this",
    });
  });

  it("overwrites existing entries for the same session id", () => {
    setStoredDismissedAgentErrors({ "session-a": "stamp-v1" });
    setStoredDismissedAgentErrors({ "session-a": "stamp-v2" });
    expect(getStoredDismissedAgentErrors()).toEqual({ "session-a": "stamp-v2" });
  });
});
