import { beforeEach, describe, expect, it } from "vitest";

import {
  getStoredAcknowledgedAgentErrors,
  getStoredDismissedAgentErrors,
  lastAgentErrorStamp,
  readLastAgentError,
  setStoredAcknowledgedAgentErrors,
  setStoredDismissedAgentErrors,
} from "./session-last-agent-error";

const AGENT_ERROR_MESSAGE = "agent process exited";
const AGENT_EXECUTION_ID = "exec-1";
const OCCURRED_AT = "2026-06-14T12:00:00Z";
const OTHER_TAB_SESSION_ID = "session-other-tab";
const THIS_TAB_SESSION_ID = "session-this-tab";
const OTHER_TAB_STAMP = "stamp-other";
const THIS_TAB_STAMP = "stamp-this";

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

  it("returns null when the server has marked the error dismissed", () => {
    expect(
      readLastAgentError({
        last_agent_error: {
          message: AGENT_ERROR_MESSAGE,
          occurred_at: OCCURRED_AT,
          dismissed_at: "2026-06-14T12:05:00Z",
        },
      }),
    ).toBeNull();
  });

  it("returns null when dismissal is provided as camelCase dismissedAt", () => {
    expect(
      readLastAgentError({
        last_agent_error: {
          message: AGENT_ERROR_MESSAGE,
          occurredAt: OCCURRED_AT,
          dismissedAt: "2026-06-14T12:05:00Z",
        },
      }),
    ).toBeNull();
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
    setStoredDismissedAgentErrors({ [OTHER_TAB_SESSION_ID]: OTHER_TAB_STAMP });

    // This tab then dismisses for a different session using a stale snapshot
    // (i.e. one that does not include `session-other-tab`).
    setStoredDismissedAgentErrors({ [THIS_TAB_SESSION_ID]: THIS_TAB_STAMP });

    expect(getStoredDismissedAgentErrors()).toEqual({
      [OTHER_TAB_SESSION_ID]: OTHER_TAB_STAMP,
      [THIS_TAB_SESSION_ID]: THIS_TAB_STAMP,
    });
  });

  it("overwrites existing entries for the same session id", () => {
    setStoredDismissedAgentErrors({ "session-a": "stamp-v1" });
    setStoredDismissedAgentErrors({ "session-a": "stamp-v2" });
    expect(getStoredDismissedAgentErrors()).toEqual({ "session-a": "stamp-v2" });
  });
});

describe("setStoredAcknowledgedAgentErrors", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("merges with existing sidebar acknowledgement entries", () => {
    setStoredAcknowledgedAgentErrors({ [OTHER_TAB_SESSION_ID]: OTHER_TAB_STAMP });
    setStoredAcknowledgedAgentErrors({ [THIS_TAB_SESSION_ID]: THIS_TAB_STAMP });

    expect(getStoredAcknowledgedAgentErrors()).toEqual({
      [OTHER_TAB_SESSION_ID]: OTHER_TAB_STAMP,
      [THIS_TAB_SESSION_ID]: THIS_TAB_STAMP,
    });
  });

  it("overwrites existing sidebar acknowledgement entries for the same session id", () => {
    setStoredAcknowledgedAgentErrors({ "session-a": "stamp-v1" });
    setStoredAcknowledgedAgentErrors({ "session-a": "stamp-v2" });
    expect(getStoredAcknowledgedAgentErrors()).toEqual({ "session-a": "stamp-v2" });
  });
});
