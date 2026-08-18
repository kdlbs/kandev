import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Turn } from "@/lib/types/http";

const mockListSessionTurns = vi.fn();
const state = {
  tasks: { activeSessionId: "session-a" as string | null },
  turns: {
    bySession: { "session-a": [] as Turn[], "session-b": [] as Turn[] } as Record<string, Turn[]>,
    loadedBySession: {} as Record<string, boolean>,
    reconcileEpochBySession: {} as Record<string, number>,
  },
  mergeTurnsSnapshot: vi.fn((sessionId: string, turns: Turn[]) => {
    state.turns.bySession[sessionId] = [...(state.turns.bySession[sessionId] ?? []), ...turns];
    state.turns.loadedBySession[sessionId] = true;
  }),
};

vi.mock("@/lib/api/domains/session-api", () => ({
  listSessionTurns: (...args: unknown[]) => mockListSessionTurns(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (store: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));

import { useSessionTurns } from "./use-session-turns";

/** Builds a Turn for the given id and session, with fixed test timestamps. */
function turn(id: string, sessionId = "session-a"): Turn {
  return {
    id,
    session_id: sessionId as Turn["session_id"],
    task_id: "task-1" as Turn["task_id"],
    started_at: "2026-01-01T00:00:00.000Z",
    created_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
  };
}

/** Fetch mock whose first call never settles until the request signal aborts
 *  (as a real fetch does), then resolves like the backend. */
function hungFirstCallTurnsMock(): void {
  let attempts = 0;
  mockListSessionTurns.mockImplementation(
    (_sessionId: string, options?: { init?: { signal?: AbortSignal } }) => {
      attempts += 1;
      const signal = options?.init?.signal;
      if (attempts === 1) {
        return new Promise<{ turns: Turn[]; total: number }>((_resolve, reject) => {
          signal?.addEventListener("abort", () =>
            reject(new DOMException("Aborted", "AbortError")),
          );
        });
      }
      return Promise.resolve({ turns: [turn("turn-a")], total: 1 });
    },
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  state.tasks.activeSessionId = "session-a";
  state.turns.bySession = { "session-a": [], "session-b": [] };
  state.turns.loadedBySession = {};
  state.turns.reconcileEpochBySession = {};
  mockListSessionTurns.mockResolvedValue({ turns: [turn("turn-a")], total: 1 });
});

describe("useSessionTurns", () => {
  it("fetches when the marker is absent and suppresses partial turns until hydration", async () => {
    state.turns.bySession["session-a"] = [turn("live")];
    const { result, rerender } = renderHook(() => useSessionTurns("session-a"));

    // While the marker is absent (fetch in flight) the partial live snapshot
    // must not leak durations to the panel.
    expect(result.current).toEqual([]);

    await waitFor(() =>
      expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
        "session-a",
        [turn("turn-a")],
        expect.any(Number),
      ),
    );

    // After the marker lands the merged turns (live + fetched) are exposed.
    rerender();
    expect(result.current).toEqual([turn("live"), turn("turn-a")]);
  });

  it("does not expose a live completed turn as a duration before hydration completes", async () => {
    // A live completeTurn event writes a completed turn while
    // listSessionTurns is still in flight; the unhydrated snapshot must not
    // reach the panel (which would derive a duration from it).
    state.turns.bySession["session-a"] = [turn("completed")];
    const { result } = renderHook(() => useSessionTurns("session-a"));

    expect(result.current).toEqual([]);
  });

  it("does not refetch a hydrated session", () => {
    state.turns.loadedBySession["session-a"] = true;
    renderHook(() => useSessionTurns("session-a"));

    expect(mockListSessionTurns).not.toHaveBeenCalled();
  });

  it("discards a delayed response after active session changes", async () => {
    let resolveA!: (value: { turns: Turn[]; total: number }) => void;
    mockListSessionTurns.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveA = resolve;
      }),
    );
    const { rerender } = renderHook(({ sessionId }) => useSessionTurns(sessionId), {
      initialProps: { sessionId: "session-a" },
    });

    await waitFor(() =>
      expect(mockListSessionTurns).toHaveBeenCalledWith(
        "session-a",
        expect.objectContaining({ init: expect.anything() }),
      ),
    );
    state.tasks.activeSessionId = "session-b";
    rerender({ sessionId: "session-b" });
    resolveA({ turns: [turn("stale")], total: 1 });

    await act(async () => {});
    expect(state.mergeTurnsSnapshot).not.toHaveBeenCalledWith(
      "session-a",
      [turn("stale")],
      expect.any(Number),
    );
  });

  it("discards a generation-0 response after an A→B→A active-session round trip", async () => {
    const deferred: Array<{ resolve: (v: { turns: Turn[]; total: number }) => void }> = [];
    for (let i = 0; i < 3; i += 1) {
      mockListSessionTurns.mockReturnValueOnce(
        new Promise((resolve) => {
          deferred.push({ resolve });
        }),
      );
    }
    const { rerender } = renderHook(({ sessionId }) => useSessionTurns(sessionId), {
      initialProps: { sessionId: "session-a" },
    });

    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledTimes(1));
    state.tasks.activeSessionId = "session-b";
    rerender({ sessionId: "session-a" });
    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledTimes(2));
    state.tasks.activeSessionId = "session-a";
    rerender({ sessionId: "session-a" });
    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledTimes(3));

    // The newest fetch lands; the generation-0 response must be discarded
    // even though the active session is A again.
    deferred[2].resolve({ turns: [turn("latest")], total: 1 });
    await act(async () => {});
    expect(state.mergeTurnsSnapshot).toHaveBeenLastCalledWith(
      "session-a",
      [turn("latest")],
      expect.any(Number),
    );

    deferred[0].resolve({ turns: [turn("stale")], total: 1 });
    await act(async () => {});
    expect(state.mergeTurnsSnapshot).not.toHaveBeenCalledWith(
      "session-a",
      [turn("stale")],
      expect.any(Number),
    );
  });
});

describe("useSessionTurns — ordering and isolation", () => {
  it("discards an out-of-order response for the SAME session (stale sequence)", async () => {
    // Task 01 stale-sequence discard: two concurrent fetches for the same
    // session (two mounted hook instances share the module fetch sequence).
    // The newer response applies first; the older response must be discarded
    // by `sequence < appliedSequence` and never reach mergeTurnsSnapshot.
    const deferredA: Array<{ resolve: (v: { turns: Turn[]; total: number }) => void }> = [];
    const deferredB: Array<{ resolve: (v: { turns: Turn[]; total: number }) => void }> = [];
    mockListSessionTurns
      .mockReturnValueOnce(
        new Promise((resolve) => {
          deferredA.push({ resolve });
        }),
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          deferredB.push({ resolve });
        }),
      );

    renderHook(() => useSessionTurns("session-a"));
    renderHook(() => useSessionTurns("session-a"));
    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledTimes(2));

    // Newer response lands first and applies.
    deferredB[0].resolve({ turns: [turn("stale-b")], total: 1 });
    await waitFor(() =>
      expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
        "session-a",
        [turn("stale-b")],
        expect.any(Number),
      ),
    );

    // The older response must be discarded by the sequence check.
    deferredA[0].resolve({ turns: [turn("stale-a")], total: 1 });
    await act(async () => {});
    expect(state.mergeTurnsSnapshot).not.toHaveBeenCalledWith(
      "session-a",
      [turn("stale-a")],
      expect.any(Number),
    );
    expect(state.mergeTurnsSnapshot).toHaveBeenCalledTimes(1);
  });

  it("tracks the applied sequence per session so sibling fetches do not invalidate each other", async () => {
    const deferredA: { resolve: (v: { turns: Turn[]; total: number }) => void }[] = [];
    const deferredB: { resolve: (v: { turns: Turn[]; total: number }) => void }[] = [];
    mockListSessionTurns
      .mockReturnValueOnce(
        new Promise((resolve) => {
          deferredA.push({ resolve });
        }),
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          deferredB.push({ resolve });
        }),
      );
    const turnB = turn("turn-b", "session-b");

    renderHook(() => useSessionTurns("session-a"));
    renderHook(() => useSessionTurns("session-b"));
    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledTimes(2));

    // session-b's later-sequence response applies first, then session-a's
    // earlier-sequence response must still apply (isolation by session).
    deferredB[0].resolve({ turns: [turnB], total: 1 });
    await act(async () => {});
    expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith("session-b", [turnB], expect.any(Number));

    deferredA[0].resolve({ turns: [turn("turn-a")], total: 1 });
    await act(async () => {});
    expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
      "session-a",
      [turn("turn-a")],
      expect.any(Number),
    );
  });

  it("hydrates a sibling session lazily when its marker is absent", async () => {
    state.turns.bySession["session-b"] = [];
    renderHook(() => useSessionTurns("session-b"));

    expect(mockListSessionTurns).toHaveBeenCalledWith(
      "session-b",
      expect.objectContaining({ init: expect.anything() }),
    );
    await waitFor(() =>
      expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
        "session-b",
        [turn("turn-a")],
        expect.any(Number),
      ),
    );
  });
});

describe("useSessionTurns — fetch failure recovery", () => {
  it("retries a transient fetch failure and hydrates on the retry", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mockListSessionTurns
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce({ turns: [turn("turn-a")], total: 1 });
    vi.useFakeTimers();
    try {
      const { result, rerender } = renderHook(() => useSessionTurns("session-a"));

      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
      // The failure logs and schedules a bounded backoff retry.
      expect(errorSpy).toHaveBeenCalledWith(
        "[useSessionTurns] failed to fetch turns for",
        "session-a",
        expect.any(Error),
      );
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(2);
      expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
        "session-a",
        [turn("turn-a")],
        expect.any(Number),
      );

      rerender();
      expect(result.current).toEqual([turn("turn-a")]);
    } finally {
      vi.useRealTimers();
      errorSpy.mockRestore();
    }
  });

  it("recovers hydration via the slow retry after the bounded budget is exhausted", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mockListSessionTurns
      .mockRejectedValueOnce(new Error("down-1"))
      .mockRejectedValueOnce(new Error("down-2"))
      .mockRejectedValueOnce(new Error("down-3"))
      .mockResolvedValueOnce({ turns: [turn("turn-a")], total: 1 });
    vi.useFakeTimers();
    try {
      const { result, rerender } = renderHook(() => useSessionTurns("session-a"));

      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
      // Drain the bounded backoff (4s then 8s) so all three fast attempts fail.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      await act(async () => {});
      await act(async () => {
        await vi.advanceTimersByTimeAsync(8000);
      });
      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(3);
      expect(result.current).toEqual([]);

      // No fast retry is scheduled after exhaustion; the slow poll fires at
      // 30s (SLOW_RETRY_INTERVAL_MS) and the 4th attempt hydrates the session.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000);
      });
      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(4);
      expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
        "session-a",
        [turn("turn-a")],
        expect.any(Number),
      );

      rerender();
      expect(result.current).toEqual([turn("turn-a")]);
    } finally {
      vi.useRealTimers();
      errorSpy.mockRestore();
    }
  });
});

describe("useSessionTurns — hung request timeout", () => {
  it("recovers from a hung request via the fetch timeout and a retry", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    hungFirstCallTurnsMock();
    vi.useFakeTimers();
    try {
      const { result, rerender } = renderHook(() => useSessionTurns("session-a"));

      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(1);
      expect(result.current).toEqual([]);

      // The fetch timeout (15s) aborts the hung request and schedules a retry
      // as if it were any other transient failure.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(15_000);
      });
      await act(async () => {});
      await act(async () => {
        await vi.advanceTimersByTimeAsync(4000);
      });
      await act(async () => {});
      expect(mockListSessionTurns).toHaveBeenCalledTimes(2);
      expect(state.mergeTurnsSnapshot).toHaveBeenCalledWith(
        "session-a",
        [turn("turn-a")],
        expect.any(Number),
      );

      rerender();
      expect(result.current).toEqual([turn("turn-a")]);
    } finally {
      vi.useRealTimers();
      errorSpy.mockRestore();
    }
  });

  it("applies the turn response despite an independent live commit mid-flight", async () => {
    // Task 01 cross-resource isolation: a message/turn commit landing in the
    // store after the turns fetch started must not invalidate the response —
    // mergeTurnsSnapshot merges over whatever the store holds.
    let resolveFetch!: (value: { turns: Turn[]; total: number }) => void;
    mockListSessionTurns.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFetch = resolve;
      }),
    );
    state.turns.bySession["session-a"] = [turn("live-before")];
    const { result, rerender } = renderHook(() => useSessionTurns("session-a"));

    await waitFor(() => expect(mockListSessionTurns).toHaveBeenCalledTimes(1));
    expect(result.current).toEqual([]);

    // An independent store commit lands while the fetch is in flight (e.g. a
    // message event completing a turn before listSessionTurns resolves).
    state.turns.bySession["session-a"] = [turn("live-before"), turn("live-mid")];
    resolveFetch({ turns: [turn("turn-a")], total: 1 });
    await waitFor(() => expect(state.mergeTurnsSnapshot).toHaveBeenCalledTimes(1));

    rerender();
    expect(result.current).toEqual([turn("live-before"), turn("live-mid"), turn("turn-a")]);
  });
});
