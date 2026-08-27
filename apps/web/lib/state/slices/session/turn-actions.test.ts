import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "@/lib/state/slices/session/session-slice";
import type { SessionSlice } from "@/lib/state/slices/session/types";
import { sessionId, taskId } from "@/lib/types/ids";
import type { Turn } from "@/lib/types/http";
import { parseTurnTimestamp, shouldApplyTurnUpdate } from "./turn-actions";

const SESSION_ID = sessionId("session-1");
const TASK_ID = taskId("task-1");
const STARTED_AT = "2026-07-23T10:00:00.000Z";
const COMPLETED_AT = "2026-07-23T10:01:00.000Z";
const LATER_AT = "2026-07-23T10:02:00.000Z";

/** Creates a zustand store backed by the session slice. */
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

function turn(
  id: string,
  overrides: Partial<{
    started_at: string;
    completed_at: string | undefined;
    updated_at: string | undefined;
    metadata: Record<string, unknown>;
  }> = {},
): Turn {
  const completedAt = overrides.completed_at ?? undefined;
  const updatedAt = overrides.updated_at ?? completedAt ?? overrides.started_at ?? STARTED_AT;
  return {
    id,
    session_id: SESSION_ID,
    task_id: TASK_ID,
    started_at: overrides.started_at ?? STARTED_AT,
    completed_at: completedAt,
    created_at: STARTED_AT,
    updated_at: updatedAt,
    metadata: overrides.metadata,
  };
}

function seedSession(store: ReturnType<typeof makeStore>, state: string, updatedAt: string) {
  store.setState((s) => {
    s.taskSessions.items[SESSION_ID] = { id: SESSION_ID, state, updated_at: updatedAt } as never;
  });
}

describe("parseTurnTimestamp", () => {
  it("accepts RFC3339 with explicit offset or UTC marker", () => {
    expect(parseTurnTimestamp("2026-07-23T10:00:00Z")).not.toBeNull();
    expect(parseTurnTimestamp("2026-07-23T10:00:00.123Z")).not.toBeNull();
    expect(parseTurnTimestamp("2026-07-23T10:00:00+02:00")).not.toBeNull();
  });

  it.each([
    ["0", "Date.parse parses this as 2000-01-01"],
    ["2026-08-31", "partial date (midnight)"],
    ["2026-08-31T23:00:00", "timezone-less (local zone)"],
    ["not-a-timestamp", "garbage"],
    ["", "empty"],
    [undefined, "absent"],
  ])("treats %s as stale", (_value, _reason) => {
    expect(parseTurnTimestamp(_value)).toBeNull();
  });

  it.each([
    ["2026-02-30T10:00:00Z", "Feb 30 (Date.parse normalizes to Mar 2)"],
    ["2025-02-29T10:00:00Z", "Feb 29 in a non-leap year"],
    ["2026-04-31T10:00:00Z", "April has 30 days"],
    ["2026-01-01T24:00:00Z", "hour 24 (RFC3339 allows 00-23)"],
    ["2026-01-01T23:60:00Z", "minute 60"],
    ["2026-01-01T23:59:60Z", "second 60 (leap second; backend never emits)"],
    ["2026-01-01T10:00:00+24:00", "offset hour 24"],
    ["2026-13-01T10:00:00Z", "month 13"],
    ["2026-01-00T10:00:00Z", "day 0"],
  ])("rejects %s (%s) as stale instead of normalizing it", (value) => {
    expect(parseTurnTimestamp(value)).toBeNull();
  });

  it("accepts genuine leap-day timestamps", () => {
    expect(parseTurnTimestamp("2024-02-29T10:00:00Z")).not.toBeNull();
    expect(parseTurnTimestamp("2000-02-29T10:00:00Z")).not.toBeNull();
  });

  it("applies UTC offsets in seconds (positive, negative, pre-epoch)", () => {
    expect(parseTurnTimestamp("2026-01-01T02:00:00+02:00")).toBe(
      parseTurnTimestamp("2026-01-01T00:00:00Z"),
    );
    expect(parseTurnTimestamp("2026-01-01T00:00:00-02:00")).toBe(
      parseTurnTimestamp("2026-01-01T02:00:00Z"),
    );
    expect(parseTurnTimestamp("1969-12-31T22:00:00-02:00")).toBe(
      parseTurnTimestamp("1970-01-01T00:00:00Z"),
    );
  });

  it("rejects fractions longer than 9 digits", () => {
    // RFC3339Nano caps at 9 fractional digits; a 10-digit fraction would
    // otherwise overflow into the seconds component.
    expect(parseTurnTimestamp("2026-01-01T00:00:00.1234567890Z")).toBeNull();
  });

  it("preserves sub-millisecond fraction ordering without rounding", () => {
    // Math.round on the fraction would collapse .9995 onto the next second's
    // epoch; the parser must keep .9995 < .9996 < next second.
    const nextSecond = parseTurnTimestamp("2026-01-01T00:00:01Z");
    const frac9995 = parseTurnTimestamp("2026-01-01T00:00:00.9995Z");
    const frac9996 = parseTurnTimestamp("2026-01-01T00:00:00.9996Z");
    expect(nextSecond).not.toBeNull();
    expect(frac9995).not.toBeNull();
    expect(frac9996).not.toBeNull();
    expect(frac9995!).toBeLessThan(nextSecond!);
    expect(frac9995!).toBeLessThan(frac9996!);
    expect(frac9996!).toBeLessThan(nextSecond!);
  });

  it("preserves nanosecond ordering from Go RFC3339Nano payloads", () => {
    // float64 epochs collapse values closer than ~244ns; the parser must keep
    // exact nanosecond distinctions (Go time.Time serializes RFC3339Nano).
    const nextSecond = parseTurnTimestamp("2026-01-01T00:00:01Z");
    const nano999 = parseTurnTimestamp("2026-01-01T00:00:00.999999999Z");
    const nano1 = parseTurnTimestamp("2026-01-01T00:00:00.000000001Z");
    const nano2 = parseTurnTimestamp("2026-01-01T00:00:00.000000002Z");
    const nano123 = parseTurnTimestamp("2026-01-01T00:00:00.123456789Z");
    const nano124 = parseTurnTimestamp("2026-01-01T00:00:00.123456790Z");
    expect(nextSecond).not.toBeNull();
    expect(nano999).not.toBeNull();
    expect(nano1).not.toBeNull();
    expect(nano2).not.toBeNull();
    expect(nano123).not.toBeNull();
    expect(nano124).not.toBeNull();
    expect(nano999!).toBeLessThan(nextSecond!);
    expect(nano1!).toBeLessThan(nano2!);
    expect(nano1!).toBeLessThan(nextSecond!);
    expect(nano123!).toBeLessThan(nano124!);
  });
});

describe("addTurn reconciliation", () => {
  it("applies a newer completed row over an incomplete WS row", () => {
    const store = makeStore();
    store.getState().addTurn(turn("turn-1", { started_at: STARTED_AT }));
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "new" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0]).toEqual(
      expect.objectContaining({ completed_at: COMPLETED_AT, metadata: { model: "new" } }),
    );
  });

  it("applies a row that is newer by sub-millisecond fraction", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(
        turn("turn-1", { completed_at: COMPLETED_AT, updated_at: "2026-01-01T00:00:00.9995Z" }),
      );
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-01-01T00:00:00.9996Z",
        metadata: { model: "fraction-newer" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({
      model: "fraction-newer",
    });
  });

  it("applies a row that is newer by a single nanosecond", () => {
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-01-01T00:00:00.999999999Z",
      }),
    );
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-01-01T00:00:01Z",
        metadata: { model: "nano-newer" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({
      model: "nano-newer",
    });
  });

  it("rejects a stale WS started event after a completed row was hydrated", () => {
    // The reviewer-reported race: REST hydration merged the completed row,
    // then a delayed session.turn.started upsert arrives via addTurn. The
    // stale started payload must not regress metadata or updated_at.
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: COMPLETED_AT,
        metadata: { prompt_usage: { total_tokens: 9 }, model: "newer" },
      }),
    );

    store.getState().addTurn(
      turn("turn-1", {
        started_at: STARTED_AT,
        updated_at: STARTED_AT,
        metadata: { model: "stale-start" },
      }),
    );

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(COMPLETED_AT);
    expect(stored.updated_at).toBe(COMPLETED_AT);
    expect(stored.metadata).toEqual({ prompt_usage: { total_tokens: 9 }, model: "newer" });
  });

  it("keeps the existing row when timestamps are equal", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "existing" } }));
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "same-ts" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({
      model: "existing",
    });
  });
});

describe("mergeTurnsSnapshot", () => {
  it("merges rows, preserves live completion, and reconciles once", () => {
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    store.getState().addTurn(
      turn("turn-live", {
        completed_at: COMPLETED_AT,
        updated_at: COMPLETED_AT,
        metadata: { model: "live" },
      }),
    );
    store.getState().setActiveTurn(SESSION_ID, "turn-live");

    let notifications = 0;
    const unsubscribe = store.subscribe(() => {
      notifications += 1;
    });
    store.getState().mergeTurnsSnapshot(
      SESSION_ID,
      [
        turn("turn-live", {
          completed_at: undefined,
          updated_at: STARTED_AT,
          metadata: { model: "stale-route" },
        }),
        turn("turn-new", {
          started_at: LATER_AT,
          updated_at: LATER_AT,
          completed_at: undefined,
        }),
      ],
      0,
    );
    unsubscribe();

    expect(notifications).toBe(1);
    expect(store.getState().turns.bySession[SESSION_ID]).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: "turn-live", completed_at: COMPLETED_AT }),
        expect.objectContaining({ id: "turn-new" }),
      ]),
    );
    expect(store.getState().turns.bySession[SESSION_ID]).toHaveLength(2);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBe(true);
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-new");
  });
});

describe("addTurn malformed timestamps", () => {
  it("treats a malformed incoming timestamp as stale", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "0",
        metadata: { model: "malformed" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("applies a valid incoming row when the existing timestamp is malformed", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(
        turn("turn-1", { completed_at: COMPLETED_AT, updated_at: "0", metadata: { model: "old" } }),
      );
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: LATER_AT, metadata: { model: "fresh" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "fresh" });
  });

  it("treats a partial-date incoming timestamp as stale", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-08-31",
        metadata: { model: "partial" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("treats a timezone-less incoming timestamp as stale", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-08-31T23:00:00",
        metadata: { model: "no-zone" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("applies a valid incoming row when the existing timestamp is timezone-less", () => {
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-08-31T23:00:00",
        metadata: { model: "old" },
      }),
    );
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: LATER_AT, metadata: { model: "fresh" } }));
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "fresh" });
  });

  it("treats a semantically invalid calendar date as stale in a merge", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-02-30T10:00:00Z",
        metadata: { model: "normalized" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });

  it("treats an over-precision fraction as stale in a merge", () => {
    const store = makeStore();
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, metadata: { model: "valid" } }));
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: COMPLETED_AT,
        updated_at: "2026-01-01T00:00:00.1234567890Z",
        metadata: { model: "overprecise" },
      }),
    );
    expect(store.getState().turns.bySession[SESSION_ID][0].metadata).toEqual({ model: "valid" });
  });
});

describe("completeTurn stale guard", () => {
  it("applies a genuine completion and clears the active marker", () => {
    const store = makeStore();
    store.getState().addTurn(turn("turn-1", { started_at: STARTED_AT }));
    store.getState().setActiveTurn(SESSION_ID, "turn-1");

    store
      .getState()
      .completeTurn(SESSION_ID, "turn-1", COMPLETED_AT, { model: "new" }, COMPLETED_AT);

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(COMPLETED_AT);
    expect(stored.metadata).toEqual({ model: "new" });
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("ignores a stale re-delivered completion", () => {
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: LATER_AT,
        updated_at: LATER_AT,
        metadata: { model: "newer" },
      }),
    );

    store
      .getState()
      .completeTurn(SESSION_ID, "turn-1", COMPLETED_AT, { model: "stale" }, COMPLETED_AT);

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(LATER_AT);
    expect(stored.metadata).toEqual({ model: "newer" });
  });

  it("ignores a re-delivered completion without updated_at", () => {
    const store = makeStore();
    store.getState().addTurn(
      turn("turn-1", {
        completed_at: LATER_AT,
        updated_at: LATER_AT,
        metadata: { model: "newer" },
      }),
    );

    store.getState().completeTurn(SESSION_ID, "turn-1", COMPLETED_AT, { model: "stale" });

    const stored = store.getState().turns.bySession[SESSION_ID][0];
    expect(stored.completed_at).toBe(LATER_AT);
    expect(stored.metadata).toEqual({ model: "newer" });
  });
});

describe("reconcileActiveTurnAfterHydration", () => {
  it("sets the latest incomplete turn as active for a running session", () => {
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, updated_at: COMPLETED_AT }));
    store
      .getState()
      .addTurn(
        turn("turn-2", { started_at: LATER_AT, updated_at: LATER_AT, completed_at: undefined }),
      );

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-2");
  });

  it("clears the active marker when every turn is completed", () => {
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, updated_at: COMPLETED_AT }));

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("does not mark an orphaned incomplete turn active for a settled session", () => {
    // IDLE snapshot already in the store (reconcileActiveTurnForIdleSession
    // would have cleared the marker); hydration must not resurrect it.
    const store = makeStore();
    seedSession(store, "IDLE", LATER_AT);
    store
      .getState()
      .addTurn(
        turn("turn-1", { started_at: STARTED_AT, updated_at: STARTED_AT, completed_at: undefined }),
      );

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("keeps a turn active when it started after the settled session snapshot", () => {
    const store = makeStore();
    seedSession(store, "IDLE", STARTED_AT);
    store
      .getState()
      .addTurn(
        turn("turn-1", { started_at: LATER_AT, updated_at: LATER_AT, completed_at: undefined }),
      );

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-1");
  });

  it("rejects a stale hydration after source adoption bumped the epoch", () => {
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    store
      .getState()
      .addTurn(
        turn("turn-1", { started_at: STARTED_AT, updated_at: STARTED_AT, completed_at: undefined }),
      );
    store.getState().setActiveTurn(SESSION_ID, "turn-1");
    // Source adoption cleared the marker and bumped the epoch mid-flight.
    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID]);
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("applies when the hydration epoch still matches", () => {
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    // Server-issued adoption boundary (WS envelope timestamp).
    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:03:00.000Z");
    // A genuinely new turn started after the adoption boundary.
    store.getState().addTurn(
      turn("turn-2", {
        started_at: "2026-07-23T10:04:00.000Z",
        updated_at: "2026-07-23T10:04:00.000Z",
      }),
    );

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 1);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-2");
  });

  it("never re-activates a turn retired by an authoritative boundary", () => {
    // Adoption retired turn-1 (server-issued boundary after its start); a
    // post-adoption hydration (fresh epoch) sees the still-incomplete row
    // and must not choose it as the active turn.
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    store
      .getState()
      .addTurn(
        turn("turn-1", { started_at: STARTED_AT, updated_at: STARTED_AT, completed_at: undefined }),
      );
    store.getState().setActiveTurn(SESSION_ID, "turn-1");
    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:00:30.000Z");

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 1);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });
});

describe("reconcileActiveTurnAfterHydration — boundary precision", () => {
  it("excludes every pre-boundary orphan, not just the latest", () => {
    // An IDLE snapshot with TWO incomplete turns started before it; the
    // reconciliation must leave the marker null so a delayed start for the
    // OLDER turn cannot resurrect it.
    const store = makeStore();
    seedSession(store, "IDLE", LATER_AT);
    store.setState((st) => {
      st.turns.settledBoundaryBySession[SESSION_ID] = LATER_AT;
    });
    store.getState().addTurn(turn("turn-old", { started_at: STARTED_AT, updated_at: STARTED_AT }));
    store
      .getState()
      .addTurn(turn("turn-new", { started_at: COMPLETED_AT, updated_at: COMPLETED_AT }));

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("keeps a sub-millisecond-post-boundary turn active", () => {
    // The boundary comparison must use nanosecond precision: a turn started
    // 800ns after a 100ns boundary is AFTER it.
    const store = makeStore();
    seedSession(store, "IDLE", "2026-01-01T00:00:00.000000100Z");
    store.setState((st) => {
      st.turns.settledBoundaryBySession[SESSION_ID] = "2026-01-01T00:00:00.000000100Z";
    });
    store.getState().addTurn(turn("turn-1", { started_at: "2026-01-01T00:00:00.000000900Z" }));

    store.getState().reconcileActiveTurnAfterHydration(SESSION_ID, 0);

    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-1");
  });
});

describe("settled boundary monotonicity", () => {
  it("never regresses the boundary when adoption would record an older time", () => {
    const store = makeStore();
    store.setState((s) => {
      s.taskSessions.items[SESSION_ID] = { id: SESSION_ID, state: "RUNNING" } as never;
      // A future server-reported boundary (clock skew / future updated_at).
      s.turns.settledBoundaryBySession[SESSION_ID] = "2099-01-01T00:00:00Z";
    });

    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID], "2000-01-01T00:00:00Z");

    expect(store.getState().turns.settledBoundaryBySession[SESSION_ID]).toBe(
      "2099-01-01T00:00:00Z",
    );
  });

  it("records the server-issued adoption boundary", () => {
    const store = makeStore();
    store.setState((s) => {
      s.taskSessions.items[SESSION_ID] = { id: SESSION_ID, state: "RUNNING" } as never;
    });

    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:03:00.000Z");

    expect(store.getState().turns.settledBoundaryBySession[SESSION_ID]).toBe(
      "2026-07-23T10:03:00.000Z",
    );
  });

  it("does not advance the boundary without a server timestamp (no client clock)", () => {
    // The optimistic REST submission has no server timestamp; the action must
    // not fall back to the client clock, which would retire legitimate turns
    // when the browser clock runs ahead of the backend. The authoritative WS
    // adoption event records the boundary on arrival.
    const store = makeStore();
    store.setState((s) => {
      s.taskSessions.items[SESSION_ID] = { id: SESSION_ID, state: "RUNNING" } as never;
    });

    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID]);

    expect(store.getState().turns.settledBoundaryBySession[SESSION_ID]).toBeUndefined();
    // The marker clear and epoch bump still apply.
    expect(store.getState().turns.reconcileEpochBySession[SESSION_ID]).toBe(1);
  });
});

describe("settled boundary lifecycle", () => {
  it("survives turn completion and blocks a stale force-hydrated snapshot", () => {
    const store = makeStore();
    store.setState((s) => {
      s.taskSessions.items[SESSION_ID] = { id: SESSION_ID, state: "RUNNING" } as never;
    });
    store
      .getState()
      .addTurn(
        turn("turn-1", { started_at: STARTED_AT, updated_at: STARTED_AT, completed_at: undefined }),
      );
    store.getState().setActiveTurn(SESSION_ID, "turn-1");
    store.getState().reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:00:30.000Z");
    // Completion arrives; the boundary persists (no tombstone to prune).
    store
      .getState()
      .addTurn(turn("turn-1", { completed_at: COMPLETED_AT, updated_at: COMPLETED_AT }));

    expect(store.getState().turns.settledBoundaryBySession[SESSION_ID]).toBe(
      "2026-07-23T10:00:30.000Z",
    );
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBeNull();
  });
});

describe("setActiveTurn nanosecond promotion", () => {
  it("promotes a start that is newer by sub-millisecond precision", () => {
    const store = makeStore();
    store.setState((s) => {
      s.taskSessions.items[SESSION_ID] = { id: SESSION_ID, state: "RUNNING" } as never;
    });
    store.getState().addTurn(turn("turn-a", { started_at: "2026-01-01T00:00:00.000000900Z" }));
    store.getState().setActiveTurn(SESSION_ID, "turn-a");
    store.getState().addTurn(turn("turn-b", { started_at: "2026-01-01T00:00:00.000001000Z" }));
    store.getState().setActiveTurn(SESSION_ID, "turn-b");

    // Date.parse would collapse both to the same millisecond and refuse to
    // promote; nanosecond precision must promote turn-b.
    expect(store.getState().turns.activeBySession[SESSION_ID]).toBe("turn-b");
  });
});

describe("shouldApplyTurnUpdate", () => {
  it("completion precedence beats equal or older timestamps", () => {
    const existing = turn("t", { started_at: STARTED_AT, updated_at: STARTED_AT });
    const incoming = turn("t", {
      completed_at: COMPLETED_AT,
      updated_at: STARTED_AT,
    });
    expect(shouldApplyTurnUpdate(existing, incoming)).toBe(true);
  });

  it("never rolls back an existing completion", () => {
    const existing = turn("t", { completed_at: COMPLETED_AT, updated_at: COMPLETED_AT });
    const incoming = turn("t", { started_at: LATER_AT, updated_at: LATER_AT });
    expect(shouldApplyTurnUpdate(existing, incoming)).toBe(false);
  });

  it("retains stored turns that the snapshot omits", () => {
    const store = makeStore();
    seedSession(store, "RUNNING", LATER_AT);
    store.getState().addTurn(turn("live-only"));

    store.getState().mergeTurnsSnapshot(SESSION_ID, [turn("turn-1")], 0);

    expect(store.getState().turns.bySession[SESSION_ID].map((item) => item.id)).toEqual([
      "live-only",
      "turn-1",
    ]);
  });
});
