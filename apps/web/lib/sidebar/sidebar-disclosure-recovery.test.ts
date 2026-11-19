import { afterEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { createDefaultSidebarView } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UserSettingsResponse } from "@/lib/types/http-user-settings";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import { pluginRegistry } from "@/lib/plugins/registry";
import { SidebarTaskPageCache } from "./sidebar-task-page-cache";

vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));
vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
afterEach(() => {
  vi.resetAllMocks();
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
      reference + ":untagged",
    ];
    const valid = [keep, reference + ":untagged"];
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
