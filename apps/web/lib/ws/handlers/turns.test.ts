import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "@/lib/state/slices/session/session-slice";
import type { SessionSlice } from "@/lib/state/slices/session/types";
import type { TurnEventPayload } from "@/lib/types/backend";
import { registerTurnsHandlers } from "./turns";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";
const TURN_STARTED = "session.turn.started";
const TURN_COMPLETED = "session.turn.completed";
const NOTIFICATION = "notification";
const TURN_STARTED_AT = "2026-07-23T10:00:00.000Z";

function makeStore() {
  return create<SessionSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      quickChat: { sessions: [] },
      availableCommands: { bySessionId: {} },
    })),
  );
}

function turn(id: string, startedAt: string, completedAt?: string): TurnEventPayload {
  return {
    id,
    session_id: SESSION_ID,
    task_id: TASK_ID,
    started_at: startedAt,
    completed_at: completedAt,
    created_at: startedAt,
    updated_at: completedAt ?? startedAt,
  };
}

function send(
  store: ReturnType<typeof makeStore>,
  action: typeof TURN_STARTED | typeof TURN_COMPLETED,
  payload: TurnEventPayload,
) {
  const handler = registerTurnsHandlers(store as never)[action]!;
  handler({ type: NOTIFICATION, action, payload } as never);
}

describe("session turn WebSocket handlers", () => {
  it("keeps a completed turn inactive when its delayed started event arrives", () => {
    const store = makeStore();
    const completed = "2026-07-23T10:01:00.000Z";

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, completed));
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
    expect(store.getState().turns.bySession[SESSION_ID]).toEqual([
      expect.objectContaining({ id: "turn-1", completed_at: completed }),
    ]);
  });

  it("does not clear a newer active turn when an older turn completes", () => {
    const store = makeStore();

    send(store, TURN_STARTED, turn("turn-a", TURN_STARTED_AT));
    send(store, TURN_STARTED, turn("turn-b", "2026-07-23T10:01:00.000Z"));
    send(store, TURN_COMPLETED, turn("turn-a", TURN_STARTED_AT, "2026-07-23T10:02:00.000Z"));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-b");
  });

  it("tracks a normal turn from start through completion", () => {
    const store = makeStore();
    const completed = "2026-07-23T10:01:00.000Z";

    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-1");

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, completed));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
    expect(store.getState().turns.bySession[SESSION_ID][0].completed_at).toBe(completed);
  });
});
