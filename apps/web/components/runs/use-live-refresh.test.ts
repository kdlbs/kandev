import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LIVE_REFRESH_INTERVAL_MS, useLiveRefresh } from "./use-live-refresh";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useLiveRefresh", () => {
  it("re-asks while something is in flight", async () => {
    // A run ends without the page asking, so a row that says "Running" would
    // keep saying it until the user reloaded.
    const refresh = vi.fn();
    renderHook(() => useLiveRefresh(true, refresh));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS * 2);
    });

    expect(refresh).toHaveBeenCalledTimes(2);
  });

  it("makes no requests at all when nothing is running", async () => {
    const refresh = vi.fn();
    renderHook(() => useLiveRefresh(false, refresh));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS * 5);
    });

    expect(refresh).not.toHaveBeenCalled();
  });

  it("stops once the last run finishes", async () => {
    const refresh = vi.fn();
    const { rerender } = renderHook(({ active }) => useLiveRefresh(active, refresh), {
      initialProps: { active: true },
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
    });
    expect(refresh).toHaveBeenCalledTimes(1);

    rerender({ active: false });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS * 3);
    });

    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it("keeps ticking when the caller rebuilds its refresh every render", async () => {
    // Callers construct refresh inline. Depending on it would restart the
    // interval on every render, so it would never actually fire.
    let calls = 0;
    const { rerender } = renderHook(() => useLiveRefresh(true, () => (calls += 1)));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS / 2);
    });
    rerender();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS / 2 + 1);
    });

    expect(calls).toBe(1);
  });

  it("calls the newest refresh, not the one it was mounted with", async () => {
    const stale = vi.fn();
    const fresh = vi.fn();
    const { rerender } = renderHook(({ refresh }) => useLiveRefresh(true, refresh), {
      initialProps: { refresh: stale },
    });

    rerender({ refresh: fresh });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
    });

    expect(fresh).toHaveBeenCalledTimes(1);
    expect(stale).not.toHaveBeenCalled();
  });
});
