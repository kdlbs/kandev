import { describe, expect, it } from "vitest";

import type { Message, TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";
import { lastAgentErrorStamp } from "@/lib/session-last-agent-error";
import {
  agentErrorMessageForTask,
  resolvedAgentErrorAcknowledgementStamp,
} from "./task-agent-error";

const ERROR_OCCURRED_AT = "2026-06-14T10:00:00Z";
const AGENT_MESSAGE_AFTER_ERROR_AT = "2026-06-14T10:00:01Z";
const PRIMARY_ERROR = "primary error";
const PRIMARY_TASK = { id: "task-1", primarySessionId: "primary" };

function session(overrides: Partial<TaskSession>): TaskSession {
  return {
    id: sessionId("session-1"),
    task_id: taskId("task-1"),
    state: "WAITING_FOR_INPUT",
    started_at: "2026-06-14T10:00:00Z",
    updated_at: "2026-06-14T10:00:00Z",
    ...overrides,
  } as TaskSession;
}

function errorMetadata(message: string) {
  return {
    last_agent_error: {
      message,
      occurred_at: ERROR_OCCURRED_AT,
    },
  };
}

function primarySession(overrides: Partial<TaskSession> = {}): TaskSession {
  return session({
    id: sessionId("primary"),
    metadata: errorMetadata(PRIMARY_ERROR),
    ...overrides,
  });
}

function agentMessage(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: sessionId("primary"),
    task_id: taskId("task-1"),
    author_type: "agent",
    content: "",
    type: "agent_message",
    created_at: "2026-06-14T11:00:00Z",
    ...overrides,
  } as Message;
}

describe("agentErrorMessageForTask", () => {
  it("uses the explicit primary session even when it is terminal", () => {
    const primary = primarySession({ state: "COMPLETED" });
    expect(agentErrorMessageForTask(PRIMARY_TASK, { primary }, { "task-1": [] })).toBe(
      PRIMARY_ERROR,
    );
  });

  it("ignores stale terminal sessions in the fallback path", () => {
    const oldFailed = session({
      id: sessionId("old-failed"),
      state: "FAILED",
      updated_at: "2026-06-14T12:00:00Z",
      metadata: errorMetadata("old failure"),
    });
    const current = session({
      id: sessionId("current"),
      state: "WAITING_FOR_INPUT",
      updated_at: "2026-06-14T11:00:00Z",
      metadata: errorMetadata("current failure"),
    });

    expect(
      agentErrorMessageForTask(
        { id: "task-1" },
        { "old-failed": oldFailed, current },
        { "task-1": [oldFailed, current] },
      ),
    ).toBe("current failure");
  });

  it("hides the error when the matching stamp is in the dismissed map", () => {
    const primary = primarySession();
    const stamp = lastAgentErrorStamp({
      message: PRIMARY_ERROR,
      occurredAt: ERROR_OCCURRED_AT,
    });
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        { dismissedAgentErrors: { primary: stamp } },
      ),
    ).toBeNull();
  });

  it("hides the error when the matching stamp is sidebar-acknowledged", () => {
    const primary = primarySession();
    const stamp = lastAgentErrorStamp({
      message: PRIMARY_ERROR,
      occurredAt: ERROR_OCCURRED_AT,
    });
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        { acknowledgedAgentErrors: { primary: stamp } },
      ),
    ).toBeNull();
  });
});

describe("agentErrorMessageForTask (hide conditions)", () => {
  it("hides the error when the session metadata is server-dismissed", () => {
    const primary = primarySession({
      metadata: {
        last_agent_error: {
          message: PRIMARY_ERROR,
          occurred_at: ERROR_OCCURRED_AT,
          dismissed_at: "2026-06-14T10:05:00Z",
        },
      },
    });
    expect(agentErrorMessageForTask(PRIMARY_TASK, { primary }, { "task-1": [primary] })).toBeNull();
  });

  it("keeps the error when only an older stamp is dismissed", () => {
    const primary = primarySession();
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        { dismissedAgentErrors: { primary: "stale-stamp" } },
      ),
    ).toBe(PRIMARY_ERROR);
  });

  it("hides the error once an agent message arrives after the error timestamp", () => {
    const primary = primarySession();
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        {
          messagesBySession: {
            primary: [agentMessage({ created_at: AGENT_MESSAGE_AFTER_ERROR_AT })],
          },
        },
      ),
    ).toBeNull();
  });

  it("keeps the error when newer messages are from the user, not the agent", () => {
    const primary = primarySession();
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        {
          messagesBySession: {
            primary: [
              agentMessage({ author_type: "user", created_at: AGENT_MESSAGE_AFTER_ERROR_AT }),
            ],
          },
        },
      ),
    ).toBe(PRIMARY_ERROR);
  });
});

describe("resolvedAgentErrorAcknowledgementStamp", () => {
  it("returns a stamp when a later agent message makes the sidebar error stale", () => {
    const primary = primarySession();
    const stamp = lastAgentErrorStamp({
      message: PRIMARY_ERROR,
      occurredAt: ERROR_OCCURRED_AT,
    });

    expect(
      resolvedAgentErrorAcknowledgementStamp("primary", primary, {
        messagesBySession: {
          primary: [agentMessage({ created_at: AGENT_MESSAGE_AFTER_ERROR_AT })],
        },
      }),
    ).toBe(stamp);
  });

  it("does not return a stamp when the sidebar acknowledgement is already stored", () => {
    const primary = primarySession();
    const stamp = lastAgentErrorStamp({
      message: PRIMARY_ERROR,
      occurredAt: ERROR_OCCURRED_AT,
    });

    expect(
      resolvedAgentErrorAcknowledgementStamp("primary", primary, {
        acknowledgedAgentErrors: { primary: stamp },
        messagesBySession: {
          primary: [agentMessage({ created_at: AGENT_MESSAGE_AFTER_ERROR_AT })],
        },
      }),
    ).toBeNull();
  });
});

// Regression: string-based comparison of RFC3339 timestamps with mixed
// fractional-second precision (e.g. ".5" vs ".50") sorts incorrectly, which
// could leave a stale error icon visible or clear it too early. Numeric
// Date.parse() comparison treats them as equal moments in time.
describe("agentErrorMessageForTask (timestamp precision)", () => {
  it("does not clear the error when an agent message shares the same instant", () => {
    const errorAt = "2026-06-14T10:00:00.5Z";
    const messageAt = "2026-06-14T10:00:00.50Z"; // same instant, longer fractional
    const primary = primarySession({
      metadata: { last_agent_error: { message: PRIMARY_ERROR, occurred_at: errorAt } },
    });
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        { messagesBySession: { primary: [agentMessage({ created_at: messageAt })] } },
      ),
    ).toBe(PRIMARY_ERROR);
  });

  it("clears the error when an agent message is strictly later despite shorter fractional precision", () => {
    const errorAt = "2026-06-14T10:00:00.999Z";
    const messageAt = "2026-06-14T10:00:01.0Z";
    const primary = primarySession({
      metadata: { last_agent_error: { message: PRIMARY_ERROR, occurred_at: errorAt } },
    });
    expect(
      agentErrorMessageForTask(
        PRIMARY_TASK,
        { primary },
        { "task-1": [primary] },
        { messagesBySession: { primary: [agentMessage({ created_at: messageAt })] } },
      ),
    ).toBeNull();
  });
});

// Coverage for the fallback `sessionsByTaskId` path (no primarySessionId).
// shouldHideError is shared with the primary path, but exercising it through
// the fallback loop guards against any regression in the per-branch wiring.
describe("agentErrorMessageForTask (fallback session path)", () => {
  const FALLBACK_TASK = { id: "task-1" };
  const FALLBACK_ERROR = "fallback failure";

  function fallbackSession() {
    return session({
      id: sessionId("fallback"),
      metadata: errorMetadata(FALLBACK_ERROR),
    });
  }

  it("hides the error when the matching stamp is dismissed", () => {
    const fallback = fallbackSession();
    const stamp = lastAgentErrorStamp({
      message: FALLBACK_ERROR,
      occurredAt: ERROR_OCCURRED_AT,
    });
    expect(
      agentErrorMessageForTask(
        FALLBACK_TASK,
        { fallback },
        { "task-1": [fallback] },
        { dismissedAgentErrors: { fallback: stamp } },
      ),
    ).toBeNull();
  });

  it("hides the error once a later agent message arrives", () => {
    const fallback = fallbackSession();
    expect(
      agentErrorMessageForTask(
        FALLBACK_TASK,
        { fallback },
        { "task-1": [fallback] },
        {
          messagesBySession: {
            fallback: [
              agentMessage({
                session_id: sessionId("fallback"),
                created_at: "2026-06-14T11:00:00Z",
              }),
            ],
          },
        },
      ),
    ).toBeNull();
  });
});
