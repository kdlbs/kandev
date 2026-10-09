import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useNowTick } from "./use-now-tick";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useNowTick", () => {
  it("advances every 30 seconds and stops on unmount", () => {
    const { result, unmount } = renderHook(() => useNowTick());
    const initial = result.current;

    act(() => {
      vi.advanceTimersByTime(30_000);
    });
    expect(result.current).toBeGreaterThan(initial);

    const afterFirstTick = result.current;
    unmount();

    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(result.current).toBe(afterFirstTick);
  });
});
