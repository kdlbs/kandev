import { useCallback, useState } from "react";
import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook, cleanup, waitFor, act } from "@testing-library/react";

// A faithful-enough fake of `useLazyLoadMessages`: real `useState` so calling
// its own setters genuinely re-renders consumers (unlike a static mock
// return value), letting the reactive drain hook under test observe fresh
// `hasMore`/`isLoading` exactly as it would against the real store-backed hook.
let mockPages: number[] = [];
let mockLoadMoreCalls = 0;
// When set, overrides the page-array-driven default: called with the same
// `setState` the fake hook itself uses, so every path (page-array or custom)
// updates `hasMore`/`isLoading` identically and consistently.
let mockLoadMoreImpl:
  | ((setState: (state: { hasMore: boolean; isLoading: boolean }) => void) => Promise<number>)
  | null = null;

function useLazyLoadMessagesFake() {
  const [state, setState] = useState({ hasMore: true, isLoading: false });
  const loadMore = useCallback(async () => {
    mockLoadMoreCalls++;
    if (mockLoadMoreImpl) return mockLoadMoreImpl(setState);
    const index = mockLoadMoreCalls - 1;
    const fetched = mockPages[index] ?? 0;
    const hasMore = index + 1 < mockPages.length;
    setState({ hasMore, isLoading: false });
    return fetched;
  }, []);
  return { loadMore, hasMore: state.hasMore, isLoading: state.isLoading };
}

vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => useLazyLoadMessagesFake(),
}));

import { useDrainOlderMessages } from "./use-drain-older-messages";

afterEach(() => {
  cleanup();
  mockPages = [];
  mockLoadMoreCalls = 0;
  mockLoadMoreImpl = null;
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

  it("stops at the batch cap when the session never reports exhaustion", async () => {
    mockPages = new Array(200).fill(20);
    const { result } = renderHook(() => useDrainOlderMessages("s1", true));
    await waitFor(() => expect(result.current.isDraining).toBe(false));
    expect(mockLoadMoreCalls).toBe(50);
  });

  it("waits for a concurrent in-flight load instead of racing it, then resumes draining", async () => {
    // Simulates another caller (e.g. the last-prompt preload effect) already
    // holding the load in flight when the drain would otherwise fire: the
    // reactive design gates every call on `!isLoading`, so it can never
    // observe an ambiguous "0 fetched" from someone else's no-op — it waits
    // for that fetch to resolve and re-reads the real `hasMore` it leaves behind.
    let resolveConcurrent: (value: number) => void = () => {};
    let callCount = 0;
    mockLoadMoreImpl = (setState) => {
      callCount++;
      if (callCount === 1) {
        setState({ hasMore: true, isLoading: true });
        return new Promise<number>((resolve) => {
          resolveConcurrent = (value) => {
            setState({ hasMore: true, isLoading: false });
            resolve(value);
          };
        });
      }
      const hasMore = callCount < 3;
      setState({ hasMore, isLoading: false });
      return Promise.resolve(hasMore ? 20 : 0);
    };
    const { result, rerender } = renderHook(
      ({ active }: { active: boolean }) => useDrainOlderMessages("s1", active),
      { initialProps: { active: true } },
    );
    expect(result.current.isDraining).toBe(true);
    expect(callCount).toBe(1);

    // Concurrent caller's fetch finally resolves (a real page, not empty) —
    // the drain hook only sees this via the reactive hasMore/isLoading update.
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
      setState({ hasMore: true, isLoading: true });
      return new Promise<number>((resolve) => {
        resolveFirst = (value) => {
          setState({ hasMore: true, isLoading: false });
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
