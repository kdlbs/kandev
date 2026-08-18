import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  ensureSessionTurnsLoaded,
  clearInFlightTurnsLoadForTest,
} from "./use-session-turns-hydration";
import { shouldApplyTurnUpdate } from "@/lib/state/slices/session/turn-actions";
import type { Turn } from "@/lib/types/http";

const mockListSessionTurns = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

type TurnStoreState = {
  loadedBySession: Record<string, boolean>;
  sessions: Record<string, { state: string }>;
  bySession: Record<string, unknown[]>;
  activeBySession: Record<string, string | null>;
  reconcileEpochBySession: Record<string, number>;
};

type TurnStoreMock = {
  getState: () => {
    turns: {
      loadedBySession: Record<string, boolean>;
      bySession: Record<string, unknown[]>;
      activeBySession: Record<string, string | null>;
      reconcileEpochBySession: Record<string, number>;
      settledBoundaryBySession: Record<string, string>;
    };
    taskSessions: { items: Record<string, { state: string }> };
  };
  addTurn: ReturnType<typeof vi.fn>;
  mergeTurnsSnapshot: ReturnType<typeof vi.fn>;
  markTurnsLoaded: ReturnType<typeof vi.fn>;
  reconcileActiveTurnAfterHydration: ReturnType<typeof vi.fn>;
};

type TurnStoreHarness = TurnStoreMock & {
  setSessions: (v: Record<string, { state: string }>) => void;
};

const SESSION_ID = "sess-1";
const EMPTY_TURNS = { turns: [], total: 0 };
const BASE_TIMESTAMP = "2026-08-10T10:00:00Z";
const COMPLETION_AT = "2026-08-10T10:55:00Z";
const LIVE_UPDATED_AT = "2026-08-10T11:00:00Z";
const SAME_SECOND_COMPLETION = "2026-08-10T10:05:00Z";
const BACKEND_DOWN = "backend down";

/** Builds a REST-shaped turn row; overrides win for race-specific fields. */
function makeTurn(id: string, overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id,
    session_id: SESSION_ID,
    task_id: "task-1",
    started_at: BASE_TIMESTAMP,
    metadata: { runtime_config_snapshot: { model: "mock-fast" } },
    created_at: BASE_TIMESTAMP,
    updated_at: BASE_TIMESTAMP,
    ...overrides,
  };
}

/** Returns a REST promise the test resolves manually (for race timing). */
function deferredTurnsResponse(): {
  promise: Promise<{ turns: unknown[]; total: number }>;
  resolve: (value: { turns: unknown[]; total: number }) => void;
} {
  let resolve!: (value: { turns: unknown[]; total: number }) => void;
  const promise = new Promise<{ turns: unknown[]; total: number }>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

/** Mirrors the store's addTurn upsert (Object.assign, completed_at preserved). */
function upsertTurn(bySession: Record<string, unknown[]>, turn: Record<string, unknown>): void {
  const sessionId = turn.session_id as string;
  const turns = (bySession[sessionId] ??= []);
  const existing = turns.find((item) => (item as Record<string, unknown>).id === turn.id);
  if (!existing) {
    turns.push(turn);
    return;
  }
  const existingRecord = existing as Record<string, unknown>;
  Object.assign(existingRecord, turn, {
    completed_at: existingRecord.completed_at ?? turn.completed_at,
  });
}

function makeStore(): TurnStoreHarness {
  const state: TurnStoreState = {
    loadedBySession: {},
    sessions: { [SESSION_ID]: { state: "RUNNING" } },
    bySession: {},
    activeBySession: {},
    reconcileEpochBySession: {},
  };
  const addTurn = vi.fn((turn: Record<string, unknown>) => {
    upsertTurn(state.bySession, turn);
  });
  const markTurnsLoaded = vi.fn((sid: string) => {
    state.loadedBySession[sid] = true;
  });
  const reconcileActiveTurnAfterHydration = vi.fn();
  const mergeTurnsSnapshot = vi.fn(
    (sid: string, turns: Record<string, unknown>[], epoch: number) => {
      const existingById = new Map(
        (state.bySession[sid] as Turn[] | undefined)?.map((turn) => [turn.id, turn]),
      );
      for (const turn of turns as Turn[]) {
        const existing = existingById.get(turn.id);
        if (!existing || shouldApplyTurnUpdate(existing, turn)) addTurn(turn);
      }
      reconcileActiveTurnAfterHydration(sid, epoch);
      markTurnsLoaded(sid);
    },
  );
  return {
    getState: () => ({
      turns: {
        loadedBySession: state.loadedBySession,
        bySession: state.bySession,
        activeBySession: state.activeBySession,
        reconcileEpochBySession: state.reconcileEpochBySession,
        settledBoundaryBySession: {},
      },
      taskSessions: { items: state.sessions },
      // The zustand store exposes actions on getState() too.
      addTurn,
      mergeTurnsSnapshot,
      markTurnsLoaded,
      reconcileActiveTurnAfterHydration,
    }),
    addTurn,
    mergeTurnsSnapshot,
    markTurnsLoaded,
    reconcileActiveTurnAfterHydration,
    setSessions: (v) => {
      state.sessions = v;
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
});

afterEach(() => {
  clearInFlightTurnsLoadForTest();
  vi.useRealTimers();
});

describe("ensureSessionTurnsLoaded — hydration and marker", () => {
  it("deduplicates concurrent hydration of the same session", async () => {
    mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
    const store = makeStore();

    await Promise.all([
      ensureSessionTurnsLoaded(SESSION_ID, store as never),
      ensureSessionTurnsLoaded(SESSION_ID, store as never),
    ]);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
  });

  it("merges unseen history turns and marks the session loaded", async () => {
    mockListSessionTurns.mockResolvedValue({ turns: [makeTurn("turn-1")], total: 1 });
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-1" }));
    expect(store.markTurnsLoaded).toHaveBeenCalledWith(SESSION_ID);
  });

  it("marks an empty session loaded so it is never refetched", async () => {
    mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);
    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("skips the fetch entirely when the session is already marked loaded", async () => {
    const store = makeStore();
    store.getState().turns.loadedBySession[SESSION_ID] = true;

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).not.toHaveBeenCalled();
  });

  it("refreshes a loaded session after a new subscription readiness generation", async () => {
    const store = makeStore();
    store.getState().turns.loadedBySession[SESSION_ID] = true;
    mockListSessionTurns.mockResolvedValue({
      turns: [makeTurn("turn-after-reconnect")],
      total: 1,
    });

    const readiness = Promise.resolve();
    await (
      ensureSessionTurnsLoaded as unknown as (
        sessionId: string,
        store: unknown,
        options: { readiness: Promise<void> },
      ) => Promise<void>
    )(SESSION_ID, store, { readiness });

    expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
    expect(store.addTurn).toHaveBeenCalledWith(
      expect.objectContaining({ id: "turn-after-reconnect" }),
    );
  });
});

describe("ensureSessionTurnsLoaded — guards and races", () => {
  it("establishes the active marker for the latest incomplete turn", async () => {
    // The WS session.turn.started event may have been missed; the REST rows
    // are the only source for the running turn, and agent-status / files-panel
    // gating read activeBySession.
    mockListSessionTurns.mockResolvedValue({
      turns: [
        makeTurn("turn-1", { completed_at: COMPLETION_AT, updated_at: COMPLETION_AT }),
        makeTurn("turn-2", {
          started_at: "2026-08-11T09:00:00Z",
          created_at: "2026-08-11T09:00:00Z",
          updated_at: "2026-08-11T09:00:00Z",
          completed_at: undefined,
        }),
      ],
      total: 2,
    });
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    // The store-owned action decides the marker (latest incomplete turn vs
    // settled-session clear vs epoch rejection); the module must hand it the
    // epoch captured before the request.
    expect(store.reconcileActiveTurnAfterHydration).toHaveBeenCalledWith(SESSION_ID, 0);
  });

  it("passes the epoch captured before an in-flight authoritative clear", async () => {
    // Source adoption bumps the epoch while the REST request is in flight;
    // the module must still pass the ORIGINAL epoch so the store action
    // rejects the stale reconciliation.
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    store.getState().turns.reconcileEpochBySession[SESSION_ID] = 2;

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    store.getState().turns.reconcileEpochBySession[SESSION_ID] = 3;
    resolveList({ turns: [makeTurn("turn-1")], total: 1 });
    await pending;

    expect(store.reconcileActiveTurnAfterHydration).toHaveBeenCalledWith(SESSION_ID, 2);
  });

  it("does not resurrect turns after the session was removed mid-flight", async () => {
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // Session deleted while the REST request is in flight (removeTaskSession
    // drops both taskSessions.items[sid] and turns.bySession[sid]).
    store.setSessions({});
    resolveList({ turns: [{ id: "turn-stale" }], total: 1 });
    await pending;

    expect(store.addTurn).not.toHaveBeenCalled();
    expect(store.markTurnsLoaded).not.toHaveBeenCalled();
  });

  it("clears the in-flight entry on failure so the next fetch retries", async () => {
    mockListSessionTurns.mockRejectedValueOnce(new Error("boom"));
    mockListSessionTurns.mockResolvedValueOnce({ turns: [], total: 0 });
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);
    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(2);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("retries a transient failure automatically until it succeeds", async () => {
    mockListSessionTurns.mockRejectedValueOnce(new Error("network blip"));
    mockListSessionTurns.mockRejectedValueOnce(new Error("network blip"));
    mockListSessionTurns.mockResolvedValueOnce({ turns: [], total: 0 });
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    // Two failures are absorbed by the bounded backoff; the third attempt
    // lands, so a transient failure never leaves metadata unresolved.
    expect(mockListSessionTurns).toHaveBeenCalledTimes(3);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });

  it("gives up after exhausting retries and leaves the marker unset for a later fetch", async () => {
    mockListSessionTurns.mockRejectedValue(new Error(BACKEND_DOWN));
    const store = makeStore();

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    // Bounded: no unbounded hammering of a failing endpoint.
    expect(mockListSessionTurns).toHaveBeenCalledTimes(3);
    expect(store.markTurnsLoaded).not.toHaveBeenCalled();

    // In-flight entry is cleared; the next natural trigger retries and wins.
    mockListSessionTurns.mockResolvedValueOnce({ turns: [], total: 0 });
    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).toHaveBeenCalledTimes(4);
    expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
  });
});
describe("ensureSessionTurnsLoaded — recovery scheduling", () => {
  it("recovers after retry exhaustion via a scheduled delayed retry", async () => {
    vi.useFakeTimers();
    try {
      mockListSessionTurns.mockRejectedValue(new Error(BACKEND_DOWN));
      const store = makeStore();

      const hydration = ensureSessionTurnsLoaded(SESSION_ID, store as never);
      // Attempt 1 fails immediately; attempts 2-3 are gated by backoff timers.
      await vi.advanceTimersByTimeAsync(750);
      await hydration;

      expect(mockListSessionTurns).toHaveBeenCalledTimes(3);
      expect(store.markTurnsLoaded).not.toHaveBeenCalled();

      // The backend returns; the scheduled recovery retries WITHOUT any
      // unrelated lifecycle event (visibility, reconnect, session switch).
      mockListSessionTurns.mockResolvedValueOnce(EMPTY_TURNS);
      await vi.advanceTimersByTimeAsync(30_000);
      await vi.advanceTimersByTimeAsync(0);

      expect(mockListSessionTurns).toHaveBeenCalledTimes(4);
      expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps a single pending recovery per session", async () => {
    vi.useFakeTimers();
    try {
      mockListSessionTurns.mockRejectedValue(new Error(BACKEND_DOWN));
      const store = makeStore();

      const first = ensureSessionTurnsLoaded(SESSION_ID, store as never);
      await vi.advanceTimersByTimeAsync(750);
      await first; // exhausted -> recovery scheduled

      const second = ensureSessionTurnsLoaded(SESSION_ID, store as never);
      await vi.advanceTimersByTimeAsync(750);
      await second; // exhausted again -> must NOT stack a second timer

      mockListSessionTurns.mockResolvedValueOnce(EMPTY_TURNS);
      await vi.advanceTimersByTimeAsync(30_000);
      await vi.advanceTimersByTimeAsync(0);

      // 3 + 3 immediate attempts + exactly ONE recovery = 7.
      expect(mockListSessionTurns).toHaveBeenCalledTimes(7);
      expect(store.markTurnsLoaded).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not retry after exhaustion when the session is removed", async () => {
    vi.useFakeTimers();
    try {
      mockListSessionTurns.mockRejectedValue(new Error(BACKEND_DOWN));
      const store = makeStore();

      const hydration = ensureSessionTurnsLoaded(SESSION_ID, store as never);
      await vi.advanceTimersByTimeAsync(750);
      await hydration; // exhausted -> recovery scheduled

      // The session is deleted while the backend is down; the pending
      // recovery must fire into a no-op (no fetch, no marker resurrection).
      store.setSessions({});
      await vi.advanceTimersByTimeAsync(30_000);
      await vi.advanceTimersByTimeAsync(0);

      expect(mockListSessionTurns).toHaveBeenCalledTimes(3);
      expect(store.markTurnsLoaded).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("skips the fetch when the session is absent from the store at entry", async () => {
    mockListSessionTurns.mockResolvedValue(EMPTY_TURNS);
    const store = makeStore();
    store.setSessions({});

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(mockListSessionTurns).not.toHaveBeenCalled();
    expect(store.addTurn).not.toHaveBeenCalled();
  });
});
describe("ensureSessionTurnsLoaded — REST/WS reconciliation", () => {
  it("reconciles an older WS start-snapshot with the newer REST completion", async () => {
    // A `session.turn.started` was delivered but the matching
    // `session.turn.completed` was missed (disconnect window), leaving the
    // store with an incomplete row for a turn the REST full history has as
    // completed. The hydration must merge the newer REST row instead of
    // skipping the ID as "already present".
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    // WS start snapshot: no completed_at, older updated_at.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: BASE_TIMESTAMP,
        completed_at: undefined,
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // REST resolves with the completed row (newer updated_at).
    resolveList({
      turns: [
        makeTurn("turn-1", {
          completed_at: SAME_SECOND_COMPLETION,
          updated_at: SAME_SECOND_COMPLETION,
        }),
      ],
      total: 1,
    });
    await pending;

    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.completed_at).toBe(SAME_SECOND_COMPLETION);
    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-1" }));
  });
});

describe("ensureSessionTurnsLoaded — timestamp edge cases", () => {
  it("applies a completed REST row when timestamps collide after precision truncation", async () => {
    // WS rows carry RFC3339Nano fractions; the REST DTO truncates to whole
    // seconds (time.RFC3339). A completion in the same second as the WS start
    // therefore has a REST updated_at <= the WS row's fractional timestamp.
    // The completion state, not the colliding timestamp, must decide.
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    // WS start snapshot with fractional updated_at, no completion.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: "2026-08-10T10:05:00.123Z",
        completed_at: undefined,
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // REST completion row, updated_at truncated to the same second (<= WS).
    resolveList({
      turns: [
        makeTurn("turn-1", {
          completed_at: "2026-08-10T10:05:00.456Z",
          updated_at: SAME_SECOND_COMPLETION,
        }),
      ],
      total: 1,
    });
    await pending;

    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.completed_at).toBe("2026-08-10T10:05:00.456Z");
  });

  it("treats a REST row without a timestamp as stale, never newest", async () => {
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();
    // Newer live WS row.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: LIVE_UPDATED_AT,
        completed_at: COMPLETION_AT,
        metadata: { runtime_config_snapshot: { model: "newer" } },
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // Malformed REST row: no updated_at at all.
    resolveList({
      turns: [
        makeTurn("turn-1", {
          updated_at: undefined,
          metadata: { runtime_config_snapshot: { model: "stale" } },
        }),
      ],
      total: 1,
    });
    await pending;

    expect(store.addTurn).not.toHaveBeenCalled();
    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.updated_at).toBe(LIVE_UPDATED_AT);
    expect(stored.completed_at).toBe(COMPLETION_AT);
  });

  it("merges a valid REST row when the existing row has an invalid timestamp", async () => {
    // Date.parse of a malformed string yields NaN, and NaN comparisons are
    // always false — the valid REST row must not lose to an invalid existing
    // timestamp. Same completion state so the timestamp path is exercised.
    let resolveList!: (value: { turns: unknown[]; total: number }) => void;
    mockListSessionTurns.mockReturnValue(
      new Promise<{ turns: unknown[]; total: number }>((resolve) => {
        resolveList = resolve;
      }),
    );
    const store = makeStore();
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: "not-a-timestamp",
        completed_at: COMPLETION_AT,
        metadata: { runtime_config_snapshot: { model: "stale" } },
      }),
    ];

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    resolveList({
      turns: [
        makeTurn("turn-1", {
          updated_at: LIVE_UPDATED_AT,
          completed_at: COMPLETION_AT,
          metadata: { runtime_config_snapshot: { model: "fresh" } },
        }),
      ],
      total: 1,
    });
    await pending;

    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.metadata).toEqual({ runtime_config_snapshot: { model: "fresh" } });
    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-1" }));
  });
});

it("keeps the existing row when timestamps are equal", async () => {
  mockListSessionTurns.mockResolvedValue({
    turns: [makeTurn("turn-1", { completed_at: COMPLETION_AT })],
    total: 1,
  });
  const store = makeStore();
  store.getState().turns.bySession[SESSION_ID] = [
    makeTurn("turn-1", {
      completed_at: COMPLETION_AT,
      metadata: { runtime_config_snapshot: { model: "existing" } },
    }),
  ];

  await ensureSessionTurnsLoaded(SESSION_ID, store as never);

  expect(store.addTurn).not.toHaveBeenCalled();
  const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
  expect(stored.metadata).toEqual({ runtime_config_snapshot: { model: "existing" } });
});

describe("ensureSessionTurnsLoaded — timestamp comparison semantics", () => {
  it("skips a REST row with an invalid timestamp over a valid existing row", async () => {
    mockListSessionTurns.mockResolvedValue({
      turns: [
        makeTurn("turn-1", {
          updated_at: "not-a-timestamp",
          completed_at: COMPLETION_AT,
          metadata: { runtime_config_snapshot: { model: "stale" } },
        }),
      ],
      total: 1,
    });
    const store = makeStore();
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: LIVE_UPDATED_AT,
        completed_at: COMPLETION_AT,
        metadata: { runtime_config_snapshot: { model: "fresh" } },
      }),
    ];

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    expect(store.addTurn).not.toHaveBeenCalled();
    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.metadata).toEqual({ runtime_config_snapshot: { model: "fresh" } });
  });

  it("merges when both rows are completed and the REST row is newer", async () => {
    mockListSessionTurns.mockResolvedValue({
      turns: [
        makeTurn("turn-1", {
          updated_at: LIVE_UPDATED_AT,
          completed_at: COMPLETION_AT,
          metadata: { runtime_config_snapshot: { model: "rest-newer" } },
        }),
      ],
      total: 1,
    });
    const store = makeStore();
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: BASE_TIMESTAMP,
        completed_at: COMPLETION_AT,
        metadata: { runtime_config_snapshot: { model: "ws-older" } },
      }),
    ];

    await ensureSessionTurnsLoaded(SESSION_ID, store as never);

    const stored = store.getState().turns.bySession[SESSION_ID][0] as Record<string, unknown>;
    expect(stored.metadata).toEqual({ runtime_config_snapshot: { model: "rest-newer" } });
  });

  it("does not clobber live WS turn data with a stale REST snapshot", async () => {
    // WS `session.turn.completed` for turn-1 arrives WHILE the REST request
    // is in flight; the REST response is an older snapshot of the same turn
    // plus the pre-WS history (turn-2). The newer live metadata must survive.
    const { promise, resolve: resolveList } = deferredTurnsResponse();
    mockListSessionTurns.mockReturnValue(promise);
    const store = makeStore();

    const pending = ensureSessionTurnsLoaded(SESSION_ID, store as never);
    // Live WS completion lands while the request is pending: newer row.
    store.getState().turns.bySession[SESSION_ID] = [
      makeTurn("turn-1", {
        updated_at: LIVE_UPDATED_AT,
        completed_at: COMPLETION_AT,
        metadata: {
          runtime_config_snapshot: { model: "newer" },
          prompt_usage: { total_tokens: 9 },
        },
      }),
    ];
    resolveList({
      turns: [
        makeTurn("turn-1", { metadata: { runtime_config_snapshot: { model: "older" } } }),
        makeTurn("turn-2", { started_at: "2026-08-11T10:00:00Z" }),
      ],
      total: 2,
    });
    await pending;

    // Only the unseen history turn may be merged; the live row is untouched.
    expect(store.addTurn).toHaveBeenCalledTimes(1);
    expect(store.addTurn).toHaveBeenCalledWith(expect.objectContaining({ id: "turn-2" }));
    const stored = store
      .getState()
      .turns.bySession[
        SESSION_ID
      ].find((turn) => (turn as Record<string, unknown>).id === "turn-1") as Record<
      string,
      unknown
    >;
    expect(stored.updated_at).toBe(LIVE_UPDATED_AT);
    expect(stored.completed_at).toBe(COMPLETION_AT);
    expect(stored.metadata).toEqual({
      runtime_config_snapshot: { model: "newer" },
      prompt_usage: { total_tokens: 9 },
    });
  });
});
