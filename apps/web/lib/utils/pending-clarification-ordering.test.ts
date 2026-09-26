import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { hasPendingClarification } from "./pending-clarification";

function message(overrides: Partial<Message>): Message {
  return {
    id: "message",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

describe("pending clarification message ordering", () => {
  it("keeps a current-turn request visible after a delayed older-turn event", () => {
    const active = message({
      id: "active",
      turn_id: "t2",
      type: "clarification_request",
      metadata: { status: "pending" },
      created_at: "2026-05-02T00:00:02Z",
    });
    const delayedOldEvent = message({
      id: "delayed-old-event",
      turn_id: "t1",
      created_at: "2026-05-02T00:00:01Z",
    });

    expect(hasPendingClarification([active, delayedOldEvent])).toBe(true);
  });
});
