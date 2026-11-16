import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import { pluginRegistry } from "@/lib/plugins/registry";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));
beforeEach(() => vi.clearAllMocks());
const grouping = { reference: "plugin:acme:labels" as const, label: "Tag" };

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
it("creates one ordinary grouped view and reuses only its unchanged generated view", () => {
  const store = createAppStore({ workspaces: { items: [], activeId: "a" } });
  const id = store.getState().createSidebarView(grouping);
  const generated = selectSidebarViews(store.getState()).views.find((v) => v.id === id)!;
  expect(generated.group).toBe(grouping.reference);
  expect(generated.name).toBe("Tag");
  expect(store.getState().createSidebarView(grouping)).toBe(id);
  expect(selectSidebarViews(store.getState()).views.map((v) => v.id)).toEqual([
    "view-all-tasks",
    id,
  ]);
  store.getState().renameSidebarView(id!, "My tags");
  const next = store.getState().createSidebarView(grouping);
  expect(next).not.toBe(id);
  expect(selectSidebarViews(store.getState()).views.find((v) => v.id === id)?.name).toBe("My tags");
  expect(updateUserSettings).toHaveBeenCalledWith(
    expect.objectContaining({
      sidebar_view_state: expect.objectContaining({
        workspace_id: "a",
        active_view_id: id,
        views: expect.arrayContaining([expect.objectContaining({ group: grouping.reference })]),
      }),
    }),
  );
});

it("preserves drafts and view limits without losing workspace isolation or name collisions", () => {
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
