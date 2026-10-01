import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { useSidebarTaskPage } from "./use-sidebar-task-page";
import { sidebarTaskPageCache } from "@/lib/sidebar/sidebar-task-page-cache";
import type { AppState } from "@/lib/state/store";

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

let store = { getState: () => mocks.state };

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => store,
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
  vi.resetAllMocks();
  store = { getState: () => mocks.state };
  mocks.state.workspaces.activeId = "ws-1";
  mocks.state.workspaceContextGeneration = 1;
  mocks.state.collapsedSubtaskParents = [];
  mocks.state.sidebarArchivedTasks.revisionByWorkspaceId = {};
});

describe("useSidebarTaskPage", () => {
  it("releases pending reads and drops the duplicate page when shared state takes over", async () => {
    const pending = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockResolvedValueOnce(response(1, false, false));
    const hook = renderHook(({ enabled }) => useSidebarTaskPage("ws-1", enabled), {
      initialProps: { enabled: true },
    });
    await waitFor(() => expect(hook.result.current.response).not.toBeNull());
    const key = hook.result.current.scopeKey;
    const cache = sidebarTaskPageCache(store as unknown as { getState: () => AppState });
    expect(cache.get(key)).not.toBeNull();
    vi.mocked(querySidebarTasks).mockReturnValueOnce(pending.promise);
    act(() => hook.result.current.refresh());
    await waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(2));
    const signal = vi.mocked(querySidebarTasks).mock.calls[1][2]?.init?.signal;
    hook.rerender({ enabled: false });
    expect(signal?.aborted).toBe(true);
    expect(hook.result.current.response).toBeNull();
    expect(cache.get(key)).toBeNull();
    expect(hook.result.current.isLoading).toBe(false);
    expect(hook.result.current.requestedPage).toBeNull();
    await act(async () => pending.resolve(response(1, false, false)));
    expect(cache.get(key)).toBeNull();
    expect(hook.result.current.response).toBeNull();
    act(() => hook.result.current.retry());
    expect(querySidebarTasks).toHaveBeenCalledTimes(2);
  });

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
    await waitFor(() => expect(result.current.error).toBe("sidebar:queryRefreshFailed"));
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
    await waitFor(() => expect(result.current.error).toBe("sidebar:queryRefreshFailed"));
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
    expect(requests).toHaveLength(2);
    expect(result.current.response?.page).toBe(2);
    expect(result.current.response?.provisional).toBe(true);
    expect(afterNavigation).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(requests).toHaveLength(3);
    expect(requests[2]?.query.page).toBe(2);
    await act(async () => requests[2]?.deferred.resolve(response(2, true, false)));
    expect(result.current.requestedPage).toBeNull();
    expect(result.current.response?.page).toBe(2);
    expect(afterNavigation).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });
});

describe("useSidebarTaskPage refresh invalidation", () => {
  it("shares one trailing refresh with updates arriving just after a slow first response", async () => {
    const requests: Array<ReturnType<typeof deferred<SidebarTaskPageResponse>>> = [];
    vi.mocked(querySidebarTasks).mockImplementation(() => {
      const request = deferred<SidebarTaskPageResponse>();
      requests.push(request);
      return request.promise;
    });
    const { result, rerender } = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(requests).toHaveLength(1));
    vi.useFakeTimers();
    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 1;
      rerender();
    });
    await act(async () => vi.advanceTimersByTimeAsync(250));
    await act(async () => requests[0]?.resolve(response(1, false, true)));
    expect(result.current.response?.provisional).toBe(true);
    expect(requests).toHaveLength(1);
    await act(async () => vi.advanceTimersByTimeAsync(100));
    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 2;
      rerender();
    });
    await act(async () => vi.advanceTimersByTimeAsync(150));
    expect(requests).toHaveLength(2);
    await act(async () => requests[1]?.resolve(response(1, false, true)));
    await act(async () => vi.advanceTimersByTimeAsync(1000));
    expect(requests).toHaveLength(2);
    expect(result.current.requestedPage).toBeNull();
    expect(result.current.response?.provisional).toBe(false);
  });

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

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15
it("shows cached first page before background refresh resolves", async () => {
  const original = { ...mocks.view };
  mocks.state.workspaces.activeId = "ws-reuse";
  const refresh = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks)
    .mockResolvedValueOnce({ ...response(1, false, false), query_key: "view-a" })
    .mockResolvedValueOnce({ ...response(1, false, false), query_key: "view-b" })
    .mockReturnValueOnce(refresh.promise);
  const { result, rerender } = renderHook(() => useSidebarTaskPage("ws-reuse"));
  await waitFor(() => expect(result.current.response?.query_key).toBe("view-a"));
  mocks.view = { ...original, group: "repository" };
  rerender();
  await waitFor(() => expect(result.current.response?.query_key).toBe("view-b"));
  mocks.view = original;
  rerender();
  expect(result.current.response?.query_key).toBe("view-a");
  expect(result.current.isLoading).toBe(false);
  await act(async () =>
    refresh.resolve({ ...response(1, false, false), query_key: "view-a-fresh" }),
  );
  await waitFor(() => expect(result.current.response?.query_key).toBe("view-a-fresh"));
});

it("localizes rejected filters without exposing raw server text", async () => {
  mocks.state.workspaces.activeId = "ws-invalid";
  vi.mocked(querySidebarTasks).mockRejectedValueOnce(new Error("private server detail"));
  const { result } = renderHook(() => useSidebarTaskPage("ws-invalid"));
  await waitFor(() => expect(result.current.error).toBe("sidebar:pageLoadFailed"));
});

// Contract coverage for shared request ownership and context fencing.
it("settles a shared read after its initiating consumer unmounts", async () => {
  const pending = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks).mockReturnValueOnce(pending.promise);
  const first = renderHook(() => useSidebarTaskPage("ws-1"));
  const second = renderHook(() => useSidebarTaskPage("ws-1"));
  expect(querySidebarTasks).toHaveBeenCalledTimes(1);
  first.unmount();
  expect(vi.mocked(querySidebarTasks).mock.calls[0][2]?.init?.signal?.aborted).toBe(false);
  await act(async () => pending.resolve(response(1, false, false)));
  expect(second.result.current.response?.page).toBe(1);
  expect(second.result.current.requestedPage).toBeNull();
});

it("ignores a late old-workspace response after the replacement settles", async () => {
  const old = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks)
    .mockReturnValueOnce(old.promise)
    .mockResolvedValueOnce({ ...response(1, false, false), query_key: "new-workspace" });
  const hook = renderHook(({ id }) => useSidebarTaskPage(id), { initialProps: { id: "ws-1" } });
  mocks.state.workspaces.activeId = "ws-2";
  mocks.state.workspaceContextGeneration = 2;
  hook.rerender({ id: "ws-2" });
  await waitFor(() => expect(hook.result.current.response?.query_key).toBe("new-workspace"));
  await act(async () => old.resolve({ ...response(1, false, false), query_key: "old-workspace" }));
  expect(hook.result.current.response?.query_key).toBe("new-workspace");
  expect(hook.result.current.requestedPage).toBeNull();
});

it.each([401, 403, 404])("removes cached and displayed rows on HTTP %s", async (status) => {
  vi.mocked(querySidebarTasks)
    .mockResolvedValueOnce(response(1, false, false))
    .mockRejectedValueOnce(new ApiError("private", status, {}));
  const hook = renderHook(() => useSidebarTaskPage("ws-1"));
  await waitFor(() => expect(hook.result.current.response).not.toBeNull());
  act(() => hook.result.current.refresh());
  await waitFor(() =>
    expect(hook.result.current.error).toBe("sidebar:workspaceContextAccessDenied"),
  );
  expect(hook.result.current.response).toBeNull();
  expect(hook.result.current.canRetry).toBe(false);
  const pending = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks).mockReturnValueOnce(pending.promise);
  const replacement = renderHook(() => useSidebarTaskPage("ws-1"));
  expect(replacement.result.current.response).toBeNull();
  await act(async () => pending.resolve(response(1, false, false)));
});

it.each([false, true])(
  "clears sibling rows and late completions on denial (pending=%s)",
  async (pendingSibling) => {
    vi.mocked(querySidebarTasks).mockResolvedValueOnce(response(1, false, true));
    const first = renderHook(() => useSidebarTaskPage("ws-1"));
    const sibling = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(sibling.result.current.response).not.toBeNull());
    const late = deferred<SidebarTaskPageResponse>();
    if (pendingSibling) {
      vi.mocked(querySidebarTasks).mockReturnValueOnce(late.promise);
      act(() => sibling.result.current.goToPage(2));
    }
    vi.mocked(querySidebarTasks).mockRejectedValueOnce(new ApiError("private", 403, {}));
    act(() => first.result.current.refresh());
    await waitFor(() => expect(first.result.current.response).toBeNull());
    expect(sibling.result.current.response).toBeNull();
    expect(sibling.result.current.error).toBe("sidebar:workspaceContextAccessDenied");
    expect(sibling.result.current.requestedPage).toBeNull();
    await act(async () => late.resolve(response(2, true, false)));
    expect(sibling.result.current.response).toBeNull();
    expect(sibling.result.current.requestedPage).toBeNull();
  },
);

it("displays a provisional response while its trailing refresh reconciles soft invalidations", async () => {
  const stale = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks)
    .mockResolvedValueOnce(response(1, false, false))
    .mockReturnValueOnce(stale.promise);
  const hook = renderHook(() => useSidebarTaskPage("ws-1"));
  await waitFor(() => expect(hook.result.current.response?.query_key).toBe("query-1"));
  vi.useFakeTimers();
  act(() => hook.result.current.refresh());
  mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 1;
  hook.rerender();
  await act(async () => stale.resolve({ ...response(1, false, false), query_key: "deleted-row" }));
  expect(hook.result.current.response?.query_key).toBe("deleted-row");
  expect(hook.result.current.response?.provisional).toBe(true);
  vi.mocked(querySidebarTasks).mockResolvedValueOnce({
    ...response(1, false, false),
    query_key: "fresh",
  });
  await act(async () => vi.advanceTimersByTimeAsync(250));
  expect(hook.result.current.response?.query_key).toBe("fresh");
});

it("offers Retry for a transient failure after correcting an invalid filter", async () => {
  vi.mocked(querySidebarTasks)
    .mockRejectedValueOnce(
      new ApiError("private", 400, {
        error_code: "sidebar_query_invalid",
        details: { reason: "invalid_clause", filter_index: 0 },
      }),
    )
    .mockResolvedValueOnce(response(1, false, false))
    .mockRejectedValueOnce(new ApiError("private", 503, {}));
  const hook = renderHook(() => useSidebarTaskPage("ws-1"));
  await waitFor(() => expect(hook.result.current.canRetry).toBe(false));
  act(() => hook.result.current.refresh());
  await waitFor(() => expect(hook.result.current.response).not.toBeNull());
  act(() => hook.result.current.refresh());
  await waitFor(() => expect(hook.result.current.error).toBe("sidebar:queryRefreshFailed"));
  expect(hook.result.current.canRetry).toBe(true);
});
