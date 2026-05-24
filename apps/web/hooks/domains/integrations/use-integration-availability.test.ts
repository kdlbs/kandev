import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { waitFor } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import {
  useIntegrationAuthed,
  useIntegrationAvailable,
  type IntegrationConfigStatus,
} from "./use-integration-availability";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeConfig(overrides: Partial<IntegrationConfigStatus> = {}): IntegrationConfigStatus {
  return { hasSecret: true, lastOk: true, ...overrides };
}

function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);

// ---------------------------------------------------------------------------
// useIntegrationAuthed
// ---------------------------------------------------------------------------

describe("useIntegrationAuthed", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns true when the config has a secret and lastOk=true", async () => {
    const fetchFn = vi.fn<[], Promise<IntegrationConfigStatus | null>>().mockResolvedValue(
      makeConfig(),
    );
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-authed", fetchFn),
    );
    await waitFor(() => expect(result.current).toBe(true));
    expect(fetchFn).toHaveBeenCalledOnce();
  });

  it("returns false while the query is loading (no cached data)", () => {
    // Never resolves during this test
    const fetchFn = vi.fn<[], Promise<IntegrationConfigStatus | null>>().mockReturnValue(
      new Promise(() => {}),
    );
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-loading", fetchFn),
    );
    // Immediately after mount, data is undefined → authed = false
    expect(result.current).toBe(false);
  });

  it("returns false when hasSecret is false", async () => {
    const fetchFn = vi
      .fn<[], Promise<IntegrationConfigStatus | null>>()
      .mockResolvedValue(makeConfig({ hasSecret: false }));
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-no-secret", fetchFn),
    );
    await waitFor(() => expect(fetchFn).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when lastOk is false", async () => {
    const fetchFn = vi
      .fn<[], Promise<IntegrationConfigStatus | null>>()
      .mockResolvedValue(makeConfig({ lastOk: false }));
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-last-failed", fetchFn),
    );
    await waitFor(() => expect(fetchFn).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the config request throws", async () => {
    const fetchFn = vi
      .fn<[], Promise<IntegrationConfigStatus | null>>()
      .mockRejectedValue(new Error("network error"));
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-throws", fetchFn),
    );
    // After the error the query enters error state; data is undefined → false
    await waitFor(() => expect(fetchFn).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the config request returns null (204)", async () => {
    const fetchFn = vi
      .fn<[], Promise<IntegrationConfigStatus | null>>()
      .mockResolvedValue(null);
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-null", fetchFn),
    );
    await waitFor(() => expect(fetchFn).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("skips fetching when active=false", async () => {
    const fetchFn = vi.fn<[], Promise<IntegrationConfigStatus | null>>().mockResolvedValue(
      makeConfig(),
    );
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAuthed("test-inactive", fetchFn, false),
    );
    // Give React a tick to settle — fetchFn must NOT have been called
    await new Promise((r) => setTimeout(r, 50));
    expect(fetchFn).not.toHaveBeenCalled();
    expect(result.current).toBe(false);
  });

  it("does not reset to false between refetch ticks while auth stays healthy", async () => {
    vi.useFakeTimers();
    try {
      const fetchFn = vi
        .fn<[], Promise<IntegrationConfigStatus | null>>()
        .mockResolvedValue(makeConfig());
      const seen: boolean[] = [];
      const { result } = renderHookWithQueryClient(() => {
        const v = useIntegrationAuthed("test-no-flicker", fetchFn);
        seen.push(v);
        return v;
      });
      // Wait for the initial probe to resolve
      await vi.waitFor(() => expect(result.current).toBe(true));
      const beforeTick = [...seen];
      // Advance past the 90s refetch interval
      await vi.advanceTimersByTimeAsync(95_000);
      expect(result.current).toBe(true);
      // No false values should have appeared between ticks
      const newRenders = seen.slice(beforeTick.length);
      expect(newRenders).not.toContain(false);
    } finally {
      vi.useRealTimers();
    }
  });
});

// ---------------------------------------------------------------------------
// useIntegrationAvailable
// ---------------------------------------------------------------------------

describe("useIntegrationAvailable", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns true when enabled=true, loaded=true, and auth is healthy", async () => {
    const fetchFn = vi.fn<[], Promise<IntegrationConfigStatus | null>>().mockResolvedValue(
      makeConfig(),
    );
    const useEnabled = vi.fn().mockReturnValue({ enabled: true, loaded: true });
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAvailable({ kind: "test-avail", useEnabled, fetchConfig: fetchFn }),
    );
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns false when enabled=false even if auth is healthy", async () => {
    const fetchFn = vi.fn<[], Promise<IntegrationConfigStatus | null>>().mockResolvedValue(
      makeConfig(),
    );
    const useEnabled = vi.fn().mockReturnValue({ enabled: false, loaded: true });
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAvailable({ kind: "test-avail-off", useEnabled, fetchConfig: fetchFn }),
    );
    await new Promise((r) => setTimeout(r, 50));
    expect(result.current).toBe(false);
    // The fetch must NOT be called since enabled=false → active=false
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("returns false when loaded=false (toggle not yet settled)", async () => {
    const fetchFn = vi.fn<[], Promise<IntegrationConfigStatus | null>>().mockResolvedValue(
      makeConfig(),
    );
    const useEnabled = vi.fn().mockReturnValue({ enabled: true, loaded: false });
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAvailable({ kind: "test-not-loaded", useEnabled, fetchConfig: fetchFn }),
    );
    await new Promise((r) => setTimeout(r, 50));
    expect(result.current).toBe(false);
    expect(fetchFn).not.toHaveBeenCalled();
  });

  it("returns false when auth is unhealthy (lastOk=false)", async () => {
    const fetchFn = vi
      .fn<[], Promise<IntegrationConfigStatus | null>>()
      .mockResolvedValue(makeConfig({ lastOk: false }));
    const useEnabled = vi.fn().mockReturnValue({ enabled: true, loaded: true });
    const { result } = renderHookWithQueryClient(() =>
      useIntegrationAvailable({ kind: "test-last-fail", useEnabled, fetchConfig: fetchFn }),
    );
    await waitFor(() => expect(fetchFn).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });
});
