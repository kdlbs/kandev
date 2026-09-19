import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Turn } from "@/lib/types/http";
import { newestDurableTurnId } from "./pending-clarification";

// AC-9: newestDurableTurnId must apply the same two rules the backend's
// currentTurnAuthority applies before its started_at/created_at/id
// comparison - skip lifecycle turns, prefer an open turn over a completed
// one - reading the marker from the turn payload rather than inferring
// anything from timestamps.

const TIMESTAMP_11 = "2026-08-14T11:00:00Z";
const TIMESTAMP_13 = "2026-08-14T13:00:00Z";
const NEWER_TURN_ID = "turn-newer";

function turn(
  id: string,
  startedAt: string,
  createdAt = startedAt,
  overrides: Partial<Turn> = {},
): Turn {
  return {
    id,
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    started_at: startedAt,
    created_at: createdAt,
    updated_at: createdAt,
    ...overrides,
  };
}

describe("newestDurableTurnId AC-9: lifecycle exclusion and open-turn precedence", () => {
  it("skips a lifecycle-only turn even when it is the newest by timestamp", () => {
    expect(
      newestDurableTurnId([
        turn("turn-real", "2026-08-14T12:00:00Z"),
        turn("turn-lifecycle", TIMESTAMP_13, undefined, { metadata: { lifecycle_only: true } }),
      ]),
    ).toBe("turn-real");
  });

  it("resolves to null when every turn is lifecycle-only", () => {
    expect(
      newestDurableTurnId([
        turn("turn-lifecycle", "2026-08-14T12:00:00Z", undefined, {
          metadata: { lifecycle_only: true },
        }),
      ]),
    ).toBeNull();
  });

  const lifecycleEncodings = [true, 1, "true", "1"];
  it.each(lifecycleEncodings)("treats metadata.lifecycle_only = %j as lifecycle", (value) => {
    expect(
      newestDurableTurnId([
        turn("turn-real", "2026-08-14T12:00:00Z"),
        turn("turn-lifecycle", TIMESTAMP_13, undefined, { metadata: { lifecycle_only: value } }),
      ]),
    ).toBe("turn-real");
  });

  const conversationalEncodings = [null, undefined, false, 0, "", "false", "0", "yes", {}, []];
  it.each(conversationalEncodings)(
    "treats metadata.lifecycle_only = %j as conversational, not lifecycle",
    (value) => {
      expect(
        newestDurableTurnId([
          turn("turn-older", TIMESTAMP_11),
          turn(NEWER_TURN_ID, TIMESTAMP_13, undefined, { metadata: { lifecycle_only: value } }),
        ]),
      ).toBe(NEWER_TURN_ID);
    },
  );

  it("treats an absent metadata.lifecycle_only key as conversational", () => {
    expect(
      newestDurableTurnId([turn("turn-older", TIMESTAMP_11), turn(NEWER_TURN_ID, TIMESTAMP_13)]),
    ).toBe(NEWER_TURN_ID);
  });

  it("prefers an open turn over a later completed turn", () => {
    expect(
      newestDurableTurnId([
        turn("turn-open", TIMESTAMP_11),
        turn("turn-completed-later", TIMESTAMP_13, undefined, {
          completed_at: "2026-08-14T13:05:00Z",
        }),
      ]),
    ).toBe("turn-open");
  });

  it("treats an explicit null completed_at the same as an absent key", () => {
    expect(
      newestDurableTurnId([
        turn("turn-open", TIMESTAMP_11, undefined, { completed_at: null as unknown as string }),
        turn("turn-completed-later", TIMESTAMP_13, undefined, {
          completed_at: "2026-08-14T13:05:00Z",
        }),
      ]),
    ).toBe("turn-open");
  });
});
