import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { mockRequest } = vi.hoisted(() => ({ mockRequest: vi.fn() }));
const mockClient = { request: mockRequest };
const TRANSPORT_ERROR = "transport-error";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockClient,
}));

import { useWorkspaceContentSearch } from "./use-workspace-content-search";

beforeEach(() => {
  vi.useFakeTimers();
  mockRequest.mockReset();
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
    mockRequest.mockResolvedValue({ results });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "  needle  ",
        sessionId: "session-1",
      }),
    );

    expect(result.current.isSearching).toBe(true);
    await act(async () => vi.advanceTimersByTimeAsync(249));
    expect(mockRequest).not.toHaveBeenCalled();

    await act(async () => vi.runAllTimersAsync());

    expect(mockRequest).toHaveBeenCalledWith("workspace.content.search", {
      session_id: "session-1",
      query: "needle",
      limit_per_repo: 50,
    });
    expect(mockRequest).toHaveBeenCalledTimes(8);
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
    mockRequest.mockResolvedValue({ results });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "cached marker",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(250));

    expect(mockRequest).toHaveBeenCalledTimes(1);
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
    mockRequest.mockResolvedValue({ results: previousResults });
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
    mockRequest.mockResolvedValue({ results: [primaryResult] });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(250));

    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(result.current.results).toEqual([primaryResult]);
    expect(result.current.isSearching).toBe(true);
  });

  it("clears results without sending when search is disabled", async () => {
    mockRequest.mockResolvedValue({ results: [] });
    const { result, rerender } = renderHook(
      ({ enabled, query }) => useWorkspaceContentSearch({ enabled, query, sessionId: "session-1" }),
      { initialProps: { enabled: true, query: "needle" } },
    );

    rerender({ enabled: false, query: "needle" });
    await act(async () => vi.runAllTimersAsync());

    expect(mockRequest).not.toHaveBeenCalled();
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

    expect(mockRequest).not.toHaveBeenCalled();
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

    expect(mockRequest).not.toHaveBeenCalled();
    expect(result.current.error).toBe("query-too-long");
    expect(result.current.isSearching).toBe(false);
  });

  it("distinguishes a transport failure from an empty result", async () => {
    mockRequest.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({
        enabled: true,
        query: "needle",
        sessionId: "session-1",
      }),
    );

    await act(async () => vi.runAllTimersAsync());

    expect(result.current.results).toEqual([]);
    expect(result.current.error).toBe(TRANSPORT_ERROR);
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
    mockRequest
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
    mockRequest.mockResolvedValue({ results: [match] });
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
    mockRequest
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

    expect(mockRequest).toHaveBeenCalledTimes(9);
    expect(result.current.results).toEqual(secondResults);
    expect(result.current.isSearching).toBe(false);
  });

  it("ignores results from a replaced workspace repository inventory", async () => {
    const stale = {
      repository_name: "old-checkout",
      path: "src/stale.ts",
      line: 1,
      column: 1,
      preview: "stale inventory",
      match_ranges: [],
    };
    const current = { ...stale, repository_name: "current-checkout", path: "src/current.ts" };
    let resolveStale: ((value: { results: (typeof stale)[] }) => void) | undefined;
    mockRequest
      .mockImplementationOnce(
        () =>
          new Promise<{ results: (typeof stale)[] }>((resolve) => {
            resolveStale = resolve;
          }),
      )
      .mockResolvedValueOnce({ results: [current] });
    const { result, rerender } = renderHook(
      ({ inventoryRevision }) =>
        useWorkspaceContentSearch({
          enabled: true,
          query: "needle",
          sessionId: "session-1",
          inventoryRevision,
        }),
      { initialProps: { inventoryRevision: "before" } },
    );
    await act(async () => vi.advanceTimersByTimeAsync(250));

    rerender({ inventoryRevision: "after" });
    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(result.current.results).toEqual([current]);
    await act(async () => resolveStale?.({ results: [stale] }));

    expect(result.current.results).toEqual([current]);
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
    mockRequest
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
    mockRequest
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

// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.10
// Only request transport is mocked; retries use the actual hook and WS wrapper.
describe("useWorkspaceContentSearch retry recovery", () => {
  it.each([
    [
      "initial failure followed by successful empty responses",
      [false, true, true, true, true, true, true, true],
      null,
    ],
    [
      "middle failure followed by successful empty responses",
      [true, false, true, true, true, true, true, true],
      null,
    ],
    ["recovery on the last attempt", [false, false, false, false, false, false, false, true], null],
    ["healthy empty search", [true, true, true, true, true, true, true, true], null],
    [
      "every attempt fails",
      [false, false, false, false, false, false, false, false],
      TRANSPORT_ERROR,
    ],
    [
      "only the final attempt fails",
      [true, true, true, true, true, true, true, false],
      TRANSPORT_ERROR,
    ],
    [
      "recovery followed by another terminal failure",
      [false, true, true, true, true, true, true, false],
      TRANSPORT_ERROR,
    ],
  ] as const)("settles %s", async (_scenario, successfulAttempts, error) => {
    for (const success of successfulAttempts) {
      if (success) mockRequest.mockResolvedValueOnce({ results: [] });
      else mockRequest.mockRejectedValueOnce(new Error("temporary disconnect"));
    }
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({ enabled: true, query: "needle", sessionId: "session-1" }),
    );

    await act(async () => vi.runAllTimersAsync());

    expect(mockRequest).toHaveBeenCalledTimes(8);
    expect(mockRequest).toHaveBeenLastCalledWith("workspace.content.search", {
      session_id: "session-1",
      query: "needle",
      limit_per_repo: 50,
    });
    expect(result.current).toEqual({ results: [], isSearching: false, error });
  });
});

describe("useWorkspaceContentSearch recovery preserves polling and cancellation", () => {
  it("retains and deduplicates raw repository matches across failed and empty attempts", async () => {
    const root = {
      repository_name: "",
      path: " src/shared.ts ",
      line: 4,
      column: 2,
      preview: "needle",
      match_ranges: [{ start: 0, end: 6 }],
    };
    const child = { ...root, repository_name: "child/raw" };
    mockRequest
      .mockRejectedValueOnce(new Error("initial failure"))
      .mockResolvedValueOnce({ results: [root] })
      .mockRejectedValueOnce(new Error("middle failure"))
      .mockResolvedValueOnce({ results: [root, child] })
      .mockResolvedValueOnce({ results: [] })
      .mockRejectedValue(new Error("terminal failures"));
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({ enabled: true, query: "needle", sessionId: "session-1" }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(750));
    expect(result.current).toEqual({ results: [root], isSearching: true, error: null });
    await act(async () => vi.runAllTimersAsync());

    expect(mockRequest).toHaveBeenCalledTimes(8);
    expect(result.current).toEqual({ results: [root, child], isSearching: false, error: null });
  });

  it("retains the debounce and retry spacing during recovery", async () => {
    mockRequest.mockRejectedValueOnce(new Error("offline")).mockResolvedValue({ results: [] });
    const { result } = renderHook(() =>
      useWorkspaceContentSearch({ enabled: true, query: "needle", sessionId: "session-1" }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(249));
    expect(mockRequest).not.toHaveBeenCalled();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(mockRequest).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(499));
    expect(mockRequest).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(mockRequest).toHaveBeenCalledTimes(2);
    expect(result.current).toEqual({ results: [], isSearching: true, error: null });
    await act(async () => vi.runAllTimersAsync());
    expect(result.current).toEqual({ results: [], isSearching: false, error: null });
  });

  it.each(["query", "sessionId", "inventoryRevision", "enabled"] as const)(
    "ignores an old failure after %s changes",
    async (key) => {
      let rejectOld: (error: Error) => void = () => {};
      mockRequest
        .mockImplementationOnce(
          () =>
            new Promise((_resolve, reject) => {
              rejectOld = reject;
            }),
        )
        .mockResolvedValue({ results: [] });
      const initialProps = {
        enabled: true,
        query: "needle",
        sessionId: "session-1",
        inventoryRevision: "before",
      };
      const { result, rerender } = renderHook(useWorkspaceContentSearch, { initialProps });
      await act(async () => vi.advanceTimersByTimeAsync(250));

      rerender({ ...initialProps, [key]: key === "enabled" ? false : "replacement" });
      await act(async () => vi.runAllTimersAsync());
      expect(result.current).toEqual({ results: [], isSearching: false, error: null });
      await act(async () => rejectOld(new Error("stale disconnect")));
      await act(async () => vi.runAllTimersAsync());

      expect(mockRequest).toHaveBeenCalledTimes(key === "enabled" ? 1 : 9);
      expect(result.current).toEqual({ results: [], isSearching: false, error: null });
    },
  );
});
