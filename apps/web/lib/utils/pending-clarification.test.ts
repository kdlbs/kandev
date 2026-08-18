import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type Turn,
} from "@/lib/types/http";
import {
  clarificationTurnIdForSession,
  findPendingClarification,
  findPendingClarificationGroup,
  hasPendingClarification,
  hasPendingPermissionRequest,
  newestDurableTurnId,
} from "./pending-clarification";

const CURRENT_TURN_ID = "turn-new";
const OLD_PENDING_ID = "pending-old";
const TURN_TIMESTAMP = "2026-08-14T12:00:00Z";

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

function turn(id: string, startedAt = TURN_TIMESTAMP, createdAt = TURN_TIMESTAMP): Turn {
  return {
    id,
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    started_at: startedAt,
    created_at: createdAt,
    updated_at: createdAt,
  };
}

describe("hasPendingClarification", () => {
  it("detects clarification requests with pending status", () => {
    expect(
      hasPendingClarification([
        message({
          type: "clarification_request",
          metadata: { status: "pending" },
        }),
      ]),
    ).toBe(true);
  });

  it("treats missing clarification status as pending", () => {
    expect(hasPendingClarification([message({ type: "clarification_request" })])).toBe(true);
  });

  it("ignores answered clarification requests and ordinary messages", () => {
    expect(
      hasPendingClarification([
        message({ type: "message" }),
        message({
          type: "clarification_request",
          metadata: { status: "answered" },
        }),
      ]),
    ).toBe(false);
  });

  it("treats rejected and expired clarifications as not pending", () => {
    expect(
      hasPendingClarification([
        message({
          type: "clarification_request",
          metadata: { status: "rejected" },
        }),
      ]),
    ).toBe(false);
    expect(
      hasPendingClarification([
        message({
          type: "clarification_request",
          metadata: { status: "expired" },
        }),
      ]),
    ).toBe(false);
  });
});

describe("hasPendingClarification turn scoping", () => {
  it("ignores a detached pending clarification from a previous turn", () => {
    expect(
      hasPendingClarification([
        message({
          id: "old-detached",
          turn_id: "t1",
          type: "clarification_request",
          metadata: { status: "pending", agent_disconnected: true },
        }),
        message({
          id: "current",
          turn_id: "t2",
          type: "message",
          created_at: "2026-05-02T00:00:01Z",
        }),
      ]),
    ).toBe(false);
  });

  it("detects a pending clarification in the current turn", () => {
    expect(
      hasPendingClarification([
        message({
          id: "stale",
          turn_id: "t1",
          type: "clarification_request",
          metadata: { status: "pending" },
        }),
        message({
          id: "active",
          turn_id: "t2",
          type: "clarification_request",
          metadata: { status: "pending" },
          created_at: "2026-05-02T00:00:01Z",
        }),
      ]),
    ).toBe(true);
  });

  it("ignores a legacy null-turn clarification when a newer turn exists", () => {
    expect(
      hasPendingClarification([
        message({
          id: "legacy",
          type: "clarification_request",
          metadata: { status: "pending" },
        }),
        message({ id: "current", turn_id: "t2", type: "message" }),
      ]),
    ).toBe(false);
  });
});

describe("findPendingClarificationGroup", () => {
  it("returns empty array when there are no messages", () => {
    expect(findPendingClarificationGroup([])).toEqual([]);
    expect(findPendingClarificationGroup(null)).toEqual([]);
    expect(findPendingClarificationGroup(undefined)).toEqual([]);
  });

  it("returns empty array when there is no pending clarification", () => {
    expect(
      findPendingClarificationGroup([
        message({ type: "clarification_request", metadata: { status: "answered" } }),
      ]),
    ).toEqual([]);
  });

  it("returns the single message when no pending_id is set", () => {
    const msg = message({ type: "clarification_request" });
    expect(findPendingClarificationGroup([msg])).toEqual([msg]);
  });

  it("returns every message that shares the latest pending_id", () => {
    const a = message({
      id: "a",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 0, status: "pending" },
    });
    const b = message({
      id: "b",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 1, status: "pending" },
    });
    const c = message({
      id: "c",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 2, status: "pending" },
    });
    const noise = message({ id: "n", type: "message" });
    expect(findPendingClarificationGroup([noise, a, b, c]).map((m) => m.id)).toEqual([
      "a",
      "b",
      "c",
    ]);
  });

  it("gates on question_total — returns [] when not all messages have arrived", () => {
    // Backend declared 3 questions but only 1 has streamed in; the overlay
    // should stay hidden until the remaining 2 land.
    const a = message({
      id: "a",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 3, question_index: 0, status: "pending" },
    });
    expect(findPendingClarificationGroup([a])).toEqual([]);
  });

  it("ignores messages from a different pending bundle", () => {
    const old = message({
      id: "old",
      type: "clarification_request",
      metadata: { pending_id: "p0", status: "answered" },
    });
    const a = message({
      id: "a",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 1, status: "pending" },
    });
    expect(findPendingClarificationGroup([old, a]).map((m) => m.id)).toEqual(["a"]);
  });

  it("counts terminal siblings for completeness but returns only pending questions", () => {
    const answered = message({
      id: "answered",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 2, status: "answered" },
    });
    const pending = message({
      id: "pending",
      type: "clarification_request",
      metadata: { pending_id: "p1", question_total: 2, status: "pending" },
    });

    expect(findPendingClarificationGroup([answered, pending]).map((item) => item.id)).toEqual([
      "pending",
    ]);
  });
});

describe("current-turn clarification ownership", () => {
  it("ignores an older pending request when a newer durable turn exists", () => {
    const messages = [
      message({
        id: "old",
        turn_id: "turn-old",
        type: "clarification_request",
        metadata: { pending_id: OLD_PENDING_ID, status: "pending" },
      }),
      message({ id: "new", turn_id: CURRENT_TURN_ID, type: "message" }),
    ];
    expect(findPendingClarification(messages, { currentTurnId: CURRENT_TURN_ID })).toBeNull();
    expect(findPendingClarificationGroup(messages, { currentTurnId: CURRENT_TURN_ID })).toEqual([]);
  });

  it.each([null, "permission"] as const)(
    "treats a matching visible turn as inert when explicit authority is %s",
    (pendingAction) => {
      const pending = message({
        id: "visible-predecessor",
        turn_id: CURRENT_TURN_ID,
        type: "clarification_request",
        metadata: { pending_id: OLD_PENDING_ID, status: "pending" },
      });
      const scope = { currentTurnId: CURRENT_TURN_ID, pendingAction };

      expect(findPendingClarification([pending], scope)).toBeNull();
      expect(findPendingClarificationGroup([pending], scope)).toEqual([]);
    },
  );

  it("does not reactivate history when every newer-turn message is deleted", () => {
    const messages = [
      message({
        id: "old",
        turn_id: "turn-old",
        type: "clarification_request",
        metadata: { pending_id: OLD_PENDING_ID, status: "pending" },
      }),
    ];
    expect(findPendingClarification(messages, { currentTurnId: CURRENT_TURN_ID })).toBeNull();
  });

  it("returns only the current turn's exact pending bundle", () => {
    const messages = [
      message({
        id: "old",
        turn_id: "turn-old",
        type: "clarification_request",
        metadata: { pending_id: OLD_PENDING_ID, status: "pending" },
      }),
      message({
        id: "current-a",
        turn_id: CURRENT_TURN_ID,
        type: "clarification_request",
        metadata: { pending_id: "pending-new", question_total: 2, status: "pending" },
      }),
      message({
        id: "current-b",
        turn_id: CURRENT_TURN_ID,
        type: "clarification_request",
        metadata: { pending_id: "pending-new", question_total: 2, status: "pending" },
      }),
    ];
    expect(
      findPendingClarificationGroup(messages, { currentTurnId: CURRENT_TURN_ID }).map(
        (item) => item.id,
      ),
    ).toEqual(["current-a", "current-b"]);
  });

  it("keeps an earlier pending bundle visible when a newer bundle is terminal", () => {
    const messages = [
      message({
        id: "pending",
        turn_id: CURRENT_TURN_ID,
        type: "clarification_request",
        metadata: { pending_id: "pending-1", status: "pending" },
      }),
      message({
        id: "rejected",
        turn_id: CURRENT_TURN_ID,
        type: "clarification_request",
        metadata: { pending_id: "pending-2", status: "rejected" },
      }),
    ];
    const scope = { currentTurnId: CURRENT_TURN_ID };
    expect(findPendingClarification(messages, scope)?.id).toBe("pending");
    expect(findPendingClarificationGroup(messages, scope).map((item) => item.id)).toEqual([
      "pending",
    ]);
  });
});

describe("newestDurableTurnId load states", () => {
  it("distinguishes unavailable, empty, and populated durable turn history", () => {
    expect(newestDurableTurnId()).toBeUndefined();
    expect(newestDurableTurnId([])).toBeNull();
    expect(newestDurableTurnId([turn("turn-only")])).toBe("turn-only");
  });

  it("selects the turn with the later start time", () => {
    expect(
      newestDurableTurnId([
        turn("turn-id-at-noon", "2026-08-14T12:00:00Z"),
        turn("turn-id-one-second-later", "2026-08-14T12:00:01Z"),
      ]),
    ).toBe("turn-id-one-second-later");
  });
});

describe("clarificationTurnIdForSession", () => {
  it("quarantines pending message history for terminal sessions", () => {
    const turns = [turn(CURRENT_TURN_ID)];
    expect(clarificationTurnIdForSession("COMPLETED", turns)).toBeNull();
    expect(clarificationTurnIdForSession("FAILED", turns)).toBeNull();
    expect(clarificationTurnIdForSession("CANCELLED", turns)).toBeNull();
  });

  it("keeps current-turn authority for input-capable sessions", () => {
    expect(clarificationTurnIdForSession("RUNNING", [turn(CURRENT_TURN_ID)])).toBe(CURRENT_TURN_ID);
  });
});

describe("clarification authority fallbacks", () => {
  it("uses compact session authority while turn history is unavailable", () => {
    const messages = [message({ type: "clarification_request", metadata: { status: "pending" } })];
    expect(findPendingClarification(messages, { pendingAction: null })).toBeNull();
    expect(findPendingClarification(messages, { pendingAction: "clarification" })?.id).toBe(
      "msg-1",
    );
  });

  it("treats loaded empty turn history as having no active clarification", () => {
    const messages = [message({ type: "clarification_request", metadata: { status: "pending" } })];
    expect(findPendingClarification(messages, { currentTurnId: null })).toBeNull();
  });

  it("applies every turn-authority state to direct pending checks", () => {
    const old = message({
      id: "old",
      turn_id: "turn-old",
      type: "clarification_request",
      metadata: { status: "pending" },
    });
    const current = message({
      id: "current",
      turn_id: CURRENT_TURN_ID,
      type: "clarification_request",
      metadata: { status: "pending" },
    });

    expect(hasPendingClarification([old, current], { currentTurnId: null })).toBe(false);
    expect(
      hasPendingClarification([old, current], {
        currentTurnId: undefined,
        pendingAction: "clarification",
      }),
    ).toBe(true);
    expect(hasPendingClarification([old, current], { currentTurnId: undefined })).toBe(false);
    expect(hasPendingClarification([old, current], { currentTurnId: CURRENT_TURN_ID })).toBe(true);
    expect(hasPendingClarification([old], { currentTurnId: CURRENT_TURN_ID })).toBe(false);
  });
});

describe("newestDurableTurnId ordering", () => {
  it("uses creation time before the turn id when start times match", () => {
    expect(
      newestDurableTurnId([
        turn("turn-z", TURN_TIMESTAMP, TURN_TIMESTAMP),
        turn("turn-a", TURN_TIMESTAMP, "2026-08-14T12:00:00.000000001Z"),
      ]),
    ).toBe("turn-a");
  });

  it("selects the newest durable turn with backend tie-break ordering", () => {
    expect(
      newestDurableTurnId([
        {
          id: "turn-a",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: TURN_TIMESTAMP,
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
        {
          id: "turn-b",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: TURN_TIMESTAMP,
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
      ]),
    ).toBe("turn-b");
  });

  it("uses preserved subsecond precision before the turn id tie-break", () => {
    expect(
      newestDurableTurnId([
        {
          id: "turn-z",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: "2026-08-14T12:00:00.100Z",
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
        {
          id: "turn-a",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: "2026-08-14T12:00:00.900Z",
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
      ]),
    ).toBe("turn-a");
  });

  it("orders an exact-second timestamp before a later fractional timestamp", () => {
    expect(
      newestDurableTurnId([
        {
          id: "turn-z",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: TURN_TIMESTAMP,
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
        {
          id: "turn-a",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: "2026-08-14T12:00:00.1Z",
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
      ]),
    ).toBe("turn-a");
  });

  it("orders an exact-second timestamp before a one-nanosecond-later timestamp", () => {
    expect(
      newestDurableTurnId([
        {
          id: "turn-z",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: TURN_TIMESTAMP,
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
        {
          id: "turn-a",
          session_id: toSessionId("session-1"),
          task_id: toTaskId("task-1"),
          started_at: "2026-08-14T12:00:00.000000001Z",
          created_at: TURN_TIMESTAMP,
          updated_at: TURN_TIMESTAMP,
        },
      ]),
    ).toBe("turn-a");
  });
});

describe("hasPendingPermissionRequest", () => {
  it("detects permission requests with pending status", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "pending" } }),
      ]),
    ).toBe(true);
  });

  it("treats missing permission status as pending", () => {
    expect(hasPendingPermissionRequest([message({ type: "permission_request" })])).toBe(true);
  });

  it("ignores approved permission requests", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "approved" } }),
      ]),
    ).toBe(false);
  });

  it("ignores rejected permission requests", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "rejected" } }),
      ]),
    ).toBe(false);
  });

  it("ignores expired permission requests", () => {
    expect(
      hasPendingPermissionRequest([
        message({ type: "permission_request", metadata: { status: "expired" } }),
      ]),
    ).toBe(false);
  });

  it("returns false for non-permission messages", () => {
    expect(hasPendingPermissionRequest([message({ type: "message" })])).toBe(false);
  });

  it("returns false for empty or null input", () => {
    expect(hasPendingPermissionRequest([])).toBe(false);
    expect(hasPendingPermissionRequest(null)).toBe(false);
    expect(hasPendingPermissionRequest(undefined)).toBe(false);
  });

  // Mixed-state: only the latest permission_request drives the UI. A stale
  // pending row left behind by an earlier crash followed by a newer approved
  // one must not light the amber icon — the agent is no longer blocked on
  // the old row.
  it("returns false when an older permission is still pending but a newer one is approved", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "old", type: "permission_request", metadata: { status: "pending" } }),
        message({ id: "new", type: "permission_request", metadata: { status: "approved" } }),
      ]),
    ).toBe(false);
  });

  it("returns true when the latest permission is pending and earlier ones are resolved", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "a", type: "permission_request", metadata: { status: "approved" } }),
        message({ id: "b", type: "permission_request", metadata: { status: "rejected" } }),
        message({ id: "c", type: "permission_request", metadata: { status: "pending" } }),
      ]),
    ).toBe(true);
  });

  it("returns false when all permission requests are resolved regardless of order", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "a", type: "permission_request", metadata: { status: "approved" } }),
        message({ id: "b", type: "permission_request", metadata: { status: "rejected" } }),
        message({ id: "c", type: "permission_request", metadata: { status: "expired" } }),
      ]),
    ).toBe(false);
  });
});

// Turn-scoped boundary: walking back across a different turn_id ends the
// scan, so a pending row from a previous (crashed) turn must not leak into
// the current turn's indicator.
describe("hasPendingPermissionRequest — turn scoping", () => {
  it("ignores a pending permission from a previous turn", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          id: "old",
          turn_id: "t1",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
        message({ id: "current", turn_id: "t2", type: "message", content: "hi" }),
      ]),
    ).toBe(false);
  });

  it("detects a pending permission in the current turn", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          id: "stale",
          turn_id: "t1",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
        message({
          id: "active",
          turn_id: "t2",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
      ]),
    ).toBe(true);
  });

  it("stops at the turn boundary even when an older turn has a pending permission", () => {
    expect(
      hasPendingPermissionRequest([
        message({
          id: "old-pending",
          turn_id: "t1",
          type: "permission_request",
          metadata: { status: "pending" },
        }),
        message({
          id: "new-approved",
          turn_id: "t2",
          type: "permission_request",
          metadata: { status: "approved" },
        }),
      ]),
    ).toBe(false);
  });

  // A legacy permission_request with no turn_id, sitting in a session whose
  // latest message *does* have a turn_id, must not bypass the boundary.
  it("ignores a legacy null-turn_id pending permission when a turn is active", () => {
    expect(
      hasPendingPermissionRequest([
        message({ id: "legacy", type: "permission_request", metadata: { status: "pending" } }),
        message({ id: "current", turn_id: "t2", type: "message", content: "hi" }),
      ]),
    ).toBe(false);
  });
});
