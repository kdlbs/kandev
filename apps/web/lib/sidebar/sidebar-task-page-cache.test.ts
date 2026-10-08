import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { AppState } from "@/lib/state/store";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import { toApiSidebarDraft, toApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import {
  SidebarTaskPageCache,
  sidebarTaskPageRankingKey,
  sidebarTaskPageScope,
} from "./sidebar-task-page-cache";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/lib/api/domains/settings-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/settings-api")>();
  return { ...actual, updateUserSettings: vi.fn() };
});
const query: SidebarTaskQuery = {
  filters: [],
  collapsed_group_keys: [],
  collapsed_task_ids: [],
  sort: { key: "state", direction: "asc" },
  group: "none",
  page: 1,
  page_size: 100,
  locale: "en",
};
const page = (key = "a"): SidebarTaskPageResponse => ({
  query_key: key,
  page: 1,
  page_size: 100,
  total_entries: 0,
  total_tasks: 0,
  total_visible_tasks: 0,
  has_previous: false,
  has_next: false,
  entries: [],
});
function setup() {
  const store = createAppStore();
  store.setState({ workspaces: { items: [], activeId: "ws" }, workspaceContextGeneration: 1 });
  return { store, cache: new SidebarTaskPageCache(store) };
}
async function load(cache: SidebarTaskPageCache, key: string, result = page(key)) {
  vi.mocked(querySidebarTasks).mockResolvedValueOnce(result);
  const request = cache.request("ws", query, key);
  await request.promise;
  request.release();
}
function settingsMessage(
  payload: Partial<BackendMessageMap["user.settings.updated"]["payload"]>,
): BackendMessageMap["user.settings.updated"] {
  return {
    type: "notification",
    action: "user.settings.updated",
    payload: { user_id: "user", workspace_id: "ws", repository_ids: [], ...payload },
  };
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(updateUserSettings).mockResolvedValue({ settings: { revision: 1 } } as Awaited<
    ReturnType<typeof updateUserSettings>
  >);
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.2
it("evicts the least recently used first page without retaining later pages", async () => {
  const { cache } = setup();
  for (let n = 0; n < 5; n++) await load(cache, String(n));
  await load(cache, "0");
  await load(cache, "5");
  expect(cache.get("0")).not.toBeNull();
  expect(cache.get("1")).toBeNull();
  for (const key of ["2", "3", "4", "5"]) expect(cache.get(key)).not.toBeNull();
});

it("expires from fetch time and bounds response bytes", async () => {
  const { cache } = setup();
  await load(cache, "a");
  vi.advanceTimersByTime(299999);
  expect(cache.get("a")).not.toBeNull();
  vi.advanceTimersByTime(1);
  expect(cache.get("a")).toBeNull();
  await load(cache, "large", page("x".repeat(2 * 1024 * 1024)));
  expect(cache.get("large")).toBeNull();
  await load(cache, "second", { ...page(), page: 2 });
  expect(cache.get("second")).toBeNull();
});

it("fences invalidation during a read and clears across workspace generations", async () => {
  const { store, cache } = setup();
  await load(cache, "a");
  let resolve!: (value: SidebarTaskPageResponse) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const request = cache.request("ws", query, "b");
  store.setState((s) => ({
    sidebarArchivedTasks: { ...s.sidebarArchivedTasks, revisionByWorkspaceId: { ws: 1 } },
  }));
  expect(cache.get("a")).toBeNull();
  resolve(page("b"));
  await request.promise;
  expect(cache.get("b")).toBeNull();
  request.release();
  await load(cache, "c");
  store.setState({ workspaceContextGeneration: 2 });
  expect(cache.get("c")).toBeNull();
});

it("shares pending reads until the final consumer releases, and ignores late aborted completions", async () => {
  const { cache } = setup();
  let resolve!: (value: SidebarTaskPageResponse) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const a = cache.request("ws", query, "a"),
    b = cache.request("ws", query, "a");
  const signal = vi.mocked(querySidebarTasks).mock.calls[0][2]?.init?.signal;
  expect(querySidebarTasks).toHaveBeenCalledTimes(1);
  a.release();
  expect(signal?.aborted).toBe(false);
  b.release();
  expect(signal?.aborted).toBe(true);
  resolve(page());
  await b.promise;
  expect(cache.get("a")).toBeNull();
});

it("isolates app stores and does not keep results after clear", async () => {
  const a = setup(),
    b = setup();
  await load(a.cache, "a");
  expect(b.cache.get("a")).toBeNull();
  a.cache.clear();
  expect(a.cache.get("a")).toBeNull();
});

it("clears retained rows on logout even when the workspace generation stays the same", async () => {
  const { store, cache } = setup();
  await load(cache, "a");
  store.getState().clearAuthenticated();
  expect(cache.get("a")).toBeNull();
});

it("keeps view identity stable while invalidating a page after a color change", async () => {
  const { store, cache } = setup();
  await load(cache, "before-color-change");
  const before = sidebarTaskPageScope(store.getState());
  store.setState((state) => {
    state.userSettings.sidebarTaskColors = { "task-red": "red" };
  });
  expect(sidebarTaskPageScope(store.getState())).toBe(before);
  expect(cache.get("before-color-change")).toBeNull();
});

it("reuses the ranking key when unrelated state updates keep ranking inputs stable", () => {
  const { store } = setup();
  const state = store.getState();
  let snapshotEnumerations = 0;
  const snapshots = new Proxy(state.kanbanMulti.snapshots, {
    ownKeys(target) {
      snapshotEnumerations++;
      return Reflect.ownKeys(target);
    },
  });
  const rankingState = {
    ...state,
    kanbanMulti: { ...state.kanbanMulti, snapshots },
  } as AppState;

  const firstKey = sidebarTaskPageRankingKey(rankingState);
  const countAfterFirstCalculation = snapshotEnumerations;
  const secondKey = sidebarTaskPageRankingKey({
    ...rankingState,
    workspaceContextGeneration: rankingState.workspaceContextGeneration + 1,
  });

  expect(secondKey).toBe(firstKey);
  expect(snapshotEnumerations).toBe(countAfterFirstCalculation);
});

it("keeps cached pages after a semantically equal full settings event", async () => {
  const { store, cache } = setup();
  await load(cache, "equal-settings");
  const before = sidebarTaskPageScope(store.getState());
  const settings = store.getState().userSettings;
  registerUsersHandlers(store)["user.settings.updated"]?.(
    settingsMessage({
      revision: 1,
      sidebar_task_colors: structuredClone(settings.sidebarTaskColors),
      sidebar_task_color_automation: structuredClone(settings.sidebarTaskColorAutomation),
    }),
  );
  expect(sidebarTaskPageScope(store.getState())).toBe(before);
  expect(cache.get("equal-settings")).not.toBeNull();
});

it("keeps the current page after a group-indent write and its full settings acknowledgement", async () => {
  const { store, cache } = setup();
  await load(cache, "group-indent");
  const before = sidebarTaskPageScope(store.getState());
  store.getState().updateSidebarDraft({ groupIndent: false });
  const local = store.getState().sidebarViewsByWorkspace.ws;
  expect(local.draft?.groupIndent).toBe(false);
  expect(updateUserSettings).toHaveBeenCalledWith(
    expect.objectContaining({
      sidebar_view_state: expect.objectContaining({
        workspace_id: "ws",
        draft: expect.objectContaining({ group_indent: false }),
      }),
    }),
  );

  const userSettings = store.getState().userSettings;
  registerUsersHandlers(store)["user.settings.updated"]?.(
    settingsMessage({
      revision: 2,
      sidebar_views_by_workspace: {
        ws: {
          views: local.views.map(toApiSidebarView),
          active_view_id: local.activeViewId,
          draft: local.draft ? toApiSidebarDraft(local.draft) : null,
        },
      },
      sidebar_task_colors: structuredClone(userSettings.sidebarTaskColors),
      sidebar_task_color_automation: structuredClone(userSettings.sidebarTaskColorAutomation),
    }),
  );
  await Promise.resolve();
  await Promise.resolve();
  expect(store.getState().sidebarViewsByWorkspace.ws.draft?.groupIndent).toBe(false);
  expect(sidebarTaskPageScope(store.getState())).toBe(before);
  expect(cache.get("group-indent")).not.toBeNull();
});

it("evicts by aggregate serialized bytes before the entry-count limit", async () => {
  const { cache } = setup();
  const medium = page("x".repeat(750000));
  await load(cache, "a", medium);
  await load(cache, "b", medium);
  await load(cache, "c", medium);
  expect(cache.get("a")).toBeNull();
  expect(cache.get("b")).not.toBeNull();
  expect(cache.get("c")).not.toBeNull();
});

it("does not extend expiry when a cached view is activated with a pending refresh", async () => {
  const { cache } = setup();
  await load(cache, "a");
  vi.advanceTimersByTime(299999);
  let resolve!: (value: SidebarTaskPageResponse) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const refresh = cache.request("ws", query, "a");
  expect(cache.get("a")).not.toBeNull();
  vi.advanceTimersByTime(1);
  expect(cache.get("a")).toBeNull();
  resolve(page());
  await refresh.promise;
  refresh.release();
  expect(cache.get("a")).not.toBeNull();
});

it("invalidates only snapshots containing a changed task summary", async () => {
  const { store, cache } = setup();
  const { taskId, workflowId, workspaceId } = await import("@/lib/types/ids");
  const task = {
    id: taskId("task-a"),
    workspace_id: workspaceId("ws"),
    workflow_id: workflowId("flow"),
    workflow_step_id: "step",
    position: 0,
    title: "A",
    description: "",
    state: "TODO" as const,
    priority: "medium" as const,
    created_at: "2026-09-28T00:00:00Z",
    updated_at: "2026-09-28T00:00:00Z",
  };
  await load(cache, "a", { ...page(), entries: [{ kind: "task", task }] });
  await load(cache, "unaffected");
  store.setState({
    sidebarStatusSummaryByWorkspaceId: {
      ws: {
        "task-a": { revision: 2, updated_at: "2026-09-28T00:00:01Z", queued_prompt_count: 1 },
      },
    },
  });
  expect(cache.get("a")).toBeNull();
  expect(cache.get("unaffected")).not.toBeNull();
});

it("keeps lookups read-only and treats committed workspace changes as hard barriers", async () => {
  const { store, cache } = setup();
  await load(cache, "a");
  let resolve!: (value: SidebarTaskPageResponse) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((r) => {
      resolve = r;
    }),
  );
  const request = cache.request("ws", query, "b");
  const signal = vi.mocked(querySidebarTasks).mock.calls.at(-1)?.[2]?.init?.signal;
  const before = store.getState();
  expect(cache.get("a")).not.toBeNull();
  expect(store.getState()).toBe(before);
  expect(signal?.aborted).toBe(false);
  store.setState({ workspaceContextGeneration: 2 });
  expect(cache.get("a")).toBeNull();
  expect(signal?.aborted).toBe(true);
  store.setState({ workspaceContextGeneration: 1 });
  expect(cache.get("a")).toBeNull();
  resolve(page("b"));
  await request.promise;
  request.release();
  expect(cache.get("b")).toBeNull();
});

it("invalidates cached pages without restarting reads on deletion and unsubscribes consumers", async () => {
  const { cache } = setup();
  await load(cache, "a");
  let resolve!: (value: SidebarTaskPageResponse) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  const request = cache.request("ws", query, "b");
  const signal = vi.mocked(querySidebarTasks).mock.calls.at(-1)?.[2]?.init?.signal;
  const live = vi.fn(),
    disposed = vi.fn();
  cache.subscribeDeletedTasks(live);
  const unsubscribe = cache.subscribeDeletedTasks(disposed);
  unsubscribe();
  cache.removeTasks(new Set(["deleted"]));
  expect(signal?.aborted).toBe(false);
  expect(cache.get("a")).toBeNull();
  expect(live).toHaveBeenCalledWith(new Set(["deleted"]));
  expect(disposed).not.toHaveBeenCalled();
  resolve(page("b"));
  expect((await request.promise).provisional).toBe(true);
  expect(cache.get("b")).toBeNull();
  request.release();
});
