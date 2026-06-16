import { describe, expect, it } from "vitest";

import type { Message, TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";
import { lastAgentErrorStamp } from "@/lib/session-last-agent-error";
import { agentErrorMessageForTask } from "./task-agent-error";

const ERROR_OCCURRED_AT = "2026-06-14T10:00:00Z";
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
        { messagesBySession: { primary: [agentMessage({ created_at: "2026-06-14T10:00:01Z" })] } },
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
            primary: [agentMessage({ author_type: "user", created_at: "2026-06-14T10:00:01Z" })],
          },
        },
      ),
    ).toBe(PRIMARY_ERROR);
  });
});
