import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { useSidebarTaskPage } from "./use-sidebar-task-page";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: "ws-1" },
    workspaceContextGeneration: 1,
    collapsedSubtaskParents: [] as string[],
    sidebarArchivedTasks: { revisionByWorkspaceId: {} as Record<string, number> },
  },
  view: {
    id: "view-1",
    name: "All",
    filters: [],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsedGroups: [],
  },
  prefs: {
    pinnedTaskIds: [],
    orderedTaskIds: [],
    subtaskOrderByParentId: {},
  },
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => ({ getState: () => mocks.state }),
}));
vi.mock("@/hooks/domains/sidebar/use-effective-sidebar-view", () => ({
  useEffectiveSidebarView: () => mocks.view,
}));
vi.mock("@/hooks/domains/sidebar/use-sidebar-task-prefs", () => ({
  useSidebarTaskPrefs: () => mocks.prefs,
}));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));
vi.mock("react-i18next", () => {
  const translate = (key: string) => key;
  return {
    useTranslation: () => ({
      i18n: { language: "en", resolvedLanguage: "en" },
      t: translate,
    }),
  };
});

function response(page: number, hasPrevious: boolean, hasNext: boolean) {
  return {
    query_key: "query-1",
    page,
    page_size: 100,
    total_entries: 101,
    total_tasks: 101,
    total_visible_tasks: 101,
    has_previous: hasPrevious,
    has_next: hasNext,
    entries: [],
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.state.workspaces.activeId = "ws-1";
  mocks.state.workspaceContextGeneration = 1;
  mocks.state.collapsedSubtaskParents = [];
  mocks.state.sidebarArchivedTasks.revisionByWorkspaceId = {};
});

describe("useSidebarTaskPage", () => {
  it("requests one bounded page and keeps the accepted page when a replacement fails", async () => {
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(response(1, false, true))
      .mockRejectedValueOnce(new Error("network"));
    const { result } = renderHook(() => useSidebarTaskPage("ws-1"));

    await waitFor(() => expect(result.current.response?.page).toBe(1));
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);
    expect(querySidebarTasks).toHaveBeenCalledWith(
      "ws-1",
      expect.objectContaining({ page: 1, page_size: 100, filters: [] }),
      expect.objectContaining({ cache: "no-store" }),
    );

    act(() => result.current.goToPage(2));
    await waitFor(() => expect(result.current.error).toBe("network"));
    expect(result.current.response?.page).toBe(1);
    expect(querySidebarTasks).toHaveBeenLastCalledWith(
      "ws-1",
      expect.objectContaining({ page: 2, page_size: 100 }),
      expect.objectContaining({ cache: "no-store" }),
    );
  });

  it("retries the accepted page after a failed replacement", async () => {
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(response(1, false, true))
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(response(1, false, true));
    const { result } = renderHook(() => useSidebarTaskPage("ws-1"));

    await waitFor(() => expect(result.current.response?.page).toBe(1));
    act(() => result.current.goToPage(2));
    await waitFor(() => expect(result.current.error).toBe("offline"));
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.error).toBeNull());
    expect(result.current.response?.page).toBe(1);
    expect(querySidebarTasks).toHaveBeenLastCalledWith(
      "ws-1",
      expect.objectContaining({ page: 1 }),
      expect.objectContaining({ cache: "no-store" }),
    );
  });
});

describe("useSidebarTaskPage request lifecycle", () => {
  it("restarts the initial request when StrictMode replays its effects", async () => {
    const requests: Array<{
      signal: AbortSignal | undefined;
      deferred: ReturnType<typeof deferred<SidebarTaskPageResponse>>;
    }> = [];
    vi.mocked(querySidebarTasks).mockImplementation((_workspaceId, _query, options) => {
      const request = deferred<SidebarTaskPageResponse>();
      requests.push({ signal: options?.init?.signal ?? undefined, deferred: request });
      return request.promise;
    });
    const wrapper = ({ children }: { children: ReactNode }) => <StrictMode>{children}</StrictMode>;
    const { result } = renderHook(() => useSidebarTaskPage("ws-1"), {
      wrapper,
      reactStrictMode: true,
    });

    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[0]?.signal?.aborted).toBe(true);
    await act(async () => {
      requests[0]?.deferred.resolve(response(1, false, true));
      requests[1]?.deferred.resolve(response(1, false, true));
    });
    await waitFor(() => expect(result.current.response?.page).toBe(1));
    await waitFor(() => expect(result.current.requestedPage).toBeNull());
  });

  it("keeps a requested page alive and refreshes that page after an invalidation", async () => {
    const workspaceId = "ws-navigation";
    mocks.state.workspaces.activeId = workspaceId;
    const requests: Array<{
      query: SidebarTaskQuery;
      signal: AbortSignal | undefined;
      deferred: ReturnType<typeof deferred<SidebarTaskPageResponse>>;
    }> = [];
    vi.mocked(querySidebarTasks).mockImplementation((_workspaceId, query, options) => {
      const request = deferred<SidebarTaskPageResponse>();
      requests.push({ query, signal: options?.init?.signal ?? undefined, deferred: request });
      return request.promise;
    });
    const { result, rerender } = renderHook(() => useSidebarTaskPage(workspaceId));
    await waitFor(() => expect(requests).toHaveLength(1));
    await act(async () => requests[0]?.deferred.resolve(response(1, false, true)));
    await waitFor(() => expect(result.current.response?.page).toBe(1));

    vi.useFakeTimers();
    vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 0;
    });
    const afterNavigation = vi.fn();
    act(() => result.current.goToPage(2, afterNavigation));
    expect(requests).toHaveLength(2);
    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId[workspaceId] = 1;
      rerender();
    });
    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(requests).toHaveLength(2);
    expect(requests[1]?.query.page).toBe(2);
    expect(requests[1]?.signal?.aborted).toBe(false);

    await act(async () => requests[1]?.deferred.resolve(response(2, true, false)));
    expect(requests).toHaveLength(3);
    expect(requests[2]?.query.page).toBe(2);
    expect(afterNavigation).toHaveBeenCalledTimes(1);
    await act(async () => requests[2]?.deferred.resolve(response(2, true, false)));
    expect(result.current.requestedPage).toBeNull();
    expect(result.current.response?.page).toBe(2);
    vi.useRealTimers();
  });
});

describe("useSidebarTaskPage refresh invalidation", () => {
  it("coalesces task invalidations and refreshes once after the trailing window", async () => {
    vi.mocked(querySidebarTasks).mockResolvedValue(response(1, false, true));
    const { rerender } = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
    vi.useFakeTimers();

    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 1;
      rerender();
    });
    act(() => vi.advanceTimersByTime(200));
    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 2;
      rerender();
    });
    act(() => vi.advanceTimersByTime(249));
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(querySidebarTasks).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });

  it("bounds continuous invalidations to one refresh within two seconds", async () => {
    vi.mocked(querySidebarTasks).mockResolvedValue(response(1, false, true));
    const { rerender } = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
    vi.useFakeTimers();

    for (let revision = 1; revision <= 9; revision += 1) {
      act(() => {
        mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = revision;
        rerender();
      });
      act(() => vi.advanceTimersByTime(200));
    }
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);
    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 10;
      rerender();
    });
    act(() => vi.advanceTimersByTime(199));
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(querySidebarTasks).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });
});
