import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState, mergeInitialState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

// Dedicated file for the turns `loadedBySession` marker predicates so
// hydrator.test.ts stays under the repo's 600-line file cap.

const SESSION_ID = "session-1";
const BOUNDARY = "2026-01-01T00:00:00Z";
const TASK_ID = "task-1";
const TURN_STARTED_AT = "2026-01-01T00:00:00Z";
const TURN_COMPLETED_AT = "2026-01-01T00:01:00Z";
const STALE_ROUTE_UPDATED_AT = "2026-01-01T00:00:30Z";
const FRESH_TURN = "fresh-turn";
const IDLE_SESSION = { id: SESSION_ID, state: "IDLE", updated_at: BOUNDARY };
const OLD_TURN = "old-turn";
const OLD_BOUNDARY = "2025-12-31T00:00:00Z";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

describe("settled boundary seeding ordering", () => {
  it("does not clear a live marker for the protected active session", () => {
    // Non-force hydration protects the active session: the merge SKIPS its
    // bySession/activeBySession entries (live data wins). The stale-marker
    // sweep must not clear the live marker even though the SSR payload
    // carries a settled session and a matching old turn row.
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.bySession[SESSION_ID] = [
        { id: "live-turn", started_at: "2026-01-02T00:00:00Z" },
      ] as never;
      draft.turns.activeBySession[SESSION_ID] = "live-turn";
      hydrateState(
        draft,
        {
          taskSessions: {
            items: { [SESSION_ID]: { id: SESSION_ID, state: "IDLE", updated_at: BOUNDARY } },
          },
          turns: {
            activeBySession: { [SESSION_ID]: "live-turn" },
            bySession: {
              [SESSION_ID]: [{ id: "live-turn", started_at: OLD_BOUNDARY, completed_at: null }],
            },
          },
        } as unknown as Partial<AppState>,
        { activeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBe("live-turn");
  });

  it("clears a stale SSR active marker for a turn before the seeded boundary", () => {
    // Production SSR payload: settled session + active marker naming a turn
    // started before the boundary. The marker clear must see the seeded
    // boundary, or the stale marker survives and drives turn/source UI.
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(
        draft,
        {
          taskSessions: {
            items: { [SESSION_ID]: IDLE_SESSION },
          },
          turns: {
            activeBySession: { [SESSION_ID]: OLD_TURN },
            bySession: {
              [SESSION_ID]: [{ id: OLD_TURN, started_at: OLD_BOUNDARY, completed_at: null }],
            },
          },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBe(BOUNDARY);
  });
});

describe("settled boundary seeding (production StateHydrator path)", () => {
  it("seeds a boundary from a settled session via hydrateState", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(
        draft,
        {
          taskSessions: {
            items: { [SESSION_ID]: IDLE_SESSION },
          },
        } as unknown as Partial<AppState>,
        {},
      );
    });
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBe(BOUNDARY);
  });

  it("does not seed a boundary for a running session via hydrateState", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(
        draft,
        {
          taskSessions: {
            items: { [SESSION_ID]: { id: SESSION_ID, state: "RUNNING", updated_at: BOUNDARY } },
          },
        } as unknown as Partial<AppState>,
        {},
      );
    });
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBeUndefined();
  });

  it.each([
    ["older valid", OLD_BOUNDARY],
    ["malformed", "not-a-timestamp"],
    ["equal", BOUNDARY],
  ])(
    "preserves an existing boundary against a %s SSR candidate via hydrateState",
    (_name, updatedAt) => {
      const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
        draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
        hydrateState(
          draft,
          {
            taskSessions: {
              items: {
                [SESSION_ID]: { id: SESSION_ID, state: "IDLE", updated_at: updatedAt },
              },
            },
          } as unknown as Partial<AppState>,
          {},
        );
      });
      expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBe(BOUNDARY);
    },
  );
});

describe("stale-marker sweep install predicate", () => {
  const STALE_TURN = {
    activeBySession: { [SESSION_ID]: "old-turn" },
    bySession: {
      [SESSION_ID]: [{ id: OLD_TURN, started_at: OLD_BOUNDARY, completed_at: null }],
    },
  } as const;

  it("sweeps a fresh non-force install", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(draft, { turns: STALE_TURN } as unknown as Partial<AppState>, {
        activeSessionId: null,
      });
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("sweeps a force merge over a pre-existing active-map key", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.activeBySession[SESSION_ID] = "live-turn";
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(draft, { turns: STALE_TURN } as unknown as Partial<AppState>, {
        forceMergeSessionId: SESSION_ID,
      });
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("sweeps when forceMergeSessionId equals activeSessionId", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(draft, { turns: STALE_TURN } as unknown as Partial<AppState>, {
        activeSessionId: SESSION_ID,
        forceMergeSessionId: SESSION_ID,
      });
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("keeps a pre-existing non-active marker (not installed by this merge)", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.activeBySession[SESSION_ID] = "live-turn";
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(draft, { turns: STALE_TURN } as unknown as Partial<AppState>, {
        activeSessionId: null,
      });
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBe("live-turn");
  });
});

describe("mergeInitialState — settled SSR sessions seed a boundary", () => {
  it("seeds a boundary from a settled session's updated_at", () => {
    const result = mergeInitialState({
      taskSessions: {
        items: {
          [SESSION_ID]: IDLE_SESSION,
        },
      } as never,
      turns: { bySession: { [SESSION_ID]: [] } } as never,
    });
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBe(BOUNDARY);
  });

  it("does not seed a boundary for a running session", () => {
    const result = mergeInitialState({
      taskSessions: {
        items: {
          [SESSION_ID]: { id: SESSION_ID, state: "RUNNING", updated_at: BOUNDARY },
        },
      } as never,
      turns: { bySession: { [SESSION_ID]: [] } } as never,
    });
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBeUndefined();
  });

  it("never regresses an existing newer boundary with an older SSR snapshot", () => {
    const result = mergeInitialState({
      taskSessions: {
        items: {
          [SESSION_ID]: { id: SESSION_ID, state: "IDLE", updated_at: OLD_BOUNDARY },
        },
      } as never,
      turns: {
        bySession: { [SESSION_ID]: [] },
        settledBoundaryBySession: { [SESSION_ID]: BOUNDARY },
      } as never,
    });
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBe(BOUNDARY);
  });

  it("keeps an existing boundary when the SSR updated_at is malformed", () => {
    const result = mergeInitialState({
      taskSessions: {
        items: {
          [SESSION_ID]: { id: SESSION_ID, state: "IDLE", updated_at: "not-a-timestamp" },
        },
      } as never,
      turns: {
        bySession: { [SESSION_ID]: [] },
        settledBoundaryBySession: { [SESSION_ID]: BOUNDARY },
      } as never,
    });
    expect(result.turns.settledBoundaryBySession[SESSION_ID]).toBe(BOUNDARY);
  });
});

describe("hydrated turn row freshness", () => {
  it("keeps a newer live completion when a forced route snapshot is stale", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.taskSessions.items[SESSION_ID] = {
        id: SESSION_ID,
        state: "RUNNING",
        updated_at: "2026-01-02T00:00:00Z",
      } as never;
      draft.turns.bySession[SESSION_ID] = [
        {
          id: FRESH_TURN,
          session_id: SESSION_ID,
          task_id: TASK_ID,
          started_at: TURN_STARTED_AT,
          completed_at: TURN_COMPLETED_AT,
          created_at: TURN_STARTED_AT,
          updated_at: TURN_COMPLETED_AT,
          metadata: { model: "live" },
        },
      ] as never;
      draft.turns.activeBySession[SESSION_ID] = FRESH_TURN;

      hydrateState(
        draft,
        {
          turns: {
            bySession: {
              [SESSION_ID]: [
                {
                  id: FRESH_TURN,
                  session_id: SESSION_ID,
                  task_id: TASK_ID,
                  started_at: TURN_STARTED_AT,
                  completed_at: null,
                  created_at: TURN_STARTED_AT,
                  updated_at: STALE_ROUTE_UPDATED_AT,
                  metadata: { model: "stale-route" },
                },
              ],
            },
            activeBySession: { [SESSION_ID]: FRESH_TURN },
          },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });

    expect(result.turns.bySession[SESSION_ID][0]).toEqual(
      expect.objectContaining({
        completed_at: TURN_COMPLETED_AT,
        metadata: { model: "live" },
      }),
    );
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
  });
});

describe("hydrateState — turns loadedBySession marker predicates", () => {
  it.each([
    ["active", "active-session"],
    ["pre-existing non-active", SESSION_ID],
  ])("does not mark a skipped %s session loaded", (_name, sessionId) => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.bySession[sessionId] = [{ id: "live-turn" }] as never;
      hydrateState(
        draft,
        {
          turns: { bySession: { [sessionId]: [{ id: "stale-turn" }] } },
        } as unknown as Partial<AppState>,
        { activeSessionId: sessionId === "active-session" ? sessionId : null },
      );
    });
    // The skipped merge must keep the live turns and must NOT claim the
    // session's full history is loaded.
    expect(result.turns.bySession[sessionId]).toEqual([{ id: "live-turn" }]);
    expect(result.turns.loadedBySession[sessionId]).toBeUndefined();
  });

  it("marks a force-merged session loaded even when it already existed", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.bySession[SESSION_ID] = [{ id: "live-turn" }] as never;
      hydrateState(
        draft,
        {
          turns: { bySession: { [SESSION_ID]: [{ id: FRESH_TURN }] } },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.bySession[SESSION_ID]).toEqual([{ id: "live-turn" }, { id: FRESH_TURN }]);
    expect(result.turns.loadedBySession[SESSION_ID]).toBe(true);
  });

  it("never force-merges a stale active marker for a boundary-settled turn", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      // The turn was retired by an authoritative boundary (source adoption /
      // settled-session clear) while the server snapshot still names it as
      // the active turn.
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(
        draft,
        {
          turns: {
            activeBySession: { [SESSION_ID]: "retired-turn" },
            bySession: {
              [SESSION_ID]: [{ id: "retired-turn", started_at: OLD_BOUNDARY, completed_at: null }],
            },
          },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBeNull();
  });

  it("keeps a force-merged active marker for a turn started after the boundary", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      draft.turns.settledBoundaryBySession[SESSION_ID] = BOUNDARY;
      hydrateState(
        draft,
        {
          turns: {
            activeBySession: { [SESSION_ID]: FRESH_TURN },
            bySession: {
              [SESSION_ID]: [
                { id: FRESH_TURN, started_at: "2026-01-02T00:00:00Z", completed_at: null },
              ],
            },
          },
        } as unknown as Partial<AppState>,
        { forceMergeSessionId: SESSION_ID },
      );
    });
    expect(result.turns.activeBySession[SESSION_ID]).toBe(FRESH_TURN);
  });
});
