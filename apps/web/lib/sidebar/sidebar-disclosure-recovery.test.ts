import { afterEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { createDefaultSidebarView } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UserSettingsResponse } from "@/lib/types/http-user-settings";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { pluginRegistry } from "@/lib/plugins/registry";
import { SidebarTaskPageCache } from "./sidebar-task-page-cache";
import { ApiError } from "@/lib/api/client";
import { sidebarTaskQueryError } from "./sidebar-task-query-error";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
afterEach(() => {
  vi.resetAllMocks();
  vi.useRealTimers();
  pluginRegistry.setSidebarGroupings([]);
});

const WORKSPACE_CHANGE_DURING_SAVE = "workspace changed while saving";

it.each(["current", "source changed", "workspace changed", WORKSPACE_CHANGE_DURING_SAVE])(
  "recovers oversized disclosure only for its current owner: %s",
  async (mode) => {
    const store = createAppStore();
    const reference = "plugin:acme:labels";
    const keep = `plugin-group:${"a".repeat(64)}`;
    const keys = [
      ...Array.from({ length: 6000 }, (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`),
      keep,
      reference + ":unassigned",
    ];
    const valid = [keep, reference + ":unassigned"];
    store.setState({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: {
        ws: {
          views: [
            {
              ...createDefaultSidebarView("saved", "Saved"),
              group: reference,
              collapsedGroups: keys,
            },
          ],
          activeViewId: "saved",
          draft: null,
          syncError: null,
        },
      },
    });
    pluginRegistry.setSidebarGroupings([{ reference, label: "Labels", generation: "g1" }]);
    const write = Promise.withResolvers<UserSettingsResponse>();
    vi.mocked(updateUserSettings)
      .mockReturnValueOnce(write.promise)
      .mockResolvedValue({ settings: {} } as UserSettingsResponse);
    store.getState().renameSidebarView("saved", "Renamed");
    const response = Promise.withResolvers<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockReturnValueOnce(response.promise);
    const query: SidebarTaskQuery = {
      group: reference,
      disclosure_view_id: "saved",
      collapsed_group_keys: keys,
      collapsed_task_ids: [],
      filters: [],
      sort: { key: "title", direction: "asc" },
      page: 1,
      page_size: 100,
      locale: "en",
    };
    const cache = new SidebarTaskPageCache(store);
    const request = cache.request("ws", query, "oversized");
    const outcome = request.promise.catch((error: unknown) => error);
    expect(querySidebarTasks).not.toHaveBeenCalled();
    if (mode === WORKSPACE_CHANGE_DURING_SAVE)
      store.setState({
        workspaces: { items: [], activeId: "other" },
        workspaceContextGeneration: 2,
      });
    write.resolve({ settings: {} } as UserSettingsResponse);
    if (mode !== WORKSPACE_CHANGE_DURING_SAVE)
      await vi.waitFor(() => expect(querySidebarTasks).toHaveBeenCalledTimes(1));
    if (mode === "source changed") pluginRegistry.invalidateSidebarGroupings("acme", "ws");
    if (mode === "workspace changed")
      store.setState({
        workspaces: { items: [], activeId: "other" },
        workspaceContextGeneration: 2,
      });
    const page: SidebarTaskPageResponse = {
      query_key: "metadata-read",
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
    };
    response.resolve(page);
    await expect(outcome).resolves.toMatchObject({ name: "AbortError" });
    expect(cache.get("oversized")).toBeNull();
    const saved = store.getState().sidebarViewsByWorkspace.ws.views[0];
    expect(saved.collapsedGroups).toEqual(mode === "current" ? valid : keys);
    request.release();
    if (mode === "current") {
      vi.mocked(querySidebarTasks).mockResolvedValueOnce(page);
      const next = cache.request(
        "ws",
        { ...query, collapsed_group_keys: saved.collapsedGroups },
        "bounded",
      );
      await expect(next.promise).resolves.toMatchObject({ query_key: "metadata-read" });
      expect(cache.get("bounded")).not.toBeNull();
      next.release();
    }
    if (mode === WORKSPACE_CHANGE_DURING_SAVE) expect(querySidebarTasks).not.toHaveBeenCalled();
    cache.clear();
  },
);

it("latches failed disclosure compaction until explicit Retry without losing saved intent", async () => {
  const reference = "plugin:acme:labels";
  const keep = `plugin-group:${"a".repeat(64)}`;
  const keys = Array.from(
    { length: 6000 },
    (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`,
  );
  keys.push(keep);
  const store = createAppStore({
    workspaces: { items: [], activeId: "ws" },
    sidebarViewsByWorkspace: {
      ws: {
        views: [
          {
            ...createDefaultSidebarView("saved", "Saved"),
            group: reference,
            collapsedGroups: keys,
          },
        ],
        activeViewId: "saved",
        draft: null,
        syncError: null,
      },
    },
  });
  pluginRegistry.setSidebarGroupings([{ reference, label: "Labels", generation: "g1" }]);
  const page: SidebarTaskPageResponse = {
    query_key: "metadata",
    page: 1,
    page_size: 100,
    total_entries: 0,
    total_tasks: 0,
    total_visible_tasks: 0,
    has_next: false,
    has_previous: false,
    grouping_generation: "g1",
    grouping_revision: "r1",
    grouping_keys: [keep],
    entries: [],
  };
  vi.mocked(querySidebarTasks).mockResolvedValue(page);
  const cleanup = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings).mockReturnValueOnce(cleanup.promise);
  const query: SidebarTaskQuery = {
    group: reference,
    disclosure_view_id: "saved",
    collapsed_group_keys: keys,
    collapsed_task_ids: [],
    filters: [],
    sort: { key: "title", direction: "asc" },
    page: 1,
    page_size: 100,
    locale: "en",
  };
  const cache = new SidebarTaskPageCache(store);
  const read = cache.request("ws", query, "oversized");
  const outcome = read.promise.catch((error: unknown) => error);
  await vi.waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  cleanup.reject(new ApiError("Settings unavailable", 503, {}));
  const error = await outcome;
  await store.getState().waitForSidebarViewSync("ws");
  expect(error).toBeInstanceOf(Error);
  expect(error).not.toMatchObject({ name: "AbortError" });
  expect(sidebarTaskQueryError(error, false, ((key: string) => key) as never).canRetry).toBe(true);
  expect(store.getState().sidebarViewsByWorkspace.ws.views[0].collapsedGroups).toEqual(keys);
  expect(cache.get("oversized")).toBeNull();
  vi.useFakeTimers();
  await vi.advanceTimersByTimeAsync(60_000);
  const foreground = cache.request("ws", query, "oversized", { foregroundRecoveryEpisode: {} });
  await expect(foreground.promise).rejects.toBe(error);
  expect(updateUserSettings).toHaveBeenCalledTimes(1);
  expect(querySidebarTasks).toHaveBeenCalledTimes(1);
  vi.mocked(updateUserSettings).mockResolvedValueOnce({ settings: {} } as UserSettingsResponse);
  const retry = cache.request("ws", query, "oversized", { manualRetry: true });
  await expect(retry.promise).rejects.toMatchObject({ name: "AbortError" });
  expect(store.getState().sidebarViewsByWorkspace.ws.views[0].collapsedGroups).toEqual([keep]);
  expect(updateUserSettings).toHaveBeenCalledTimes(2);
  foreground.release();
  retry.release();
  read.release();
  cache.clear();
});
