import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AutonomyRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { useAutonomy } from "./use-autonomy";
import { usePauseControl } from "./use-pause-control";

const getAutonomy = vi.fn();
const putCoordinatorPause = vi.fn();
vi.mock("@/lib/api/domains/coordinator-autonomy-api", () => ({
  getAutonomy: (...a: unknown[]) => getAutonomy(...a),
  putCoordinatorPause: (...a: unknown[]) => putCoordinatorPause(...a),
}));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));

function read(over: Partial<AutonomyRead> = {}): AutonomyRead {
  return {
    server_time: "2026-09-30T10:00:00Z",
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
    paused: false,
    paused_at: null,
    paused_by: null,
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

const flush = () => act(async () => void (await Promise.resolve()));

function setup() {
  return renderHook(() => {
    const autonomy = useAutonomy("w1", "c1", true);
    return { autonomy, pause: usePauseControl("w1", "c1", autonomy) };
  });
}

const PAUSED = {
  paused: true,
  paused_at: "2026-09-30T10:01:00Z",
  paused_by: { id: "u1", name: "Ada" },
};

beforeEach(() => {
  getAutonomy.mockReset();
  putCoordinatorPause.mockReset();
  getAutonomy.mockResolvedValue(read());
});

describe("usePauseControl", () => {
  it("applies the response as a read issued at the click and disables while in flight", async () => {
    const request = deferred<typeof PAUSED>();
    putCoordinatorPause.mockReturnValue(request.promise);
    const { result } = setup();
    await flush();
    act(() => result.current.pause.setPaused(true));
    expect(result.current.pause.pending).toBe(true);
    act(() => result.current.pause.setPaused(true));
    expect(putCoordinatorPause).toHaveBeenCalledTimes(1);
    await act(async () => request.resolve(PAUSED));
    expect(result.current.pause.pending).toBe(false);
    expect(result.current.autonomy.value?.paused).toBe(true);
    expect(result.current.autonomy.value?.paused_by?.name).toBe("Ada");
    expect(putCoordinatorPause).toHaveBeenCalledWith("w1", "c1", true);
  });

  it("lets a read issued after the click outrank the response", async () => {
    const request = deferred<typeof PAUSED>();
    putCoordinatorPause.mockReturnValue(request.promise);
    const { result } = setup();
    await flush();
    act(() => result.current.pause.setPaused(true));
    getAutonomy.mockResolvedValueOnce(read({ paused: false, pending_wakes: 4 }));
    act(() => result.current.autonomy.retry());
    await flush();
    await act(async () => request.resolve(PAUSED));
    expect(result.current.autonomy.value?.paused).toBe(false);
    expect(result.current.autonomy.value?.pending_wakes).toBe(4);
  });

  it("outranks a read issued before the click", async () => {
    const early = deferred<AutonomyRead>();
    getAutonomy.mockReturnValueOnce(early.promise);
    const request = deferred<typeof PAUSED>();
    putCoordinatorPause.mockReturnValue(request.promise);
    const { result } = renderHook(() => {
      const autonomy = useAutonomy("w1", "c1", true);
      return { autonomy, pause: usePauseControl("w1", "c1", autonomy) };
    });
    await flush();
    await act(async () => early.resolve(read()));
    getAutonomy.mockReturnValueOnce(deferred<AutonomyRead>().promise);
    act(() => result.current.autonomy.retry());
    act(() => result.current.pause.setPaused(true));
    await act(async () => request.resolve(PAUSED));
    expect(result.current.autonomy.value?.paused).toBe(true);
  });

  it("shows the failure and re-reads the latest state when the request fails", async () => {
    putCoordinatorPause.mockRejectedValue(new Error("boom"));
    const { result } = setup();
    await flush();
    getAutonomy.mockResolvedValueOnce(read({ paused: true, paused_at: PAUSED.paused_at }));
    act(() => result.current.pause.setPaused(true));
    await flush();
    await flush();
    expect(result.current.pause.failed).toBe(true);
    expect(result.current.pause.pending).toBe(false);
    expect(result.current.autonomy.value?.paused).toBe(true);
  });

  it("clears the failure on the next attempt", async () => {
    putCoordinatorPause.mockRejectedValueOnce(new Error("boom"));
    const { result } = setup();
    await flush();
    act(() => result.current.pause.setPaused(true));
    await flush();
    await flush();
    putCoordinatorPause.mockResolvedValueOnce({ paused: true, paused_at: null, paused_by: null });
    act(() => result.current.pause.setPaused(true));
    expect(result.current.pause.failed).toBe(false);
    await flush();
    await flush();
    expect(result.current.autonomy.value?.paused).toBe(true);
  });
});
