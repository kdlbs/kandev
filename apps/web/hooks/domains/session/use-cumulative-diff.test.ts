import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, act } from "@testing-library/react";

const mockRequest = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

let storeState: Record<string, unknown> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(storeState),
}));

import { useCumulativeDiff, invalidateCumulativeDiffCache } from "./use-cumulative-diff";

// Mirrors COALESCE_WINDOW_MS in the hook; advance past it to flush the timer.
const WINDOW = 250;

function setStore() {
  // No environment mapping → envKey resolves to the sessionId itself.
  storeState = {
    environmentIdBySessionId: {} as Record<string, string>,
  };
}

describe("useCumulativeDiff invalidation coalescing", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    setStore();
    mockRequest.mockResolvedValue({ cumulative_diff: { session_id: "x", files: {} } });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("collapses a burst of invalidations into a single trailing refetch", async () => {
    const sid = "sess-burst";
    renderHook(() => useCumulativeDiff(sid));

    // Drain the mount fetch.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mockRequest).toHaveBeenCalledTimes(1);
    mockRequest.mockClear();

    // Fire 5 invalidations within the coalesce window.
    act(() => {
      for (let i = 0; i < 5; i += 1) invalidateCumulativeDiffCache(sid);
    });
    // Nothing should have fired yet (trailing edge).
    expect(mockRequest).toHaveBeenCalledTimes(0);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(WINDOW);
    });
    expect(mockRequest).toHaveBeenCalledTimes(1);
  });

  it("does not refetch after an unmounted subscriber's coalesced timer fires", async () => {
    const sid = "sess-unmount";
    const { unmount } = renderHook(() => useCumulativeDiff(sid));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    mockRequest.mockClear();

    act(() => {
      invalidateCumulativeDiffCache(sid);
    });
    // Unmount before the window elapses. The timer is NOT cleared (it is shared
    // per envKey), but the listener-subscription cleanup removed this hook's
    // handler, so the timer firing is a no-op for the unmounted subscriber.
    act(() => {
      unmount();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(WINDOW);
    });
    expect(mockRequest).toHaveBeenCalledTimes(0);
  });

  it("uses independent timers per envKey", async () => {
    const sidA = "sess-A";
    const sidB = "sess-B";
    renderHook(() => useCumulativeDiff(sidA));
    renderHook(() => useCumulativeDiff(sidB));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(mockRequest).toHaveBeenCalledTimes(2); // one mount fetch each
    mockRequest.mockClear();

    act(() => {
      invalidateCumulativeDiffCache(sidA);
      invalidateCumulativeDiffCache(sidB);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(WINDOW);
    });
    // One refetch per envKey — they don't collapse into each other.
    expect(mockRequest).toHaveBeenCalledTimes(2);
  });
});
