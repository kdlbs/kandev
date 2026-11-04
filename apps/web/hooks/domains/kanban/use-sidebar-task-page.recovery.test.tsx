import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse } from "@/lib/types/http";
import { useSidebarTaskPage } from "./use-sidebar-task-page";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";

const QUERY_REFRESH_FAILED: string = "sidebar:queryRefreshFailed";
const PAGE_LOAD_FAILED: string = "sidebar:pageLoadFailed";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: "ws-1" },
    workspaceContextGeneration: 1,
    collapsedSubtaskParents: [] as string[],
    sidebarArchivedTasks: { revisionByWorkspaceId: {} as Record<string, number> },
    userSettings: {
      sidebarTaskColors: {} as Record<string, string | null>,
      sidebarTaskColorAutomation: { enabled: false, rules: [] },
    },
    repositories: { itemsByWorkspaceId: {} as Record<string, never[]> },
    kanbanMulti: { snapshots: {} as Record<string, never> },
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

const busy = () => new ApiError("busy", 503, { code: "persistence_unavailable" });

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

describe("useSidebarTaskPage quiet recovery", () => {
  function provisionalResponse() {
    return { ...response(1, false, false), provisional: true };
  }

  it("stops the provisional refresh loop and stays quiet until the cycle is exhausted", async () => {
    vi.useFakeTimers();
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(provisionalResponse())
      .mockRejectedValue(busy());
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(hook.result.current.response?.provisional).toBe(true);

    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(querySidebarTasks).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTimeAsync(2_000));
    expect(querySidebarTasks).toHaveBeenCalledTimes(3);
    expect(hook.result.current.error).toBeNull();
    expect(hook.result.current.requestedPage).toBe(1);

    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(hook.result.current.error).toBe(QUERY_REFRESH_FAILED);
    expect(hook.result.current.response?.provisional).toBe(true);
    expect(querySidebarTasks).toHaveBeenCalledTimes(6);

    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(querySidebarTasks).toHaveBeenCalledTimes(6);
    expect(hook.result.current.error).toBe(QUERY_REFRESH_FAILED);
    vi.useRealTimers();
  });

  it("keeps the warning visible and Retry pending through explicit recovery", async () => {
    vi.useFakeTimers();
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(response(1, false, false))
      .mockRejectedValue(busy());
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(hook.result.current.response).not.toBeNull();

    act(() => hook.result.current.refresh());
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(hook.result.current.error).toBe(QUERY_REFRESH_FAILED);
    expect(querySidebarTasks).toHaveBeenCalledTimes(6);

    const retry = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockReturnValueOnce(retry.promise);
    act(() => hook.result.current.retry());
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(hook.result.current.error).toBe(QUERY_REFRESH_FAILED);
    expect(hook.result.current.requestedPage).toBe(1);

    await act(async () => retry.resolve(response(1, false, false)));
    expect(hook.result.current.error).toBeNull();
    expect(hook.result.current.requestedPage).toBeNull();
    expect(querySidebarTasks).toHaveBeenCalledTimes(7);
    vi.useRealTimers();
  });

  it("shares one bounded cycle between desktop and phone consumers", async () => {
    vi.useFakeTimers();
    vi.mocked(querySidebarTasks).mockRejectedValue(busy());
    const desktop = renderHook(() => useSidebarTaskPage("ws-1"));
    const phone = renderHook(() => useSidebarTaskPage("ws-1"));
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(querySidebarTasks).toHaveBeenCalledTimes(5);
    expect(desktop.result.current.error).toBe(PAGE_LOAD_FAILED);
    expect(phone.result.current.error).toBe(PAGE_LOAD_FAILED);
    vi.useRealTimers();
  });
});

describe("useSidebarTaskPage recovery admission", () => {
  it("does not restart an exhausted cycle from a trigger that arrived during recovery", async () => {
    vi.useFakeTimers();
    vi.mocked(querySidebarTasks).mockRejectedValue(busy());
    const desktop = renderHook(() => useSidebarTaskPage("ws-1"));
    const phone = renderHook(() => useSidebarTaskPage("ws-1"));
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);

    // The desktop consumer renders first; its callback stays valid across renders.
    const runForeground = () =>
      (vi.mocked(useForegroundRefresh).mock.calls[0]?.[0] as () => void)();
    act(() => runForeground());
    await act(async () => vi.advanceTimersByTimeAsync(2_000));
    expect(querySidebarTasks).toHaveBeenCalledTimes(2);

    act(() => {
      mocks.state.sidebarArchivedTasks.revisionByWorkspaceId["ws-1"] = 1;
      desktop.rerender();
      phone.rerender();
    });
    await act(async () => vi.advanceTimersByTimeAsync(250));
    expect(querySidebarTasks).toHaveBeenCalledTimes(2);

    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(querySidebarTasks).toHaveBeenCalledTimes(5);
    expect(desktop.result.current.error).toBe(PAGE_LOAD_FAILED);
    expect(phone.result.current.error).toBe(PAGE_LOAD_FAILED);
    expect(desktop.result.current.canRetry).toBe(true);
    expect(desktop.result.current.requestedPage).toBeNull();

    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(querySidebarTasks).toHaveBeenCalledTimes(5);
    expect(desktop.result.current.error).toBe(PAGE_LOAD_FAILED);
    expect(desktop.result.current.requestedPage).toBeNull();

    vi.mocked(querySidebarTasks).mockResolvedValueOnce(response(1, false, false));
    act(() => runForeground());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(querySidebarTasks).toHaveBeenCalledTimes(6);
    expect(desktop.result.current.error).toBeNull();
    vi.useRealTimers();
  });

  it("starts one new cycle on a foreground episode after exhaustion", async () => {
    vi.useFakeTimers();
    vi.mocked(querySidebarTasks).mockRejectedValue(busy());
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await act(async () => vi.advanceTimersByTimeAsync(0));
    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(hook.result.current.error).toBe(PAGE_LOAD_FAILED);
    expect(querySidebarTasks).toHaveBeenCalledTimes(5);

    vi.mocked(querySidebarTasks).mockResolvedValueOnce(response(1, false, true));
    const foreground = vi.mocked(useForegroundRefresh).mock.calls.at(-1)?.[0] as () => void;
    act(() => foreground());
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(querySidebarTasks).toHaveBeenCalledTimes(6);
    expect(hook.result.current.error).toBeNull();
    vi.useRealTimers();
  });
});
