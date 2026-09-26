import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMinVisibleDuration } from "./use-min-visible-duration";

describe("useMinVisibleDuration", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("turns on immediately", () => {
    const { result, rerender } = renderHook(({ active }) => useMinVisibleDuration(active, 450), {
      initialProps: { active: false },
    });
    expect(result.current).toBe(false);

    rerender({ active: true });
    expect(result.current).toBe(true);
  });

  it("holds on for the minimum duration when the flag clears immediately", () => {
    const { result, rerender } = renderHook(({ active }) => useMinVisibleDuration(active, 450), {
      initialProps: { active: true },
    });

    rerender({ active: false });
    expect(result.current).toBe(true);

    act(() => {
      vi.advanceTimersByTime(449);
    });
    expect(result.current).toBe(true);

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe(false);
  });

  it("clears without extra delay once the minimum has already elapsed", () => {
    const { result, rerender } = renderHook(({ active }) => useMinVisibleDuration(active, 450), {
      initialProps: { active: true },
    });

    act(() => {
      vi.advanceTimersByTime(600);
    });
    expect(result.current).toBe(true);

    rerender({ active: false });
    expect(result.current).toBe(false);
  });

  it("restarts the window when it reactivates during the tail", () => {
    const { result, rerender } = renderHook(({ active }) => useMinVisibleDuration(active, 450), {
      initialProps: { active: true },
    });

    rerender({ active: false });
    act(() => {
      vi.advanceTimersByTime(400);
    });
    rerender({ active: true });
    rerender({ active: false });

    // The pending 50ms tail from the first window must not settle the second.
    act(() => {
      vi.advanceTimersByTime(100);
    });
    expect(result.current).toBe(true);

    act(() => {
      vi.advanceTimersByTime(350);
    });
    expect(result.current).toBe(false);
  });

  it("stays off while the flag is never set", () => {
    const { result } = renderHook(() => useMinVisibleDuration(false, 450));
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current).toBe(false);
  });
});
