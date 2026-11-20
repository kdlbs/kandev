import { afterEach, expect, it, vi } from "vitest";
import { workspaceId } from "@/lib/types/ids";
import { createAppStore } from "@/lib/state/store";
import { createDefaultSidebarView } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { toApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import type { UserSettingsResponse } from "@/lib/types/http-user-settings";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { pluginRegistry } from "@/lib/plugins/registry";
import { SidebarTaskPageCache } from "./sidebar-task-page-cache";
import { sidebarTaskQueryError } from "./sidebar-task-query-error";
import { applyViewUpdateForTest } from "@/lib/state/slices/ui/sidebar-view-test-helpers";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
afterEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
  pluginRegistry.setSidebarGroupings([]);
});

function disclosureFixture() {
  const reference = "plugin:acme:labels" as const;
  const keep = `plugin-group:${"f".repeat(64)}`;
  const keys = [
    ...Array.from({ length: 6000 }, (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`),
    keep,
  ];
  const view = {
    ...createDefaultSidebarView("saved", "Saved"),
    group: reference,
    collapsedGroups: keys,
  };
  const store = createAppStore({
    workspaces: { items: [], activeId: "ws" },
    sidebarViewsByWorkspace: {
      ws: { views: [view], activeViewId: view.id, draft: null, syncError: null },
    },
  });
  pluginRegistry.setSidebarGroupings([{ reference, label: "Labels", generation: "g1" }]);
  const query: SidebarTaskQuery = {
    group: reference,
    disclosure_view_id: view.id,
    collapsed_group_keys: keys,
    collapsed_task_ids: [],
    filters: [],
    sort: { key: "title", direction: "asc" },
    page: 1,
    page_size: 100,
    locale: "en",
  };
  const metadata = (valid: string[]): SidebarTaskPageResponse => ({
    query_key: "probe",
    page: 1,
    page_size: 100,
    total_entries: 0,
    total_tasks: 0,
    total_visible_tasks: 0,
    has_next: false,
    has_previous: false,
    grouping_generation: "g1",
    grouping_revision: "r1",
    grouping_keys: valid,
    entries: [],
  });
  return {
    store,
    reference,
    keep,
    keys,
    view,
    query,
    metadata,
    cache: new SidebarTaskPageCache(store),
  };
}

it.each([false, true])(
  "admits queued compaction only for its current provider: invalidated=%s",
  async (invalidated) => {
    const f = disclosureFixture();
    let persisted = toApiSidebarView(f.view);
    let revision = 0;
    const acknowledge = (): UserSettingsResponse => ({
      settings: {
        user_id: "default-user",
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: ++revision,
        sidebar_views_by_workspace: {
          ws: { views: [structuredClone(persisted)], active_view_id: f.view.id, draft: null },
        },
      },
    });
    const rename = Promise.withResolvers<UserSettingsResponse>();
    let renameAck: UserSettingsResponse | undefined;
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      for (const view of payload.sidebar_view_state?.views_update ?? [])
        persisted = applyViewUpdateForTest(persisted, view);
      if (!renameAck) {
        renameAck = acknowledge();
        return rename.promise;
      }
      return acknowledge();
    });
    const response = Promise.withResolvers<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockReturnValueOnce(response.promise);
    const reconcile = f.store.getState().reconcileSidebarGroupDisclosure;
    const observed = vi.fn((...args: Parameters<typeof reconcile>) => reconcile(...args));
    f.store.setState({ reconcileSidebarGroupDisclosure: observed });
    const read = f.cache.request("ws", f.query, "old-provider");
    const outcome = read.promise.catch((error: unknown) => error);
    try {
      await vi.waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
      f.store.getState().renameSidebarView(f.view.id, "Renamed");
      response.resolve(f.metadata([]));
      await vi.waitFor(() => expect(observed).toHaveBeenCalledTimes(1));
      if (invalidated) pluginRegistry.invalidateSidebarGroupings("acme", "ws");
      rename.resolve(renameAck!);
      await expect(outcome).resolves.toMatchObject({ name: "AbortError" });
      await f.store.getState().waitForSidebarViewSync("ws");
      expect(persisted.collapsed_groups).toEqual(invalidated ? f.keys : []);
      expect(f.cache.get("old-provider")).toBeNull();
      if (invalidated) {
        vi.mocked(querySidebarTasks).mockResolvedValueOnce(f.metadata([f.keep]));
        const current = f.cache.request("ws", f.query, "current-provider");
        await expect(current.promise).rejects.toMatchObject({ name: "AbortError" });
        current.release();
        expect(persisted.collapsed_groups).toEqual([f.keep]);
      }
    } finally {
      rename.resolve(renameAck!);
      read.release();
      f.cache.clear();
    }
  },
);

it("a compaction rejected after the metadata deadline requires manual Retry", async () => {
  vi.useFakeTimers();
  const f = disclosureFixture();
  vi.mocked(querySidebarTasks).mockResolvedValue(f.metadata([f.keep]));
  const write = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(write.promise)
    .mockResolvedValue({ settings: {} } as UserSettingsResponse);
  const read = f.cache.request("ws", f.query, "slow-compaction");
  const outcome = read.promise.catch((error: unknown) => error);
  try {
    await vi.advanceTimersByTimeAsync(0);
    expect(updateUserSettings).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(11_000);
    write.reject(new ApiError("Settings unavailable", 503, {}));
    await vi.advanceTimersByTimeAsync(5_000);
    const error = await outcome;
    expect(updateUserSettings).toHaveBeenCalledTimes(1);
    expect(querySidebarTasks).toHaveBeenCalledTimes(1);
    expect(sidebarTaskQueryError(error, false, ((key: string) => key) as never).canRetry).toBe(
      true,
    );
    expect(f.store.getState().sidebarViewsByWorkspace.ws.views[0].collapsedGroups).toEqual(f.keys);
    const retry = f.cache.request("ws", f.query, "slow-compaction", { manualRetry: true });
    await expect(retry.promise).rejects.toMatchObject({ name: "AbortError" });
    retry.release();
    expect(updateUserSettings).toHaveBeenCalledTimes(2);
    expect(f.store.getState().sidebarViewsByWorkspace.ws.views[0].collapsedGroups).toEqual([
      f.keep,
    ]);
  } finally {
    read.release();
    f.cache.clear();
  }
});

it("a provider event during network backoff cannot resend a retired disclosure probe", async () => {
  vi.useFakeTimers();
  const f = disclosureFixture();
  vi.mocked(querySidebarTasks).mockResolvedValue(f.metadata([]));
  vi.mocked(updateUserSettings)
    .mockRejectedValueOnce(new TypeError("Network disconnected"))
    .mockResolvedValue({ settings: {} } as UserSettingsResponse);
  const read = f.cache.request("ws", f.query, "network-retired-probe");
  const outcome = read.promise.catch((error: unknown) => error);
  try {
    await vi.advanceTimersByTimeAsync(0);
    expect(updateUserSettings).toHaveBeenCalledTimes(1);
    pluginRegistry.invalidateSidebarGroupings("acme", "ws");
    await vi.advanceTimersByTimeAsync(1000);
    await outcome;
    expect(updateUserSettings).toHaveBeenCalledTimes(1);
    expect(f.store.getState().sidebarViewsByWorkspace.ws.views[0].collapsedGroups).toEqual(f.keys);
    expect(f.cache.get("network-retired-probe")).toBeNull();
  } finally {
    read.release();
    f.cache.clear();
  }
});

it.each([false, true])(
  "skipped compaction preserves failed predecessor intent: successor=%s",
  async (successor) => {
    const f = disclosureFixture();
    let persisted = toApiSidebarView(f.view);
    const first = Promise.withResolvers<UserSettingsResponse>();
    const metadata = Promise.withResolvers<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockReturnValueOnce(metadata.promise);
    let writes = 0;
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      if (++writes === 1) return first.promise;
      for (const update of payload.sidebar_view_state?.views_update ?? [])
        persisted = applyViewUpdateForTest(persisted, update);
      return {
        settings: {
          user_id: "default-user",
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: writes,
          sidebar_views_by_workspace: {
            ws: { views: [structuredClone(persisted)], active_view_id: f.view.id, draft: null },
          },
        },
      };
    });
    const reconcile = f.store.getState().reconcileSidebarGroupDisclosure;
    const observed = vi.fn((...args: Parameters<typeof reconcile>) => reconcile(...args));
    f.store.setState({ reconcileSidebarGroupDisclosure: observed });
    const read = f.cache.request("ws", f.query, "skipped-between-writes");
    const outcome = read.promise.catch((error: unknown) => error);
    try {
      await vi.waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
      f.store.getState().renameSidebarView(f.view.id, "First rename");
      metadata.resolve(f.metadata([]));
      await vi.waitFor(() => expect(observed).toHaveBeenCalledTimes(1));
      if (successor) f.store.getState().toggleSidebarGroupCollapsed(f.view.id, "third");
      pluginRegistry.invalidateSidebarGroupings("acme", "ws");
      first.reject(new ApiError("Settings unavailable", 503, {}));
      await outcome;
      await f.store.getState().waitForSidebarViewSync("ws");
      expect(persisted.name).toBe(successor ? "First rename" : "Saved");
      expect(persisted.collapsed_groups).toEqual(successor ? [...f.keys, "third"] : f.keys);
      const current = f.store.getState().sidebarViewsByWorkspace.ws;
      expect(current.views[0].name).toBe(persisted.name);
      expect(current.syncPending).toBe(false);
      expect(f.cache.get("skipped-between-writes")).toBeNull();
    } finally {
      first.reject(new ApiError("Cleanup", 503, {}));
      read.release();
      f.cache.clear();
    }
  },
);

it("a queued draft prune cannot acquire its remotely saved owner's disclosure", async () => {
  const f = disclosureFixture();
  f.store.setState((state) => ({
    sidebarViewsByWorkspace: {
      ...state.sidebarViewsByWorkspace,
      ws: {
        ...state.sidebarViewsByWorkspace.ws,
        views: [{ ...f.view, group: "state", collapsedGroups: [] }],
        draft: {
          baseViewId: f.view.id,
          group: f.reference,
          filters: [],
          sort: f.view.sort,
          collapsedGroups: f.keys,
        },
      },
    },
  }));
  const predecessor = Promise.withResolvers<UserSettingsResponse>();
  const saved = { ...toApiSidebarView(f.view), name: "Renamed" };
  const response: UserSettingsResponse = {
    settings: {
      user_id: "default-user",
      workspace_id: workspaceId("ws"),
      repository_ids: [],
      updated_at: "",
      revision: 2,
      sidebar_views_by_workspace: {
        ws: { views: [saved], active_view_id: saved.id, draft: null },
      },
    },
  };
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(predecessor.promise)
    .mockResolvedValue(response);
  f.store.getState().renameSidebarView(f.view.id, "Renamed");
  const prune = f.store
    .getState()
    .reconcileSidebarGroupDisclosure(f.view.id, f.reference, f.keys, [f.keep]);
  predecessor.resolve(response);
  await prune;
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(updateUserSettings).toHaveBeenCalledTimes(1);
  expect(f.store.getState().sidebarViewsByWorkspace.ws.draft).toBeNull();
  expect(f.store.getState().sidebarViewsByWorkspace.ws.views[0].collapsedGroups).toEqual(f.keys);
});
