import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import type { MessageSearchHit } from "@/lib/api/domains/session-api";

const mockSearch = vi.fn();

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: vi.fn(),
  listSessionTurns: vi.fn(),
  listTaskSessionMessages: vi.fn(),
  listTaskSessions: vi.fn(),
  searchSessionMessages: (sessionId: string, query: string, limit: number) =>
    mockSearch(sessionId, query, limit),
}));

import { useSessionSearch } from "./use-session-search";

function makeHit(id: string): MessageSearchHit {
  return {
    id,
    author_type: "agent",
    type: "text",
    snippet: id,
    created_at: new Date().toISOString(),
  };
}

/** Drain the microtask queue so pending promise continuations run. */
async function flush() {
  await Promise.resolve();
  await Promise.resolve();
}

function renderSessionSearch(sessionId: string | null, loadOlder?: () => Promise<number>) {
  const client = makeQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return renderHook(() => useSessionSearch(sessionId, loadOlder), { wrapper });
}

describe("useSessionSearch", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockSearch.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
    cleanup();
  });

  it("debounces rapid setQuery calls into a single search", async () => {
    mockSearch.mockResolvedValue({ hits: [makeHit("m1")], total: 1 });
    const { result } = renderSessionSearch("sess-1");

    act(() => {
      result.current.open();
      result.current.setQuery("a");
      result.current.setQuery("ab");
      result.current.setQuery("abc");
    });
    expect(mockSearch).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(250);
      await flush();
    });
    expect(mockSearch).toHaveBeenCalledTimes(1);
    expect(mockSearch).toHaveBeenCalledWith("sess-1", "abc", 50);
  });

  it("drops responses from superseded requests", async () => {
    let resolveFirst: (v: { hits: MessageSearchHit[]; total: number }) => void = () => {};
    const first = new Promise<{ hits: MessageSearchHit[]; total: number }>((r) => {
      resolveFirst = r;
    });
    mockSearch
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce({ hits: [makeHit("second")], total: 1 });

    const { result } = renderSessionSearch("sess-1");
    act(() => {
      result.current.open();
      result.current.setQuery("first");
    });
    await act(async () => {
      vi.advanceTimersByTime(200);
      await flush();
    });
    act(() => {
      result.current.setQuery("second");
    });
    await act(async () => {
      vi.advanceTimersByTime(200);
      await flush();
    });
    // Second response has landed.
    expect(result.current.hits[0]?.id).toBe("second");

    // Let the superseded request finally resolve — it must not overwrite state.
    await act(async () => {
      resolveFirst({ hits: [makeHit("stale")], total: 1 });
      await flush();
    });
    expect(result.current.hits[0]?.id).toBe("second");
    expect(result.current.hits.some((h) => h.id === "stale")).toBe(false);
  });

  it("skips the request when sessionId is null", async () => {
    const { result } = renderSessionSearch(null);
    act(() => {
      result.current.open();
      result.current.setQuery("anything");
    });
    await act(async () => {
      vi.advanceTimersByTime(500);
      await flush();
    });
    expect(mockSearch).not.toHaveBeenCalled();
  });

  it("clears hits and query when closed", async () => {
    mockSearch.mockResolvedValue({ hits: [makeHit("m1")], total: 1 });
    const { result } = renderSessionSearch("sess-1");
    act(() => {
      result.current.open();
      result.current.setQuery("foo");
    });
    await act(async () => {
      vi.advanceTimersByTime(200);
      await flush();
    });
    expect(result.current.hits.length).toBe(1);
    act(() => {
      result.current.close();
    });
    expect(result.current.hits).toEqual([]);
    expect(result.current.query).toBe("");
  });
});
