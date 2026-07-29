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

    await act(async () => vi.advanceTimersByTimeAsync(1));

    expect(mockSearchWorkspaceContent).toHaveBeenCalledWith(mockClient, "session-1", "needle", 50);
    expect(result.current.results).toEqual(results);
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
    mockSearchWorkspaceContent
      .mockResolvedValueOnce({ results: previousResults })
      .mockResolvedValueOnce({ results: [] });
    const { result, rerender } = renderHook(
      ({ query }) => useWorkspaceContentSearch({ enabled: true, query, sessionId: "session-1" }),
      { initialProps: { query: "previous" } },
    );
    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(result.current.results).toEqual(previousResults);

    rerender({ query: "next" });

    expect(result.current.results).toEqual([]);
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

    await act(async () => vi.advanceTimersByTimeAsync(250));

    expect(result.current.results).toEqual([]);
    expect(result.current.error).toBe("transport-error");
    expect(result.current.isSearching).toBe(false);
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

    expect(mockSearchWorkspaceContent).toHaveBeenCalledTimes(2);
    expect(result.current.results).toEqual(secondResults);
    expect(result.current.isSearching).toBe(false);
  });
});
