import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { StartupSnapshot } from "@/lib/startup-progress/types";

const mocks = vi.hoisted(() => ({ fetchJson: vi.fn() }));

vi.mock("@/lib/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/client")>();
  return { ...actual, fetchJson: mocks.fetchJson };
});

import { useStartupProgress } from "./use-startup-progress";

function snapshot(overrides: Partial<StartupSnapshot> = {}): StartupSnapshot {
  return {
    phase: "applying_migrations",
    boot: 1,
    seq: 1,
    elapsed_ms: 1000,
    phase_elapsed_ms: 1000,
    ...overrides,
  };
}

beforeEach(() => {
  mocks.fetchJson.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useStartupProgress", () => {
  it("does not poll while disabled", async () => {
    renderHook(() => useStartupProgress(false));
    await Promise.resolve();
    expect(mocks.fetchJson).not.toHaveBeenCalled();
  });

  it("applies the snapshot from a successful poll", async () => {
    mocks.fetchJson.mockResolvedValue({ startup: snapshot() });
    const { result } = renderHook(() => useStartupProgress(true));

    await waitFor(() => expect(result.current.snapshot).not.toBeNull());
    expect(result.current.snapshot?.phase).toBe("applying_migrations");
    expect(result.current.lastKnown).toBe(false);
  });

  it("treats a parseable 503 body as fresh data, not a failure", async () => {
    mocks.fetchJson.mockRejectedValue(
      new ApiError("starting", 503, {
        status: "starting",
        startup: snapshot({ phase: "opening_database" }),
      }),
    );
    const { result } = renderHook(() => useStartupProgress(true));

    await waitFor(() => expect(result.current.snapshot?.phase).toBe("opening_database"));
    expect(result.current.lastKnown).toBe(false);
  });

  it("falls back to last known on a response with no parseable snapshot", async () => {
    vi.useFakeTimers();
    mocks.fetchJson
      .mockResolvedValueOnce({ startup: snapshot() })
      .mockRejectedValueOnce(new TypeError("fetch failed"));
    const { result } = renderHook(() => useStartupProgress(true));

    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.snapshot).not.toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(result.current.lastKnown).toBe(true);
    expect(result.current.snapshot?.phase).toBe("applying_migrations");
  });

  it("treats a successful response with no snapshot as ready, not last known", async () => {
    vi.useFakeTimers();
    mocks.fetchJson
      .mockResolvedValueOnce({ startup: snapshot() })
      .mockResolvedValueOnce({ service: "kandev", version: "e2e-test", status: "ok" });
    const { result } = renderHook(() => useStartupProgress(true));

    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.snapshot).not.toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(result.current.lastKnown).toBe(false);
  });

  it("treats invalid JSON from a successful response as ready", async () => {
    vi.useFakeTimers();
    mocks.fetchJson
      .mockResolvedValueOnce({ startup: snapshot() })
      .mockRejectedValueOnce(new ApiError("invalid JSON", 200, null));
    const { result } = renderHook(() => useStartupProgress(true));

    await act(async () => {
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(result.current.lastKnown).toBe(false);
    expect(result.current.snapshot).not.toBeNull();
  });
});

describe("useStartupProgress validation and cleanup", () => {
  it("keeps the last valid snapshot when a later body is malformed", async () => {
    vi.useFakeTimers();
    const first = snapshot();
    mocks.fetchJson
      .mockResolvedValueOnce({ startup: first })
      .mockResolvedValueOnce({ startup: { ...first, step: {} } });
    const { result } = renderHook(() => useStartupProgress(true));

    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.snapshot).toEqual(first);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(result.current.snapshot).toEqual(first);
    expect(result.current.lastKnown).toBe(false);
  });

  it("skips a tick while the previous poll is still outstanding", async () => {
    vi.useFakeTimers();
    let resolveFirst!: (value: { startup: StartupSnapshot }) => void;
    mocks.fetchJson.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );

    renderHook(() => useStartupProgress(true));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(mocks.fetchJson).toHaveBeenCalledTimes(1);

    resolveFirst({ startup: snapshot() });
    await act(async () => {
      await Promise.resolve();
    });
  });

  it("resets to null when disabled after being enabled", async () => {
    mocks.fetchJson.mockResolvedValue({ startup: snapshot() });
    const { result, rerender } = renderHook(({ enabled }) => useStartupProgress(enabled), {
      initialProps: { enabled: true },
    });

    await waitFor(() => expect(result.current.snapshot).not.toBeNull());

    rerender({ enabled: false });
    expect(result.current.snapshot).toBeNull();
    expect(result.current.lastKnown).toBe(false);
  });

  it("aborts an active request when disabled", async () => {
    let signal: AbortSignal | undefined;
    mocks.fetchJson.mockImplementation((_path: string, options: { init?: RequestInit }) => {
      signal = options.init?.signal as AbortSignal | undefined;
      return new Promise(() => {});
    });
    const { rerender } = renderHook(({ enabled }) => useStartupProgress(enabled), {
      initialProps: { enabled: true },
    });

    await waitFor(() => expect(signal).toBeDefined());
    rerender({ enabled: false });

    expect(signal?.aborted).toBe(true);
  });
});
