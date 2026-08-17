import { describe, expect, it, vi } from "vitest";
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
const TURN_COMPLETED_AT = "2026-07-23T10:01:00.000Z";

function makeStore() {
  return create<SessionSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      quickChat: { sessions: [] },
      availableCommands: { bySessionId: {} },
      prepareProgress: { bySessionId: {} },
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

const IDLE_SESSION = { id: SESSION_ID, state: "IDLE", updated_at: TURN_COMPLETED_AT } as never;

function settleViaSetTaskSession(store: ReturnType<typeof makeStore>): void {
  store.getState().setTaskSession(IDLE_SESSION);
}

function settleViaSetTaskSessionsForTask(store: ReturnType<typeof makeStore>): void {
  store.getState().setTaskSessionsForTask(TASK_ID, [IDLE_SESSION]);
}

function settleViaUpsertTaskSessionFromEvent(store: ReturnType<typeof makeStore>): void {
  store.getState().upsertTaskSessionFromEvent(TASK_ID, IDLE_SESSION);
}

describe("session turn WebSocket handlers", () => {
  it("keeps a completed turn inactive when its delayed started event arrives", () => {
    const store = makeStore();
    const completed = TURN_COMPLETED_AT;

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, completed));
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
    expect(store.getState().turns.bySession[SESSION_ID]).toEqual([
      expect.objectContaining({ id: "turn-1", completed_at: completed }),
    ]);
  });

  it("does not resurrect the marker after source adoption retires a turn", () => {
    const store = makeStore();
    // An incomplete turn with an active marker, as a pre-adoption WS start
    // left it.
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-1");

    // Source adoption clears the marker and retires the turn (server-issued
    // boundary after the turn's start — the WS envelope timestamp).
    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:00:30.000Z");
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();

    // A delayed delivery of the SAME older start arrives after adoption.
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("does not resurrect the marker after a settled-session clear retires a turn", () => {
    const store = makeStore();
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    // An IDLE session snapshot at/after the turn's start retires it.
    store.getState().setTaskSession({
      id: SESSION_ID,
      state: "IDLE",
      updated_at: TURN_COMPLETED_AT,
    } as never);
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();

    // The delayed start for the retired turn must not resurrect the marker.
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("does not resurrect the marker after a hydration settled-clear retires a turn", () => {
    const store = makeStore();
    // An IDLE session with an orphaned incomplete turn, hydrated via the
    // REST reconciliation (which clears the marker by the settled rule).
    store.getState().setTaskSession({
      id: SESSION_ID,
      state: "IDLE",
      updated_at: TURN_COMPLETED_AT,
    } as never);
    store.getState().addTurn(turn("turn-1", TURN_STARTED_AT) as never);
    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();

    // A delayed start for the turn the hydration settled must not resurrect
    // the marker.
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it.each([
    ["setTaskSession", settleViaSetTaskSession],
    ["setTaskSessionsForTask", settleViaSetTaskSessionsForTask],
    ["upsertTaskSessionFromEvent", settleViaUpsertTaskSessionFromEvent],
  ] as const)(
    "retires an unmarked incomplete turn when %s settles the session",
    (_name, settle) => {
      const store = makeStore();
      // The start event was missed entirely: the incomplete row exists but no
      // active marker was ever set.
      store.getState().addTurn(turn("turn-1", TURN_STARTED_AT) as never);

      settle(store);

      // A delayed start for the pre-settlement turn must not resurrect it.
      send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

      expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
    },
  );

  it("still marks a genuinely new turn active after source adoption", () => {
    const store = makeStore();
    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));
    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID]);

    // A new turn started after the (now-based) boundary is legitimate and
    // must be marked.
    const afterBoundary = new Date(Date.now() + 60_000).toISOString();
    send(store, TURN_STARTED, turn("turn-2", afterBoundary));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-2");
  });

  it("does not clear a newer active turn when an older turn completes", () => {
    const store = makeStore();

    send(store, TURN_STARTED, turn("turn-a", TURN_STARTED_AT));
    send(store, TURN_STARTED, turn("turn-b", TURN_COMPLETED_AT));
    send(store, TURN_COMPLETED, turn("turn-a", TURN_STARTED_AT, "2026-07-23T10:02:00.000Z"));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-b");
  });

  it("tracks a normal turn from start through completion", () => {
    const store = makeStore();
    const completed = TURN_COMPLETED_AT;

    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-1");

    send(store, TURN_COMPLETED, turn("turn-1", TURN_STARTED_AT, completed));
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
    expect(store.getState().turns.bySession[SESSION_ID][0].completed_at).toBe(completed);
  });

  it("flushes batched message updates before completing a turn", () => {
    const store = makeStore();
    const flush = vi.fn();
    const handler = registerTurnsHandlers(store as never, { flush })[TURN_COMPLETED]!;

    handler({
      id: "complete-1",
      type: NOTIFICATION,
      action: TURN_COMPLETED,
      payload: turn("turn-1", TURN_STARTED_AT, TURN_COMPLETED_AT),
    } as never);

    expect(flush).toHaveBeenCalledTimes(1);
  });
});

describe("settled boundary WS guard", () => {
  it("does not mark a malformed started_at even without a boundary", () => {
    const store = makeStore();

    send(store, TURN_STARTED, turn("turn-1", "not-a-timestamp"));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
  });

  it("does not mark a malformed started_at with a boundary", () => {
    const store = makeStore();
    store.getState().setTaskSession({
      id: SESSION_ID,
      state: "IDLE",
      updated_at: TURN_COMPLETED_AT,
    } as never);

    send(store, TURN_STARTED, turn("turn-1", "not-a-timestamp"));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
  });

  it("rejects a delayed start for a turn unknown at the boundary", () => {
    const store = makeStore();
    // The boundary (IDLE settle) happens BEFORE any turn is known to this
    // client; the delayed old start arrives afterwards.
    store.getState().setTaskSession({
      id: SESSION_ID,
      state: "IDLE",
      updated_at: TURN_COMPLETED_AT,
    } as never);

    send(store, TURN_STARTED, turn("turn-1", TURN_STARTED_AT));

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeFalsy();
  });
});
