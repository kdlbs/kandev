import { describe, expect, it } from "vitest";

import type { TaskSession } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";
import { agentErrorMessageForTask } from "./task-agent-error";

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
      occurred_at: "2026-06-14T10:00:00Z",
    },
  };
}

describe("agentErrorMessageForTask", () => {
  it("uses the explicit primary session even when it is terminal", () => {
    const primary = session({
      id: sessionId("primary"),
      state: "COMPLETED",
      metadata: errorMetadata("primary error"),
    });

    expect(
      agentErrorMessageForTask(
        { id: "task-1", primarySessionId: "primary" },
        { primary },
        { "task-1": [] },
      ),
    ).toBe("primary error");
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
});
