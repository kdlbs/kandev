import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import {
  createSidebarWorkspaceState,
  mapSidebarWorkspaces,
  selectSidebarViews,
} from "@/lib/state/slices/ui/sidebar-workspace-state";
import { toApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import type {
  SidebarWorkspaceStateApi,
  UserSettingsResponse,
} from "@/lib/types/http-user-settings";
import { pluginRegistry } from "@/lib/plugins/registry";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockResolvedValue({ settings: {} } as UserSettingsResponse);
});
const grouping = { reference: "plugin:acme:labels" as const, label: "Tag" };

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
it("persists an ordinary generated view and reuses only its unchanged identity after reload", async () => {
  const workspaces: Record<string, SidebarWorkspaceStateApi> = {};
  let revision = 0;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    const patch = payload.sidebar_view_state!;
    const current = workspaces[patch.workspace_id] ?? {
      views: createSidebarWorkspaceState().views.map(toApiSidebarView),
      active_view_id: "view-all-tasks",
      draft: null,
    };
    const views = new Map(current.views.map((view) => [view.id, view]));
    for (const id of patch.view_ids_delete ?? []) views.delete(id);
    for (const view of patch.views_upsert ?? []) views.set(view.id, view);
    workspaces[patch.workspace_id] = {
      views: (patch.view_order ?? [...views.keys()]).map((id) => views.get(id)!),
      active_view_id: patch.active_view_id ?? current.active_view_id,
      draft: "draft" in patch ? patch.draft! : current.draft,
    };
    return {
      settings: { revision: ++revision, sidebar_views_by_workspace: structuredClone(workspaces) },
    } as UserSettingsResponse;
  });
  const reload = () =>
    createAppStore({
      workspaces: { items: [], activeId: "a" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces(workspaces),
    });
  let store = reload();
  const id = store.getState().createSidebarView(grouping);
  await store.getState().waitForSidebarViewSync("a");
  store = reload();
  expect(selectSidebarViews(store.getState()).activeViewId).toBe(id);
  const generated = selectSidebarViews(store.getState()).views.find((v) => v.id === id)!;
  expect(generated.group).toBe(grouping.reference);
  expect(generated.name).toBe("Tag");
  expect(store.getState().createSidebarView(grouping)).toBe(id);
  await store.getState().waitForSidebarViewSync("a");
  expect(selectSidebarViews(store.getState()).views.map((v) => v.id)).toEqual([
    "view-all-tasks",
    id,
  ]);
  store.getState().renameSidebarView(id!, "My tags");
  const next = store.getState().createSidebarView(grouping);
  expect(next).not.toBe(id);
  expect(selectSidebarViews(store.getState()).views.find((v) => v.id === id)?.name).toBe("My tags");
  await store.getState().waitForSidebarViewSync("a");
  store = reload();
  expect(selectSidebarViews(store.getState()).views.find((view) => view.id === id)?.name).toBe(
    "My tags",
  );
  expect(store.getState().createSidebarView(grouping)).toBe(next);
  await store.getState().waitForSidebarViewSync("a");
});

it("preserves drafts and view limits without losing workspace isolation or name collisions", async () => {
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  const custom = store.getState().createSidebarView()!;
  store.getState().renameSidebarView(custom, "Tag");
  const generated = store.getState().createSidebarView(grouping)!;
  expect(selectSidebarViews(store.getState()).views.find((v) => v.id === generated)?.name).toBe(
    "Tag 2",
  );
  store.getState().updateSidebarDraft({ group: "state" });
  const draft = selectSidebarViews(store.getState()).draft;
  expect(store.getState().createSidebarView(grouping)).toBeNull();
  expect(selectSidebarViews(store.getState()).draft).toEqual(draft);
  store.getState().setActiveWorkspace("b");
  const inB = store.getState().createSidebarView(grouping);
  expect(inB).not.toBe(generated);
  expect(store.getState().sidebarViewsByWorkspace.a.draft).toEqual(draft);
  for (let i = 0; i < 48; i++) store.getState().createSidebarView();
  expect(selectSidebarViews(store.getState()).views).toHaveLength(50);
  expect(
    store.getState().createSidebarView({ ...grouping, reference: "plugin:acme:other" }),
  ).toBeNull();
  expect(store.getState().createSidebarView(grouping)).toBe(inB);
  await store.getState().waitForSidebarViewSync("a");
  await store.getState().waitForSidebarViewSync("b");
});

it("keeps complete host declarations independent of browser facets and fences provider/workspace generations", () => {
  const provider = { reference: grouping.reference, label: "Tag", generation: "install-1" };
  pluginRegistry.setSidebarGroupings([provider]);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([provider]);
  const a = pluginRegistry.sidebarGroupingIdentity(grouping.reference, "a");
  const b = pluginRegistry.sidebarGroupingIdentity(grouping.reference, "b");
  pluginRegistry.invalidateSidebarGroupings("acme", "a");
  expect(pluginRegistry.sidebarGroupingIdentity(grouping.reference, "a")).not.toBe(a);
  expect(pluginRegistry.sidebarGroupingIdentity(grouping.reference, "b")).toBe(b);
  expect(pluginRegistry.sidebarGroupingIdentity("repository", "a")).toBe("");
  const updated = pluginRegistry.sidebarGroupingIdentity(grouping.reference, "a");
  pluginRegistry.setSidebarGroupings([]);
  expect(pluginRegistry.sidebarGroupingIdentity(grouping.reference, "a")).not.toBe(updated);
  expect(pluginRegistry.getSidebarGroupings()).toEqual([]);
  pluginRegistry.setSidebarGroupings([{ ...provider, generation: "install-2" }]);
  expect(pluginRegistry.sidebarGroupingIdentity(grouping.reference, "a")).not.toBe(a);
  pluginRegistry.setSidebarGroupings([]);
});
