import { useCallback, useState } from "react";
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { renderHook, cleanup, waitFor, act } from "@testing-library/react";

const storeMock = vi.hoisted(() => ({ messagesLoading: false }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      messages: {
        metaBySession: { s1: { isLoading: storeMock.messagesLoading } },
      },
    }),
}));

// A faithful-enough fake of `useLazyLoadMessages`: real `useState` so calling
// its own setters genuinely re-renders consumers (unlike a static mock
// return value), letting the reactive drain hook under test observe fresh
// `hasMore`/`isLoadingMore` exactly as it would against the real store-backed
// hook. `messagesLoading` (initial/refetch) is controlled through the store
// mock separately.
let mockPages: number[] = [];
let mockLoadMoreCalls = 0;
let mockLoadMoreRawCalls = 0;
let mockRawHasMore = true;
// When set, overrides the page-array-driven default: called with the same
// `setState` the fake hook itself uses, so every path (page-array or custom)
// updates `hasMore`/`isLoadingMore` identically and consistently.
let mockLoadMoreImpl:
  | ((setState: (state: { hasMore: boolean; isLoadingMore: boolean }) => void) => Promise<number>)
  | null = null;

function useLazyLoadMessagesFake() {
  const [state, setState] = useState({ hasMore: true, isLoadingMore: false });
  const loadMore = useCallback(async () => {
    mockLoadMoreCalls++;
    if (mockLoadMoreImpl) return mockLoadMoreImpl(setState);
    const index = mockLoadMoreCalls - 1;
    const fetched = mockPages[index] ?? 0;
    const hasMore = index + 1 < mockPages.length;
    setState({ hasMore, isLoadingMore: false });
    return fetched;
  }, []);
  const loadMoreRaw = useCallback(async () => {
    mockLoadMoreRawCalls++;
    const index = mockLoadMoreRawCalls - 1;
    const fetched = mockPages[index] ?? 0;
    mockRawHasMore = index + 1 < mockPages.length;
    setState((current) => ({ ...current, isLoadingMore: false }));
    return fetched;
  }, []);
  return {
    loadMore,
    loadMoreRaw,
    hasMore: state.hasMore,
    rawHasMore: mockRawHasMore,
    isLoadingMore: state.isLoadingMore,
  };
}

vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => useLazyLoadMessagesFake(),
}));

import { useDrainOlderMessages } from "./use-drain-older-messages";

beforeEach(() => {
  storeMock.messagesLoading = false;
});

afterEach(() => {
  cleanup();
  mockPages = [];
  mockLoadMoreCalls = 0;
  mockLoadMoreRawCalls = 0;
  mockRawHasMore = true;
  mockLoadMoreImpl = null;
  storeMock.messagesLoading = false;
});

describe("useDrainOlderMessages", () => {
  it("is idle when inactive", () => {
    mockPages = [20, 0];
    const { result } = renderHook(() => useDrainOlderMessages("s1", false));
    expect(result.current.isDraining).toBe(false);
    expect(mockLoadMoreCalls).toBe(0);
  });

  it("is idle when sessionId is null", () => {
    mockPages = [20, 0];
    const { result } = renderHook(() => useDrainOlderMessages(null, true));
    expect(result.current.isDraining).toBe(false);
    expect(mockLoadMoreCalls).toBe(0);
  });

  it("drains batches until the session reports no more older messages", async () => {
    mockPages = [20, 20, 0];
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    expect(result.current.isDraining).toBe(true);
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(3);
  });

  it("uses raw pagination for reverse search backfill", async () => {
    mockPages = [20, 0];
    const { result } = renderHook(() => useDrainOlderMessages("s1", true, { rawPagination: true }));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(0);
    expect(mockLoadMoreRawCalls).toBe(2);
  });

  it("stops at the 1000-message budget when the session never reports exhaustion", async () => {
    mockPages = new Array(200).fill(20);
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(50); // 50 × 20 = 1000
  });

  it("stops at the first batch reaching or exceeding 1000 messages (overshoot by at most one batch)", async () => {
    // Mixed first-request-wins page sizes: 600 then 600 overshoots by one batch.
    mockPages = [600, 600, 600];
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(2); // 600 + 600 = 1200 >= 1000
  });
});

describe("useDrainOlderMessages — concurrency and lifecycle", () => {
  it("stops immediately after a zero-result batch even when hasMore stays true", async () => {
    // Retained-cursor zero-row drain termination: the third page must not fire.
    mockPages = [20, 0, 20];
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(2);
  });

  it("resets the cumulative counter on reactivation", async () => {
    // First drain: 600 + 0 (zero-row stop). A fresh drain starts from a zero
    // cumulative count, so the budget is not pre-consumed by the earlier drain.
    let callCount = 0;
    mockLoadMoreImpl = (setState) => {
      callCount++;
      if (callCount === 1 || callCount === 2 || callCount === 3) {
        setState({ hasMore: true, isLoadingMore: false });
        return Promise.resolve(600);
      }
      setState({ hasMore: false, isLoadingMore: false });
      return Promise.resolve(600);
    };
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useDrainOlderMessages("s1", active),
      { initialProps: { active: true } },
    );
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(callCount).toBe(2); // 600 + 0 (zero-row stop)

    rerender({ active: false });
    rerender({ active: true });
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(callCount).toBe(4); // fresh drain: 600 + 600 = 1200 >= 1000
  });

  it("blocks batches while an initial/refetch fetch (messagesLoading) is in flight", async () => {
    mockPages = [20, 0];
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useDrainOlderMessages("s1", active),
      { initialProps: { active: true } },
    );
    expect(mockLoadMoreCalls).toBe(1);

    // A turn-settle refetch starts: the drain must not race it.
    storeMock.messagesLoading = true;
    rerender({ active: true });
    await act(async () => {});
    expect(mockLoadMoreCalls).toBe(1);
    storeMock.messagesLoading = false;
    rerender({ active: true });
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(2);
  });
});

describe("useDrainOlderMessages — concurrent loads and teardown", () => {
  it("waits for a concurrent in-flight load instead of racing it, then resumes draining", async () => {
    // Simulates another caller (e.g. the last-prompt preload effect) already
    // holding the load in flight when the drain would otherwise fire: the
    // reactive design gates every call on `!isLoadingMore`, so it can never
    // observe an ambiguous "0 fetched" from someone else's no-op — it waits
    // for that fetch to resolve and re-reads the real `hasMore` it leaves behind.
    let resolveConcurrent: (value: number) => void = () => {};
    let callCount = 0;
    mockLoadMoreImpl = (setState) => {
      callCount++;
      if (callCount === 1) {
        setState({ hasMore: true, isLoadingMore: true });
        return new Promise<number>((resolve) => {
          resolveConcurrent = (value) => {
            setState({ hasMore: true, isLoadingMore: false });
            resolve(value);
          };
        });
      }
      const hasMore = callCount < 3;
      setState({ hasMore, isLoadingMore: false });
      return Promise.resolve(hasMore ? 20 : 0);
    };
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useDrainOlderMessages("s1", active),
      { initialProps: { active: true } },
    );
    expect(result.current.isDraining).toBe(true);
    expect(callCount).toBe(1);

    // Concurrent caller's fetch finally resolves (a real page, not empty) —
    // the drain hook only sees this via the reactive hasMore/isLoadingMore update.
    await act(async () => {
      resolveConcurrent(20);
    });
    rerender({ active: true });
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(callCount).toBe(3);
  });

  it("clears isDraining when active flips to false mid-drain", async () => {
    let resolveFirst: (value: number) => void = () => {};
    mockLoadMoreImpl = (setState) => {
      setState({ hasMore: true, isLoadingMore: true });
      return new Promise<number>((resolve) => {
        resolveFirst = (value) => {
          setState({ hasMore: true, isLoadingMore: false });
          resolve(value);
        };
      });
    };
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useDrainOlderMessages("s1", active),
      { initialProps: { active: true } },
    );
    expect(result.current.isDraining).toBe(true);
    rerender({ active: false });
    expect(result.current.isDraining).toBe(false);
    resolveFirst(20);
  });

  it("stops draining (does not throw) if loadMore rejects", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    mockLoadMoreImpl = () => Promise.reject(new Error("boom"));
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    errorSpy.mockRestore();
  });
});
