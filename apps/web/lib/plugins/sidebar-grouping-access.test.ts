import { expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { listSidebarGroupingProviders } from "@/lib/api/domains/plugins-api";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { createAppStore } from "@/lib/state/store";
import { SidebarTaskPageCache } from "@/lib/sidebar/sidebar-task-page-cache";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import {
  seedSidebarGroupingCatalog,
  refreshSidebarGroupingCatalog,
} from "./sidebar-grouping-catalog";
import { pluginRegistry } from "./registry";
import { registerSidebarGroupingHandlers } from "@/lib/ws/handlers/sidebar-grouping";

vi.mock("@/lib/api/domains/plugins-api", () => ({ listSidebarGroupingProviders: vi.fn() }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ querySidebarTasks: vi.fn() }));

it.each([401, 403])(
  "never reuses pre-denial pages after identical discovery (%s)",
  async (status) => {
    const store = createAppStore();
    store.setState({ workspaces: { items: [], activeId: "ws" }, workspaceContextGeneration: 1 });
    const provider = {
      reference: "plugin:acme:labels" as const,
      label: "Labels",
      generation: "same",
    };
    seedSidebarGroupingCatalog(store, [provider]);
    const cache = new SidebarTaskPageCache(store);
    const query: SidebarTaskQuery = {
      filters: [],
      collapsed_group_keys: [],
      collapsed_task_ids: [],
      sort: { key: "state", direction: "asc" },
      group: provider.reference,
      page: 1,
      page_size: 100,
      locale: "en",
    };
    const page: SidebarTaskPageResponse = {
      query_key: "old",
      page: 1,
      page_size: 100,
      total_entries: 0,
      total_tasks: 0,
      total_visible_tasks: 0,
      has_previous: false,
      has_next: false,
      entries: [],
      grouping_generation: "same",
      grouping_revision: "old",
    };
    vi.mocked(querySidebarTasks).mockResolvedValueOnce(page);
    const loaded = cache.request("ws", query, "retained");
    await loaded.promise;
    loaded.release();
    expect(cache.get("retained")).not.toBeNull();
    const pending = Promise.withResolvers<SidebarTaskPageResponse>();
    vi.mocked(querySidebarTasks).mockReturnValueOnce(pending.promise);
    const old = cache.request("ws", query, "inflight");
    const settled = old.promise.catch((error: unknown) => error);
    vi.mocked(listSidebarGroupingProviders).mockRejectedValueOnce(
      new ApiError("denied", status, null),
    );
    await refreshSidebarGroupingCatalog(store);
    vi.mocked(listSidebarGroupingProviders).mockResolvedValueOnce([provider]);
    await refreshSidebarGroupingCatalog(store);
    expect(cache.get("retained")).toBeNull();
    pending.resolve(page);
    await expect(settled).resolves.toMatchObject({ name: "AbortError" });
    expect(cache.get("inflight")).toBeNull();
    old.release();
    pluginRegistry.setSidebarGroupings([]);
  },
);

it("fences a removed provider while its catalog refresh is still pending", async () => {
  const store = createAppStore();
  store.setState({ workspaces: { items: [], activeId: "ws" }, workspaceContextGeneration: 1 });
  const provider = {
    reference: "plugin:acme:labels" as const,
    label: "Labels",
    generation: "same",
  };
  seedSidebarGroupingCatalog(store, [provider]);
  const cache = new SidebarTaskPageCache(store);
  const query: SidebarTaskQuery = {
    filters: [],
    collapsed_group_keys: [],
    collapsed_task_ids: [],
    sort: { key: "state", direction: "asc" },
    group: provider.reference,
    page: 1,
    page_size: 100,
    locale: "en",
  };
  const pending = Promise.withResolvers<SidebarTaskPageResponse>();
  vi.mocked(querySidebarTasks).mockReturnValueOnce(pending.promise);
  const old = cache.request("ws", query, "removed");
  const discovery = Promise.withResolvers<(typeof provider)[]>();
  vi.mocked(listSidebarGroupingProviders).mockReturnValueOnce(discovery.promise);
  registerSidebarGroupingHandlers(store)["plugin.sidebar-grouping.updated"]!({
    type: "notification",
    action: "plugin.sidebar-grouping.updated",
    payload: { catalog: true, plugin_id: "acme" },
  });
  pending.resolve({
    query_key: "old",
    page: 1,
    page_size: 100,
    total_entries: 0,
    total_tasks: 0,
    total_visible_tasks: 0,
    has_previous: false,
    has_next: false,
    entries: [],
    grouping_generation: "same",
    grouping_revision: "old",
  });
  await expect(old.promise).rejects.toMatchObject({ name: "AbortError" });
  expect(cache.get("removed")).toBeNull();
  old.release();
  discovery.resolve([]);
  await vi.waitFor(() => expect(pluginRegistry.getSidebarGroupings()).toEqual([]));
});
