import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockSearchWorkspaceContent = vi.fn();
const mockClient = { request: vi.fn() };

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockClient,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  searchWorkspaceContent: (...args: unknown[]) => mockSearchWorkspaceContent(...args),
}));

import { useWorkspaceContentSearch } from "./use-workspace-content-search";

beforeEach(() => {
  vi.useFakeTimers();
  mockSearchWorkspaceContent.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useWorkspaceContentSearch request lifecycle", () => {
  it("debounces a trimmed query for the active session", async () => {
    const results = [
      {
        repository_name: "web",
        path: "src/app.tsx",
        line: 4,
        column: 2,
        preview: "needle",
        match_ranges: [{ start: 0, end: 6 }],
      },
    ];
    mockSearchWorkspaceContent.mockResolvedValue({ results });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "  needle  ",
        sessionId: "session-1",
      }),
    );

    expect(result.current.isSearching).toBe(true);
    await act(async () => vi.advanceTimersByTimeAsync(249));
    expect(mockSearchWorkspaceContent).not.toHaveBeenCalled();

    await act(async () => vi.runAllTimersAsync());

    expect(mockSearchWorkspaceContent).toHaveBeenCalledWith(mockClient, "session-1", "needle", 50);
    expect(mockSearchWorkspaceContent).toHaveBeenCalledTimes(8);
    expect(result.current.results).toEqual(results);
    expect(result.current.isSearching).toBe(false);
  });

  it("publishes matches before retry polling completes", async () => {
    const results = [
      {
        repository_name: "web",
        path: "src/cached-content-target.ts",
        line: 180,
        column: 1,
        preview: "cached marker",
        match_ranges: [{ start: 0, end: 13 }],
      },
    ];
    mockSearchWorkspaceContent.mockResolvedValue({ results });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "cached marker",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(250));

    expect(mockSearchWorkspaceContent).toHaveBeenCalledTimes(1);
    expect(result.current.results).toEqual(results);
    expect(result.current.isSearching).toBe(true);

    await act(async () => vi.runAllTimersAsync());
    expect(result.current.isSearching).toBe(false);
  });

  it("clears the previous query results while the next search is pending", async () => {
    const previousResults = [
      {
        repository_name: "web",
        path: "src/old.ts",
        line: 1,
        column: 1,
        preview: "previous",
        match_ranges: [],
      },
    ];
    mockSearchWorkspaceContent.mockResolvedValue({ results: previousResults });
    const { result, rerender } = renderHook(
      ({ query }) => useWorkspaceContentSearch({ enabled: true, query, sessionId: "session-1" }),
      { initialProps: { query: "previous" } },
    );
    await act(async () => vi.runAllTimersAsync());
    expect(result.current.results).toEqual(previousResults);

    rerender({ query: "next" });

    expect(result.current.results).toEqual([]);
    expect(result.current.isSearching).toBe(true);
  });
});

describe("useWorkspaceContentSearch retry publishing", () => {
  it("publishes the first available results while later repositories are still retrying", async () => {
    const primaryResult = {
      repository_name: "primary",
      path: "src/primary.ts",
      line: 1,
      column: 1,
      preview: "primary needle",
      match_ranges: [],
    };
    mockSearchWorkspaceContent.mockResolvedValue({ results: [primaryResult] });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(250));

    expect(mockSearchWorkspaceContent).toHaveBeenCalledTimes(1);
    expect(result.current.results).toEqual([primaryResult]);
    expect(result.current.isSearching).toBe(true);
  });

  it("clears results without sending when search is disabled", async () => {
    mockSearchWorkspaceContent.mockResolvedValue({ results: [] });
    const { result, rerender } = renderHook(
      ({ enabled, query }) => useWorkspaceContentSearch({ enabled, query, sessionId: "session-1" }),
      { initialProps: { enabled: true, query: "needle" } },
    );

    rerender({ enabled: false, query: "needle" });
    await act(async () => vi.runAllTimersAsync());

    expect(mockSearchWorkspaceContent).not.toHaveBeenCalled();
    expect(result.current.results).toEqual([]);
    expect(result.current.isSearching).toBe(false);
  });
});

describe("useWorkspaceContentSearch validation and failures", () => {
  it("reports when the task has no active session", () => {
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: null,
      }),
    );

    expect(mockSearchWorkspaceContent).not.toHaveBeenCalled();
    expect(result.current.error).toBe("session-unavailable");
  });

  it("rejects queries over 200 Unicode code points without sending", async () => {
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "😀".repeat(201),
        sessionId: "session-1",
      }),
    );
    await act(async () => vi.runAllTimersAsync());

    expect(mockSearchWorkspaceContent).not.toHaveBeenCalled();
    expect(result.current.error).toBe("query-too-long");
    expect(result.current.isSearching).toBe(false);
  });

  it("distinguishes a transport failure from an empty result", async () => {
    mockSearchWorkspaceContent.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.runAllTimersAsync());

    expect(result.current.results).toEqual([]);
    expect(result.current.error).toBe("transport-error");
    expect(result.current.isSearching).toBe(false);
  });

  it("merges repository results that arrive on later retry attempts", async () => {
    const primary = {
      repository_name: "primary",
      path: "src/primary.ts",
      line: 1,
      column: 1,
      preview: "primary needle",
      match_ranges: [],
    };
    const extra = {
      repository_name: "extra",
      path: "src/extra.ts",
      line: 2,
      column: 1,
      preview: "extra needle",
      match_ranges: [],
    };
    mockSearchWorkspaceContent
      .mockResolvedValueOnce({ results: [primary] })
      .mockResolvedValueOnce({ results: [primary, extra] })
      .mockResolvedValue({ results: [primary, extra] });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.runAllTimersAsync());

    expect(result.current.results).toEqual([primary, extra]);
  });

  it("publishes matches before retry polling completes", async () => {
    const match = {
      repository_name: "primary",
      path: "src/match.ts",
      line: 1,
      column: 1,
      preview: "needle",
      match_ranges: [],
    };
    mockSearchWorkspaceContent.mockResolvedValue({ results: [match] });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(250));

    expect(result.current.results).toEqual([match]);
    expect(result.current.isSearching).toBe(true);
  });
});

describe("useWorkspaceContentSearch stale responses", () => {
  it("ignores a response from an outdated query", async () => {
    const firstResults = [
      {
        repository_name: "",
        path: "old.ts",
        line: 1,
        column: 1,
        preview: "first",
        match_ranges: [],
      },
    ];
    const secondResults = [
      {
        repository_name: "",
        path: "new.ts",
        line: 1,
        column: 1,
        preview: "second",
        match_ranges: [],
      },
    ];
    let resolveFirst: ((value: { results: typeof firstResults }) => void) | undefined;
    mockSearchWorkspaceContent
      .mockImplementationOnce(
        () =>
          new Promise<{ results: typeof firstResults }>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce({ results: secondResults });
    const { result, rerender } = renderHook(
      ({ query }) => useWorkspaceContentSearch({ enabled: true, query, sessionId: "session-1" }),
      { initialProps: { query: "first" } },
    );
    await act(async () => vi.advanceTimersByTimeAsync(250));

    rerender({ query: "second" });
    await act(async () => vi.advanceTimersByTimeAsync(250));
    await act(async () => resolveFirst?.({ results: firstResults }));
    await act(async () => vi.runAllTimersAsync());

    expect(mockSearchWorkspaceContent).toHaveBeenCalledTimes(9);
    expect(result.current.results).toEqual(secondResults);
    expect(result.current.isSearching).toBe(false);
  });
});

describe("useWorkspaceContentSearch incremental publishing", () => {
  // The retry loop exists so a repository that registers with the workspace
  // process manager after task launch still contributes matches. Publishing
  // the merged set only once the whole budget was spent put a fixed
  // 250ms + 7 * 500ms = 3.75s floor under every search, which is what made
  // the "cached preview content match" e2e spec fail its 5s assertion under
  // CI load. This pins the per-attempt half of that contract: a repository
  // whose matches only appear on a later attempt is published when that
  // attempt lands, not when the budget runs out.
  it("publishes a late repository's matches as soon as its attempt lands", async () => {
    const primary = {
      repository_name: "primary",
      path: "src/primary.ts",
      line: 1,
      column: 1,
      preview: "primary needle",
      match_ranges: [],
    };
    const extra = {
      repository_name: "extra",
      path: "src/extra.ts",
      line: 2,
      column: 1,
      preview: "extra needle",
      match_ranges: [],
    };
    mockSearchWorkspaceContent
      .mockResolvedValueOnce({ results: [primary] })
      .mockResolvedValue({ results: [primary, extra] });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({ enabled: true, query: "needle", sessionId: "session-1" }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(result.current.results).toEqual([primary]);

    // One retry delay later the second repository has registered.
    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(result.current.results).toEqual([primary, extra]);
    expect(result.current.isSearching).toBe(true);
  });

  it("drops a partial result published by a superseded query", async () => {
    const staleResult = {
      repository_name: "",
      path: "stale.ts",
      line: 1,
      column: 1,
      preview: "stale",
      match_ranges: [],
    };
    const liveResult = {
      repository_name: "",
      path: "live.ts",
      line: 1,
      column: 1,
      preview: "live",
      match_ranges: [],
    };
    let resolveStale: ((value: { results: (typeof staleResult)[] }) => void) | undefined;
    mockSearchWorkspaceContent
      .mockImplementationOnce(
        () =>
          new Promise<{ results: (typeof staleResult)[] }>((resolve) => {
            resolveStale = resolve;
          }),
      )
      .mockResolvedValue({ results: [liveResult] });
    const { result, rerender } = renderHook(
      ({ query }) => useWorkspaceContentSearch({ enabled: true, query, sessionId: "session-1" }),
      { initialProps: { query: "stale" } },
    );
    await act(async () => vi.advanceTimersByTimeAsync(250));

    rerender({ query: "live" });
    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(result.current.results).toEqual([liveResult]);

    // The superseded query's first attempt lands late. Only promises are
    // flushed here, so the live query's next attempt cannot mask an overwrite.
    await act(async () => {
      resolveStale?.({ results: [staleResult] });
    });

    expect(result.current.results).toEqual([liveResult]);
  });
});
