import { describe, expect, it } from "vitest";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import { findPendingClarification, findPendingClarificationGroup } from "./pending-clarification";

const TURN_ID = "turn-detached";
const PENDING_ID = "pending-detached";

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: "message-detached",
    session_id: sessionId("session-detached"),
    task_id: taskId("task-detached"),
    turn_id: TURN_ID,
    author_type: "agent",
    type: "clarification_request",
    content: "Which database should we use?",
    created_at: "2026-09-12T00:00:00Z",
    updated_at: "2026-09-12T00:00:00Z",
    metadata: {
      pending_id: PENDING_ID,
      status: "pending",
      agent_disconnected: true,
    },
    ...overrides,
  };
}

describe("detached clarification ownership", () => {
  it("keeps an explicitly detached request visible after pending action clears", () => {
    const detached = message();
    const scope = { currentTurnId: TURN_ID, pendingAction: null } as const;

    expect(findPendingClarification([detached], scope)).toBe(detached);
    expect(findPendingClarificationGroup([detached], scope)).toEqual([detached]);
  });
});
