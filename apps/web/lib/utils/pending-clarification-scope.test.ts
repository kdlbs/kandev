import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { findPendingClarification, findPendingClarificationGroup } from "./pending-clarification";

const CURRENT_TURN_ID = "turn-new";

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

describe("completed-turn clarification ownership", () => {
  it("does not use a detached clarification while the current action is scoped", () => {
    const detached = message({
      id: "detached",
      turn_id: "turn-old",
      type: "clarification_request",
      metadata: { status: "pending", agent_disconnected: true },
    });
    const newer = message({ id: "newer", turn_id: CURRENT_TURN_ID, type: "message" });
    const scope = { currentTurnId: CURRENT_TURN_ID, pendingAction: "clarification" as const };

    expect(findPendingClarification([detached, newer], scope)).toBeNull();
    expect(findPendingClarificationGroup([detached, newer], scope)).toEqual([]);
  });

  it("supersedes a detached clarification after the newer turn completes", () => {
    const detached = message({
      id: "detached",
      turn_id: "turn-old",
      type: "clarification_request",
      metadata: { status: "pending", agent_disconnected: true },
    });
    const newer = message({ id: "newer", turn_id: CURRENT_TURN_ID, type: "message" });
    const scope = {
      currentTurnId: CURRENT_TURN_ID,
      currentTurnCompleted: true,
      pendingAction: null,
    } as const;

    expect(findPendingClarification([detached, newer], scope)).toBeNull();
    expect(findPendingClarificationGroup([detached, newer], scope)).toEqual([]);
  });
});
