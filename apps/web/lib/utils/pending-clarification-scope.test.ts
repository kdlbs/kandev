import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { findPendingClarification, findPendingClarificationGroup } from "./pending-clarification";

const OLD_TURN_ID = "turn-old";
const CURRENT_TURN_ID = "turn-new";
const OLD_PENDING_ID = "pending-old";
const CURRENT_PENDING_ID = "pending-current";
const OLD_QUESTION_ID = "old-question";
const CURRENT_QUESTION_ID = "current-question";
const DETACHED_METADATA = { agent_disconnected: true };
const CURRENT_ACTIVITY_ID = "current-activity";

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

function clarificationMessage(
  id: string,
  turnId: string,
  pendingId: string,
  metadata: Record<string, unknown> = {},
): Message {
  return message({
    id,
    turn_id: turnId,
    type: "clarification_request",
    metadata: { pending_id: pendingId, status: "pending", ...metadata },
  });
}

function selection(
  messages: readonly Message[],
  scope: Parameters<typeof findPendingClarification>[1],
): { selected: string | null; bundle: string[] } {
  return {
    selected: findPendingClarification(messages, scope)?.id ?? null,
    bundle: findPendingClarificationGroup(messages, scope).map((item) => item.id),
  };
}

const CLEAN_CURRENT_SCOPE = {
  currentTurnId: CURRENT_TURN_ID,
  currentTurnCompleted: false,
  pendingAction: null,
} as const;

describe("superseded detached clarification selection", () => {
  it("does not reactivate an older detached bundle after the current bundle is answered", () => {
    const messages = [
      clarificationMessage(OLD_QUESTION_ID, OLD_TURN_ID, OLD_PENDING_ID, DETACHED_METADATA),
      clarificationMessage(CURRENT_QUESTION_ID, CURRENT_TURN_ID, CURRENT_PENDING_ID, {
        status: "answered",
      }),
      message({ id: CURRENT_ACTIVITY_ID, turn_id: CURRENT_TURN_ID }),
    ];

    expect(selection(messages, CLEAN_CURRENT_SCOPE)).toEqual({ selected: null, bundle: [] });
  });

  it("does not reactivate an older detached bundle after the current bundle is rejected", () => {
    const messages = [
      clarificationMessage(OLD_QUESTION_ID, OLD_TURN_ID, OLD_PENDING_ID, DETACHED_METADATA),
      clarificationMessage(CURRENT_QUESTION_ID, CURRENT_TURN_ID, CURRENT_PENDING_ID, {
        status: "rejected",
      }),
      message({ id: CURRENT_ACTIVITY_ID, turn_id: CURRENT_TURN_ID }),
    ];

    expect(selection(messages, CLEAN_CURRENT_SCOPE)).toEqual({ selected: null, bundle: [] });
  });

  it.each([null, "clarification"] as const)(
    "does not infer older turn ownership after newer-turn messages disappear (projection %s)",
    (pendingAction) => {
      const messages = [
        clarificationMessage(OLD_QUESTION_ID, OLD_TURN_ID, OLD_PENDING_ID, DETACHED_METADATA),
      ];

      expect(selection(messages, { currentTurnId: CURRENT_TURN_ID, pendingAction })).toEqual({
        selected: null,
        bundle: [],
      });
    },
  );

  it.each(["answered", "rejected"] as const)(
    "does not fall back to the older detached bundle while projection still says clarification (%s)",
    (status) => {
      const messages = [
        clarificationMessage(OLD_QUESTION_ID, OLD_TURN_ID, OLD_PENDING_ID, DETACHED_METADATA),
        clarificationMessage(CURRENT_QUESTION_ID, CURRENT_TURN_ID, CURRENT_PENDING_ID, { status }),
        message({ id: CURRENT_ACTIVITY_ID, turn_id: CURRENT_TURN_ID }),
      ];
      const scope = {
        currentTurnId: CURRENT_TURN_ID,
        currentTurnCompleted: false,
        pendingAction: "clarification",
      } as const;

      expect(selection(messages, scope)).toEqual({ selected: null, bundle: [] });
    },
  );
});

describe("current detached clarification authority", () => {
  it("keeps a current detached bundle answerable within its completed turn", () => {
    const detached = clarificationMessage(
      CURRENT_QUESTION_ID,
      CURRENT_TURN_ID,
      CURRENT_PENDING_ID,
      DETACHED_METADATA,
    );
    const scope = {
      currentTurnId: CURRENT_TURN_ID,
      currentTurnCompleted: true,
      pendingAction: "clarification",
    } as const;

    expect(selection([detached], scope)).toEqual({
      selected: CURRENT_QUESTION_ID,
      bundle: [CURRENT_QUESTION_ID],
    });
  });

  it("keeps a current detached bundle answerable while its turn remains open", () => {
    const detached = clarificationMessage(
      CURRENT_QUESTION_ID,
      CURRENT_TURN_ID,
      CURRENT_PENDING_ID,
      DETACHED_METADATA,
    );
    const scope = {
      currentTurnId: CURRENT_TURN_ID,
      currentTurnCompleted: false,
      pendingAction: "clarification",
    } as const;

    expect(selection([detached], scope)).toEqual({
      selected: CURRENT_QUESTION_ID,
      bundle: [CURRENT_QUESTION_ID],
    });
  });

  it("suppresses a current detached bundle when the projection clears", () => {
    const detached = clarificationMessage(
      CURRENT_QUESTION_ID,
      CURRENT_TURN_ID,
      CURRENT_PENDING_ID,
      DETACHED_METADATA,
    );

    expect(selection([detached], CLEAN_CURRENT_SCOPE)).toEqual({ selected: null, bundle: [] });
  });

  it("suppresses discovery for a loaded session without an eligible turn", () => {
    const current = clarificationMessage(CURRENT_QUESTION_ID, CURRENT_TURN_ID, CURRENT_PENDING_ID);

    expect(selection([current], { currentTurnId: null, pendingAction: "clarification" })).toEqual({
      selected: null,
      bundle: [],
    });
  });

  it("keeps a legacy current row without status eligible in the authoritative turn", () => {
    const current = message({
      id: "legacy-question",
      turn_id: CURRENT_TURN_ID,
      type: "clarification_request",
      metadata: { pending_id: CURRENT_PENDING_ID },
    });

    expect(selection([current], { currentTurnId: CURRENT_TURN_ID })).toEqual({
      selected: "legacy-question",
      bundle: ["legacy-question"],
    });
  });

  it("uses the newest message turn as the fallback when only the projection is available", () => {
    const olderDetached = clarificationMessage(
      OLD_QUESTION_ID,
      OLD_TURN_ID,
      OLD_PENDING_ID,
      DETACHED_METADATA,
    );
    const newerActivity = message({
      id: CURRENT_ACTIVITY_ID,
      turn_id: CURRENT_TURN_ID,
      created_at: "2026-05-02T00:01:00Z",
    });

    expect(selection([olderDetached, newerActivity], { pendingAction: "clarification" })).toEqual({
      selected: null,
      bundle: [],
    });
    expect(selection([olderDetached], { pendingAction: "clarification" })).toEqual({
      selected: OLD_QUESTION_ID,
      bundle: [OLD_QUESTION_ID],
    });
  });

  it("keeps an earlier pending bundle in the authoritative turn when a newer one resolves", () => {
    const messages = [
      clarificationMessage("current-pending", CURRENT_TURN_ID, "pending-earlier"),
      clarificationMessage("current-resolved", CURRENT_TURN_ID, "pending-later", {
        status: "answered",
      }),
    ];

    expect(
      selection(messages, { currentTurnId: CURRENT_TURN_ID, pendingAction: "clarification" }),
    ).toEqual({ selected: "current-pending", bundle: ["current-pending"] });
  });
});
