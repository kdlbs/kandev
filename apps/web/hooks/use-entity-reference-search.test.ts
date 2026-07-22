import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EntityReferenceSearchGroup } from "@/lib/types/entity-reference";

const searchEntityReferencesMock = vi.fn();
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/api/domains/mentions-api", () => ({
  searchEntityReferences: (...args: unknown[]) => searchEntityReferencesMock(...args),
}));

import { useEntityReferenceSearch } from "./use-entity-reference-search";

const groups: EntityReferenceSearchGroup[] = [
  {
    source: "kandev_tasks",
    provider: "kandev",
    kind: "task",
    display_name: "Kandev tasks",
    kind_label: "Task",
    status: "ok",
    results: [
      {
        version: 1,
        ref: "mention:v1:kandev:task:task-2",
        provider: "kandev",
        kind: "task",
        id: "task-2",
        title: "Repair authentication",
        url: "/t/task-2",
        scope: WORKSPACE_ID,
      },
    ],
  },
  {
    source: "github_issues",
    provider: "github",
    kind: "issue",
    display_name: "GitHub issues",
    kind_label: "Issue",
    status: "timeout",
    results: [],
  },
];

async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

beforeEach(() => {
  vi.useFakeTimers();
  searchEntityReferencesMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useEntityReferenceSearch request lifecycle", () => {
  it("debounces scoped queries for 250 ms and preserves partial provider groups", async () => {
    searchEntityReferencesMock.mockResolvedValueOnce({ query: "auth", groups });

    const { result } = renderHook(() =>
      useEntityReferenceSearch({
        workspaceId: WORKSPACE_ID,
        sessionId: "session-1",
        query: "auth",
        limit: 5,
      }),
    );

    await act(async () => vi.advanceTimersByTimeAsync(249));
    expect(searchEntityReferencesMock).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
      await flush();
    });

    expect(searchEntityReferencesMock).toHaveBeenCalledTimes(1);
    expect(searchEntityReferencesMock).toHaveBeenCalledWith(
      {
        workspaceId: WORKSPACE_ID,
        query: "auth",
        limit: 5,
      },
      { cache: "no-store", init: { signal: expect.any(AbortSignal) } },
    );
    expect(result.current.groups).toEqual(groups);
    expect(result.current.isSearching).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("returns a safe retryable aggregate error and retries the current search", async () => {
    searchEntityReferencesMock.mockRejectedValueOnce(new Error("private upstream details"));
    const { result } = renderHook(() =>
      useEntityReferenceSearch({
        workspaceId: WORKSPACE_ID,
        sessionId: "session-1",
        query: "auth",
      }),
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
      await flush();
    });

    expect(result.current.groups).toEqual([]);
    expect(result.current.error).toEqual({
      message: "Reference search failed. Try again.",
      retryable: true,
    });

    searchEntityReferencesMock.mockResolvedValueOnce({ query: "auth", groups });
    act(() => result.current.retry());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
      await flush();
    });

    expect(searchEntityReferencesMock).toHaveBeenCalledTimes(2);
    expect(result.current.groups).toEqual(groups);
    expect(result.current.error).toBeNull();
  });
});

describe("useEntityReferenceSearch scope guards", () => {
  it("aborts and ignores stale work after workspace, session, or query changes", async () => {
    let resolveStale: (value: {
      query: string;
      groups: EntityReferenceSearchGroup[];
    }) => void = () => {};
    const staleRequest = new Promise<{ query: string; groups: EntityReferenceSearchGroup[] }>(
      (resolve) => {
        resolveStale = resolve;
      },
    );
    const staleGroups = [{ ...groups[0]!, source: "stale_workspace_tasks" }];
    searchEntityReferencesMock
      .mockReturnValueOnce(staleRequest)
      .mockResolvedValueOnce({ query: "oauth", groups });

    const { result, rerender } = renderHook(
      (props: { workspaceId: string; sessionId: string; query: string }) =>
        useEntityReferenceSearch(props),
      {
        initialProps: {
          workspaceId: WORKSPACE_ID,
          sessionId: "session-1",
          query: "auth",
        },
      },
    );
    await act(async () => vi.advanceTimersByTimeAsync(250));
    const staleSignal = searchEntityReferencesMock.mock.calls[0]![1].init.signal as AbortSignal;

    rerender({ workspaceId: "workspace-2", sessionId: "session-2", query: "oauth" });
    expect(staleSignal.aborted).toBe(true);
    expect(result.current.groups).toEqual([]);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
      await flush();
    });
    expect(result.current.groups).toEqual(groups);

    await act(async () => {
      resolveStale({ query: "auth", groups: staleGroups });
      await flush();
    });
    expect(result.current.groups).toEqual(groups);
  });

  it("does not search without an active workspace or non-empty query", async () => {
    searchEntityReferencesMock.mockResolvedValue({ query: "", groups: [] });
    const initialProps: {
      workspaceId: string | null;
      query: string;
      enabled?: boolean;
    } = { workspaceId: null, query: "auth", enabled: true };
    const { rerender } = renderHook(
      (props: { workspaceId: string | null; query: string; enabled?: boolean }) =>
        useEntityReferenceSearch(props),
      { initialProps },
    );

    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(searchEntityReferencesMock).not.toHaveBeenCalled();

    rerender({ workspaceId: WORKSPACE_ID, query: "   ", enabled: true });
    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(searchEntityReferencesMock).not.toHaveBeenCalled();

    rerender({ workspaceId: WORKSPACE_ID, query: "auth", enabled: false });
    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(searchEntityReferencesMock).not.toHaveBeenCalled();
  });

  it("reports searching only after debounce while request remains in flight", async () => {
    searchEntityReferencesMock.mockReturnValueOnce(new Promise(() => {}));
    const { result } = renderHook(() =>
      useEntityReferenceSearch({
        workspaceId: WORKSPACE_ID,
        sessionId: "session-1",
        query: "auth",
      }),
    );

    expect(result.current.isSearching).toBe(false);
    await act(async () => vi.advanceTimersByTimeAsync(249));
    expect(result.current.isSearching).toBe(false);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(result.current.isSearching).toBe(true);
  });
});
