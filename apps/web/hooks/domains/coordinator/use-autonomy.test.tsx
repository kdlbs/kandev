import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { AUTONOMY_RETRY_MS, AUTONOMY_TIMER_FLOOR_MS, useAutonomy } from "./use-autonomy";

const getAutonomy = vi.fn();
vi.mock("@/lib/api/domains/coordinator-autonomy-api", () => ({
  getAutonomy: (...args: unknown[]) => getAutonomy(...args),
}));

type Handler = (message: { payload: Record<string, unknown> }) => void;
let handlers: Handler[] = [];
vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => ({
    on: (_event: string, handler: Handler) => {
      handlers.push(handler);
      return () => {
        handlers = handlers.filter((h) => h !== handler);
      };
    },
  }),
}));

const NOW = Date.parse("2026-09-30T10:00:00Z");

function read(over: Partial<AutonomyRead> = {}): AutonomyRead {
  return {
    server_time: new Date(NOW).toISOString(),
    autonomy_enabled: true,
    admission: { ok: true, detail: "" },
    pending_wakes: 0,
    oldest_pending_at: null,
    last_woke_at: null,
    last_turn: null,
    containment: { conditions: [] },
    spend: {
      measurable: true,
      degraded: false,
      window_subcents: 0,
      mean_daily_subcents_7d: 0,
      mean_known: true,
      ceiling_subcents: 1000,
    },
    ...over,
  };
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

async function flush() {
  await act(async () => {
    await Promise.resolve();
  });
}

function emit(payload: Record<string, unknown>) {
  act(() => handlers.forEach((h) => h({ payload })));
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
  getAutonomy.mockReset();
  handlers = [];
});
afterEach(() => vi.useRealTimers());

describe("useAutonomy", () => {
  it("reads on mount and exposes the value", async () => {
    getAutonomy.mockResolvedValue(read());
    const { result } = renderHook(() => useAutonomy("w1", "c1", true));
    expect(result.current.loading).toBe(true);
    await flush();
    expect(result.current.value?.autonomy_enabled).toBe(true);
    expect(result.current.error).toBe(false);
    expect(getAutonomy).toHaveBeenCalledTimes(1);
  });

  it("does not read when disabled", async () => {
    const { result } = renderHook(() => useAutonomy("w1", "c1", false));
    await flush();
    expect(getAutonomy).not.toHaveBeenCalled();
    expect(result.current.value).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("treats a malformed body as an error, never as healthy", async () => {
    getAutonomy.mockResolvedValue({ server_time: "bad" });
    const { result } = renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    expect(result.current.value).toBeNull();
    expect(result.current.error).toBe(true);
  });

  it("keeps the last good value with the error when a later read fails", async () => {
    getAutonomy.mockResolvedValueOnce(read({ pending_wakes: 3 }));
    const { result } = renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    getAutonomy.mockRejectedValueOnce(new Error("boom"));
    act(() => result.current.retry());
    await flush();
    expect(result.current.value?.pending_wakes).toBe(3);
    expect(result.current.error).toBe(true);
  });

  it("lets only the latest issued read write: an older success never overwrites a newer failure", async () => {
    const first = deferred<AutonomyRead>();
    getAutonomy.mockReturnValueOnce(first.promise);
    const { result } = renderHook(() => useAutonomy("w1", "c1", true));
    getAutonomy.mockRejectedValueOnce(new Error("boom"));
    act(() => result.current.retry());
    await flush();
    expect(result.current.error).toBe(true);
    await act(async () => {
      first.resolve(read({ pending_wakes: 9 }));
      await Promise.resolve();
    });
    expect(result.current.error).toBe(true);
    expect(result.current.value).toBeNull();
  });

  it("re-reads on autonomy_changed for its coordinator only", async () => {
    getAutonomy.mockResolvedValue(read());
    renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    emit({ workspace_id: "w1", coordinator_id: "c1", open_proposals: 0 });
    emit({ workspace_id: "w1", coordinator_id: "other", autonomy_changed: true });
    expect(getAutonomy).toHaveBeenCalledTimes(1);
    emit({ workspace_id: "w1", coordinator_id: "c1", autonomy_changed: true });
    await flush();
    expect(getAutonomy).toHaveBeenCalledTimes(2);
  });
});

describe("useAutonomy timers and switching", () => {
  it("re-reads at stop_requested_at + 5 min + 1 s, and does not re-arm an unmoved deadline", async () => {
    const requested = new Date(NOW - 4 * 60_000).toISOString();
    const stale = read({
      last_turn: {
        id: "t",
        coordinator_id: "c1",
        conversation_task_id: "k",
        session_id: "s",
        started_at: requested,
        finished_at: null,
        outcome: null,
        wake_count: 1,
        denied_permissions: 0,
        cost_subcents: null,
        stop_requested_at: requested,
        stop_state: null,
      },
    });
    getAutonomy.mockResolvedValue(stale);
    renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    expect(getAutonomy).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(62_000);
    });
    expect(getAutonomy).toHaveBeenCalledTimes(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10 * 60_000);
    });
    expect(getAutonomy).toHaveBeenCalledTimes(2);
  });

  it("floors the timer delay at 5 s", async () => {
    const until = new Date(NOW + 500).toISOString();
    getAutonomy.mockResolvedValue(
      read({ admission: { ok: false, reason: "cooldown", detail: "", until } }),
    );
    renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(AUTONOMY_TIMER_FLOOR_MS - 1);
    });
    expect(getAutonomy).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2);
    });
    expect(getAutonomy).toHaveBeenCalledTimes(2);
  });

  it("retries a failed timer read on the next tick and keeps the last good data", async () => {
    const until = new Date(NOW + 10_000).toISOString();
    getAutonomy.mockResolvedValueOnce(
      read({ pending_wakes: 2, admission: { ok: false, reason: "cooldown", detail: "", until } }),
    );
    const { result } = renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    getAutonomy.mockRejectedValueOnce(new Error("boom"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(12_000);
    });
    expect(result.current.error).toBe(true);
    expect(result.current.value?.pending_wakes).toBe(2);
    getAutonomy.mockResolvedValue(read({ pending_wakes: 2 }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(AUTONOMY_RETRY_MS + 100);
    });
    expect(result.current.error).toBe(false);
  });

  it("clears its timers on unmount", async () => {
    const until = new Date(NOW + 10_000).toISOString();
    getAutonomy.mockResolvedValue(
      read({ admission: { ok: false, reason: "cooldown", detail: "", until } }),
    );
    const { unmount } = renderHook(() => useAutonomy("w1", "c1", true));
    await flush();
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });
    expect(getAutonomy).toHaveBeenCalledTimes(1);
  });

  it("drops the previous coordinator's value on a switch", async () => {
    getAutonomy.mockResolvedValueOnce(read({ pending_wakes: 4 }));
    const { result, rerender } = renderHook(({ id }) => useAutonomy("w1", id, true), {
      initialProps: { id: "c1" },
    });
    await flush();
    expect(result.current.value?.pending_wakes).toBe(4);
    getAutonomy.mockReturnValueOnce(new Promise(() => {}));
    rerender({ id: "c2" });
    expect(result.current.value).toBeNull();
  });
});
