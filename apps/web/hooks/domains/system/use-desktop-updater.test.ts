import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { DesktopUpdaterAdapter } from "@/lib/desktop/updater-adapter";
import type { DesktopUpdateState } from "@/lib/desktop/protocol";
import { useDesktopUpdater } from "./use-desktop-updater";

const STABLE_POLL_INTERVAL_MS = 5_000;

function updateState(overrides: Partial<DesktopUpdateState> = {}): DesktopUpdateState {
  return {
    phase: "idle",
    currentVersion: "1.0.0",
    latestVersion: null,
    releaseNotes: null,
    releaseUrl: null,
    checkedAtEpochMs: null,
    downloadedBytes: null,
    totalBytes: null,
    installSupported: true,
    installUnsupportedReason: null,
    error: null,
    ...overrides,
  };
}

function adapter(getState: DesktopUpdaterAdapter["getState"]): DesktopUpdaterAdapter {
  return {
    isAvailable: () => true,
    getState,
    checkForUpdates: vi.fn(),
    installUpdate: vi.fn(),
  };
}

function setVisibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
});

describe("useDesktopUpdater background synchronization", () => {
  it("learns automatic native state changes on a bounded stable-state cadence", async () => {
    vi.useFakeTimers();
    const getState = vi
      .fn<DesktopUpdaterAdapter["getState"]>()
      .mockResolvedValueOnce(updateState())
      .mockResolvedValue(updateState({ phase: "available", latestVersion: "1.1.0" }));
    const updater = adapter(getState);
    const { result } = renderHook(() => useDesktopUpdater(updater));

    await act(async () => undefined);
    expect(result.current.state?.phase).toBe("idle");

    await act(async () => vi.advanceTimersByTimeAsync(STABLE_POLL_INTERVAL_MS - 1));
    expect(getState).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(result.current.state?.phase).toBe("available");
    expect(getState).toHaveBeenCalledTimes(2);
  });

  it("pauses background polling while hidden and refreshes immediately when visible", async () => {
    vi.useFakeTimers();
    setVisibility("hidden");
    const getState = vi
      .fn<DesktopUpdaterAdapter["getState"]>()
      .mockResolvedValueOnce(updateState())
      .mockResolvedValue(updateState({ phase: "up-to-date", checkedAtEpochMs: 42 }));
    const updater = adapter(getState);
    const { result } = renderHook(() => useDesktopUpdater(updater));

    await act(async () => undefined);
    await act(async () => vi.advanceTimersByTimeAsync(STABLE_POLL_INTERVAL_MS * 2));
    expect(getState).toHaveBeenCalledTimes(1);

    await act(async () => setVisibility("visible"));
    expect(getState).toHaveBeenCalledTimes(2);
    expect(result.current.state?.phase).toBe("up-to-date");
  });
});
