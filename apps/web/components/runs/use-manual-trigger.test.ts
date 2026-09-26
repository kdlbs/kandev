import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mockTrigger = vi.fn();
vi.mock("@/lib/api/domains/automation-api", () => ({
  triggerAutomation: (...args: unknown[]) => mockTrigger(...args),
}));

const toastInfo = vi.fn();
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    info: (...args: unknown[]) => toastInfo(...args),
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

import { useManualTrigger } from "./use-manual-trigger";

const AUTOMATION_ID = "automation-1";

afterEach(() => {
  vi.clearAllMocks();
});

describe("useManualTrigger", () => {
  it("fires the automation it was given", async () => {
    mockTrigger.mockResolvedValue({ triggered: true });
    const onFired = vi.fn();

    const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, onFired));
    await act(async () => {
      await result.current.runNow();
    });

    expect(mockTrigger).toHaveBeenCalledWith(AUTOMATION_ID);
    expect(toastSuccess).toHaveBeenCalledWith("Triggered");
  });

  it("refreshes so the new run is visible without a reload", async () => {
    mockTrigger.mockResolvedValue({ triggered: true });
    const onFired = vi.fn();

    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, onFired));
      await act(async () => {
        await result.current.runNow();
      });

      // The first read goes out immediately.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(onFired).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});

// The orchestrator writes the automation_runs row on its own goroutine, so the
// trigger response can land before the run exists to be read.
describe("useManualTrigger retries", () => {
  // The orchestrator writes the automation_runs row on its own goroutine, so
  // the trigger response can land first. One read would then find nothing —
  // and with no open run the page stops polling, so "Triggered" sits above an
  // empty list until the user reloads by hand.
  it("keeps looking for the run the fire promised", async () => {
    mockTrigger.mockResolvedValue({ triggered: true });
    const onFired = vi.fn();

    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, onFired));
      await act(async () => {
        await result.current.runNow();
      });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(0);
      });
      const immediate = onFired.mock.calls.length;

      await act(async () => {
        await vi.advanceTimersByTimeAsync(5_000);
      });
      expect(onFired.mock.calls.length).toBeGreaterThan(immediate);
    } finally {
      vi.useRealTimers();
    }
  });

  it("stops retrying once the page is gone", async () => {
    mockTrigger.mockResolvedValue({ triggered: true });
    const onFired = vi.fn();

    vi.useFakeTimers();
    try {
      const { result, unmount } = renderHook(() => useManualTrigger(AUTOMATION_ID, onFired));
      await act(async () => {
        await result.current.runNow();
      });
      unmount();
      onFired.mockClear();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(5_000);
      });
      expect(onFired).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports a skip as a skip, with its reason", async () => {
    // The cap turning a request away is the case that made the old trigger feel
    // broken: success was reported and nothing ran.
    mockTrigger.mockResolvedValue({ triggered: false, skipped: true, reason: "already running" });
    const onFired = vi.fn();

    const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, onFired));
    await act(async () => {
      await result.current.runNow();
    });

    expect(toastInfo).toHaveBeenCalledWith("Skipped; already running");
    expect(toastSuccess).not.toHaveBeenCalled();
    // Nothing started, so there is nothing new to show.
    expect(onFired).not.toHaveBeenCalled();
  });

  it("still says it was skipped when the backend gives no reason", async () => {
    mockTrigger.mockResolvedValue({ triggered: false, skipped: true });

    const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, vi.fn()));
    await act(async () => {
      await result.current.runNow();
    });

    expect(toastInfo).toHaveBeenCalledWith("Skipped; already running");
  });

  it("surfaces a failure instead of leaving the click silent", async () => {
    mockTrigger.mockRejectedValue(new Error("workspace not authorized"));

    const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, vi.fn()));
    await act(async () => {
      await result.current.runNow();
    });

    expect(toastError).toHaveBeenCalledWith("workspace not authorized");
    await waitFor(() => expect(result.current.triggering).toBe(false));
  });

  it("does not fire twice while the first request is in flight", async () => {
    let release: (value: unknown) => void = () => {};
    mockTrigger.mockImplementation(
      () =>
        new Promise((resolve) => {
          release = resolve;
        }),
    );

    const { result } = renderHook(() => useManualTrigger(AUTOMATION_ID, vi.fn()));
    act(() => {
      void result.current.runNow();
    });
    await waitFor(() => expect(result.current.triggering).toBe(true));

    await act(async () => {
      await result.current.runNow();
      release({ triggered: true });
    });

    expect(mockTrigger).toHaveBeenCalledTimes(1);
  });

  it("does nothing without an automation id", async () => {
    const { result } = renderHook(() => useManualTrigger("", vi.fn()));
    await act(async () => {
      await result.current.runNow();
    });

    expect(mockTrigger).not.toHaveBeenCalled();
  });
});
