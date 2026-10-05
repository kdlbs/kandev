import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { useSidebarTaskPage } from "./use-sidebar-task-page";
import type { AppState } from "@/lib/state/store";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: "ws-1" },
    workspaceContextGeneration: 1,
    repositories: { itemsByWorkspaceId: {} },
    workflows: { items: [] },
    kanbanMulti: { snapshots: {} },
    collapsedSubtaskParents: [] as string[],
    language: "en",
    auth: undefined as AppState["auth"] | undefined,
    workspaceContextRead: undefined as
      | Pick<AppState["workspaceContextRead"], "snapshotError" | "errors">
      | undefined,
    sidebarArchivedTasks: { revisionByWorkspaceId: {} as Record<string, number> },
  },
  view: {
    id: "view-1",
    name: "All",
    filters: [] as SidebarTaskQuery["filters"],
    sort: { key: "updatedAt", direction: "desc" },
    group: "none",
    collapsedGroups: [] as string[],
  },
  prefs: {
    pinnedTaskIds: [] as string[],
    orderedTaskIds: [] as string[],
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
      i18n: { language: mocks.state.language, resolvedLanguage: mocks.state.language },
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
  mocks.state.language = "en";
  mocks.state.auth = undefined;
  mocks.state.workspaceContextRead = undefined;
  mocks.state.sidebarArchivedTasks.revisionByWorkspaceId = {};
});

const refreshFailure = "sidebar:queryRefreshFailed";

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.18, AC-UI-SIDEBAR-ARCHIVED-FILTER-002.25
it.each([
  ["snapshot", false, false],
  ["snapshot", true, false],
  ["collection", false, false],
  ["collection", true, false],
  ["snapshot", true, true],
  ["collection", true, true],
] as const)(
  "hides rows on the first %s denial render (disclosure=%s, local=%s)",
  async (source, disclosure, local) => {
    const original = mocks.view;
    const pending = deferred<SidebarTaskPageResponse>();
    const accepted = {
      ...response(1, false, true),
      entries: [{ kind: "task" as const, task_id: "task-a" }],
    };
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(accepted)
      .mockReturnValueOnce(pending.promise);
    const rendered: ReturnType<typeof useSidebarTaskPage>[] = [];
    try {
      const hook = renderHook(() => {
        const page = useSidebarTaskPage("ws-1", !local, local ? [] : null);
        rendered.push(page);
        return page;
      });
      await waitFor(() =>
        expect(hook.result.current.response?.entries).toEqual(local ? [] : accepted.entries),
      );
      if (disclosure) {
        mocks.view = { ...original, collapsedGroups: ["repo-a"] };
        hook.rerender();
        expect(hook.result.current.isDisclosureTransition).toBe(!local);
      }
      mocks.state.workspaceContextRead = {
        snapshotError: source === "snapshot" ? "access_denied" : null,
        errors: {
          workflows: source === "collection" ? "access_denied" : null,
          repositories: null,
          steps: null,
        },
      };
      rendered.length = 0;
      hook.rerender();
      expect(rendered.length).toBeGreaterThan(0);
      for (const page of rendered) {
        expect(page.response).toBeNull();
        expect(page.isDisclosureTransition).toBe(false);
        expect(page.canRetry).toBe(false);
      }
      await act(async () => pending.resolve(accepted));
      expect(hook.result.current.response).toBeNull();
      if (local) expect(querySidebarTasks).not.toHaveBeenCalled();
    } finally {
      mocks.view = original;
    }
  },
);

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.25
it("keeps eligible rows while an uncached repository collapse loads", async () => {
  const original = mocks.view;
  mocks.view = { ...original, group: "repository", collapsedGroups: [] };
  const pending = deferred<SidebarTaskPageResponse>();
  const accepted: SidebarTaskPageResponse = {
    ...response(1, false, false),
    total_tasks: 2,
    total_visible_tasks: 2,
    entries: [
      { kind: "group", group_key: "repo-a", group_label: "Repo A", matching_count: 1 },
      { kind: "task", task_id: "task-a", group_key: "repo-a" },
      { kind: "group", group_key: "repo-b", group_label: "Repo B", matching_count: 1 },
      { kind: "task", task_id: "task-b", group_key: "repo-b" },
    ],
  };
  vi.mocked(querySidebarTasks).mockResolvedValueOnce(accepted).mockReturnValueOnce(pending.promise);
  try {
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(hook.result.current.response?.entries).toHaveLength(4));
    mocks.view = { ...mocks.view, collapsedGroups: ["repo-a"] };
    hook.rerender();
    expect(hook.result.current.response?.entries).toEqual(accepted.entries);
    expect(hook.result.current.isLoading).toBe(false);
    expect(hook.result.current.isRefreshing).toBe(true);
    expect(querySidebarTasks).toHaveBeenLastCalledWith(
      "ws-1",
      expect.objectContaining({ collapsed_group_keys: ["repo-a"], page: 1 }),
      expect.anything(),
    );
    await act(async () =>
      pending.resolve({
        ...accepted,
        total_visible_tasks: 1,
        entries: accepted.entries.filter((entry) => entry.task_id !== "task-a"),
      }),
    );
    expect(hook.result.current.response?.entries).toHaveLength(3);
    expect(hook.result.current.isLoading).toBe(false);
    expect(hook.result.current.isRefreshing).toBe(false);
  } finally {
    mocks.view = original;
  }
});

it("keeps all-collapsed headings visible when their accepted page needs refresh", async () => {
  const original = mocks.view;
  mocks.view = { ...original, group: "repository", collapsedGroups: ["repo-a", "repo-b"] };
  const pending = deferred<SidebarTaskPageResponse>();
  const headers: SidebarTaskPageResponse = {
    ...response(1, false, false),
    provisional: true,
    total_tasks: 2,
    total_visible_tasks: 0,
    entries: [
      { kind: "group", group_key: "repo-a", group_label: "Repo A", matching_count: 1 },
      { kind: "group", group_key: "repo-b", group_label: "Repo B", matching_count: 1 },
    ],
  };
  vi.mocked(querySidebarTasks).mockResolvedValueOnce(headers).mockReturnValue(pending.promise);
  try {
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(hook.result.current.response?.entries).toHaveLength(2));
    expect(hook.result.current.isLoading).toBe(false);
    await act(async () => pending.resolve({ ...headers, provisional: false }));
  } finally {
    mocks.view = original;
  }
});

it.each(["expansion", "subtasks", "provisional expansion"])(
  "retains a bounded display during %s without inventing rows",
  async (change) => {
    const original = mocks.view;
    mocks.view = { ...original, group: "repository", collapsedGroups: ["repo-a"] };
    const page: SidebarTaskPageResponse = {
      ...response(1, false, false),
      entries: [
        { kind: "group", group_key: "repo-a", group_label: "Repo A", matching_count: 1 },
        { kind: "task", task_id: "task-b", group_key: "repo-b" },
      ],
    };
    if (change === "provisional expansion") {
      page.provisional = true;
      page.entries = page.entries.filter((entry) => entry.kind === "group");
    }
    const pending = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockResolvedValueOnce(page).mockReturnValueOnce(pending.promise);
    try {
      const hook = renderHook(() => useSidebarTaskPage("ws-1"));
      await waitFor(() => expect(hook.result.current.response).not.toBeNull());
      if (change !== "subtasks") mocks.view = { ...mocks.view, collapsedGroups: [] };
      else mocks.state.collapsedSubtaskParents = ["task-b"];
      hook.rerender();
      expect(hook.result.current.response?.entries).toEqual(page.entries);
      expect(hook.result.current.isDisclosureTransition).toBe(true);
      expect(hook.result.current.isLoading).toBe(false);
      await act(async () => pending.resolve({ ...page, provisional: false }));
      expect(hook.result.current.isDisclosureTransition).toBe(false);
    } finally {
      mocks.view = original;
    }
  },
);

it("retries page one after a failed collapse from a later page and blocks stale navigation", async () => {
  const original = mocks.view;
  const replacement = deferred<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks)
    .mockResolvedValueOnce(response(1, false, true))
    .mockResolvedValueOnce(response(2, true, false))
    .mockRejectedValueOnce(new Error("offline"))
    .mockReturnValueOnce(replacement.promise);
  try {
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(hook.result.current.response?.page).toBe(1));
    act(() => hook.result.current.goToPage(2));
    await waitFor(() => expect(hook.result.current.response?.page).toBe(2));
    mocks.view = { ...original, collapsedGroups: ["repo-a"] };
    hook.rerender();
    await waitFor(() => expect(hook.result.current.error).toBe(refreshFailure));
    expect(hook.result.current.response?.page).toBe(2);
    expect(hook.result.current.isDisclosureTransition).toBe(true);
    expect(hook.result.current.isLoading).toBe(false);
    act(() => hook.result.current.goToPage(1));
    expect(querySidebarTasks).toHaveBeenCalledTimes(3);
    act(() => hook.result.current.retry());
    expect(querySidebarTasks).toHaveBeenLastCalledWith(
      "ws-1",
      expect.objectContaining({ page: 1 }),
      expect.anything(),
    );
    await act(async () => replacement.resolve(response(1, false, false)));
    expect(hook.result.current.response?.page).toBe(1);
    expect(hook.result.current.isDisclosureTransition).toBe(false);
  } finally {
    mocks.view = original;
  }
});

it("ignores a superseded disclosure response while retaining the accepted display", async () => {
  const original = mocks.view;
  const first = deferred<SidebarTaskPageResponse>();
  const latest = deferred<SidebarTaskPageResponse>();
  const accepted = { ...response(1, false, false), query_key: "accepted" };
  vi.mocked(querySidebarTasks)
    .mockResolvedValueOnce(accepted)
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(latest.promise);
  try {
    const hook = renderHook(() => useSidebarTaskPage("ws-1"));
    await waitFor(() => expect(hook.result.current.response?.query_key).toBe("accepted"));
    mocks.view = { ...original, collapsedGroups: ["repo-a"] };
    hook.rerender();
    mocks.view = { ...original, collapsedGroups: ["repo-b"] };
    hook.rerender();
    await act(async () => first.resolve({ ...accepted, query_key: "obsolete" }));
    expect(hook.result.current.response?.query_key).toBe("accepted");
    expect(hook.result.current.requestedPage).toBe(1);
    await act(async () => latest.resolve({ ...accepted, query_key: "latest" }));
    expect(hook.result.current.response?.query_key).toBe("latest");
    expect(hook.result.current.requestedPage).toBeNull();
  } finally {
    mocks.view = original;
  }
});

it.each(["filter", "sort", "group", "pins", "order", "workspace", "locale", "account"])(
  "does not retain disclosure rows across a changed %s",
  async (change) => {
    const original = mocks.view;
    const prefs = { ...mocks.prefs };
    const pending = deferred<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks)
      .mockResolvedValueOnce(response(1, false, false))
      .mockReturnValueOnce(pending.promise);
    try {
      const hook = renderHook(() => useSidebarTaskPage("ws-1"));
      await waitFor(() => expect(hook.result.current.response).not.toBeNull());
      mocks.view = { ...original, collapsedGroups: ["repo-a"] };
      if (change === "filter")
        mocks.view.filters = [{ dimension: "archived", op: "is", value: true }];
      if (change === "sort") mocks.view.sort = { key: "title", direction: "asc" };
      if (change === "group") mocks.view.group = "workflow";
      if (change === "pins") mocks.prefs.pinnedTaskIds = ["task-a"];
      if (change === "order") mocks.prefs.orderedTaskIds = ["task-a"];
      if (change === "workspace") mocks.state.workspaceContextGeneration++;
      if (change === "locale") mocks.state.language = "pt-pt";
      if (change === "account")
        mocks.state.auth = {
          mode: "enabled",
          authenticated: true,
          user: {
            id: "other-account",
            email: "other@example.test",
            display_name: "Other",
            role: "admin",
            status: "active",
          },
        };
      hook.rerender();
      expect(hook.result.current.response).toBeNull();
      expect(hook.result.current.isDisclosureTransition).toBe(false);
      expect(hook.result.current.isLoading).toBe(true);
      await act(async () => pending.resolve(response(1, false, false)));
    } finally {
      mocks.view = original;
      mocks.prefs = prefs;
    }
  },
);
