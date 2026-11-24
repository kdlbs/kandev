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
import {
  applyDraftMutationForTest,
  applyPluginSelectionForTest,
  applyViewMutationsForTest,
} from "@/lib/state/slices/ui/sidebar-view-test-helpers";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockResolvedValue({ settings: {} } as UserSettingsResponse);
});
const grouping = { reference: "plugin:acme:labels" as const, label: "Tag" };
const ALL_TASKS = "view-all-tasks";

function mockSidebarSettings() {
  const workspaces: Record<string, SidebarWorkspaceStateApi> = {};
  let revision = 0;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    const patch = payload.sidebar_view_state!;
    const current = workspaces[patch.workspace_id] ?? {
      views: createSidebarWorkspaceState().views.map(toApiSidebarView),
      active_view_id: ALL_TASKS,
      draft: null,
    };
    const views = new Map(current.views.map((view) => [view.id, view]));
    applyViewMutationsForTest(views, patch);
    workspaces[patch.workspace_id] = applyPluginSelectionForTest(
      {
        views: (patch.view_order ?? [...views.keys()]).map((id) => views.get(id)!),
        active_view_id: patch.active_view_id ?? current.active_view_id,
        draft: applyDraftMutationForTest(
          views,
          patch.active_view_id ?? current.active_view_id,
          current.draft,
          patch,
        ),
      },
      patch,
    );
    return {
      settings: { revision: ++revision, sidebar_views_by_workspace: structuredClone(workspaces) },
    } as UserSettingsResponse;
  });
  const reload = () =>
    createAppStore({
      workspaces: { items: [], activeId: "a" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces(workspaces),
    });
  return { workspaces, reload };
}

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.1
it("persists an ordinary generated view and reuses only its unchanged identity after reload", async () => {
  const { reload } = mockSidebarSettings();
  let store = reload();
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  const id = selectSidebarViews(store.getState()).activeViewId;
  store = reload();
  expect(selectSidebarViews(store.getState()).activeViewId).toBe(id);
  const generated = selectSidebarViews(store.getState()).views.find((v) => v.id === id)!;
  expect(generated.group).toBe(grouping.reference);
  expect(generated.name).toBe("Tag");
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  expect(selectSidebarViews(store.getState()).activeViewId).toBe(id);
  expect(selectSidebarViews(store.getState()).views.map((v) => v.id)).toEqual([
    "view-all-tasks",
    id,
  ]);
  store.getState().renameSidebarView(id!, "My tags");
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  const next = selectSidebarViews(store.getState()).activeViewId;
  expect(next).not.toBe(id);
  expect(selectSidebarViews(store.getState()).views.find((v) => v.id === id)?.name).toBe("My tags");
  await store.getState().waitForSidebarViewSync("a");
  store = reload();
  expect(selectSidebarViews(store.getState()).views.find((view) => view.id === id)?.name).toBe(
    "My tags",
  );
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  expect(selectSidebarViews(store.getState()).activeViewId).toBe(next);
});

it("preserves drafts and view limits without losing workspace isolation or name collisions", async () => {
  const { reload } = mockSidebarSettings();
  const store = reload();
  const custom = store.getState().createSidebarView()!;
  store.getState().renameSidebarView(custom, "Tag");
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  const generated = selectSidebarViews(store.getState()).activeViewId;
  expect(selectSidebarViews(store.getState()).views.find((v) => v.id === generated)?.name).toBe(
    "Tag 2",
  );
  store.getState().updateSidebarDraft({ group: "state" });
  const draft = selectSidebarViews(store.getState()).draft;
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(false);
  expect(selectSidebarViews(store.getState()).draft).toEqual(draft);
  store.getState().setActiveWorkspace("b");
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  const inB = selectSidebarViews(store.getState()).activeViewId;
  expect(inB).not.toBe(generated);
  expect(store.getState().sidebarViewsByWorkspace.a.draft).toEqual(draft);
  for (let i = 0; i < 48; i++) store.getState().createSidebarView();
  expect(selectSidebarViews(store.getState()).views).toHaveLength(50);
  expect(
    await store
      .getState()
      .selectSidebarPluginGrouping({ ...grouping, reference: "plugin:acme:other" }),
  ).toBe(false);
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  expect(selectSidebarViews(store.getState()).activeViewId).toBe(inB);
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

it("does not replay a committed provider selection after its transport ACK is lost", async () => {
  const { reload, workspaces } = mockSidebarSettings();
  let store = reload();
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(true);
  const generatedId = workspaces.a.active_view_id;
  store = reload();
  const send = vi.mocked(updateUserSettings).getMockImplementation()!;
  vi.mocked(updateUserSettings).mockImplementationOnce(async (payload) => {
    await send(payload);
    const current = workspaces.a;
    current.views.find((view) => view.id === generatedId)!.name = "Renamed remotely";
    current.active_view_id = ALL_TASKS;
    throw new TypeError("Committed response was lost");
  });
  expect(await store.getState().selectSidebarPluginGrouping(grouping)).toBe(false);
  const persisted = selectSidebarViews(reload().getState());
  expect(persisted.activeViewId).toBe(ALL_TASKS);
  expect(persisted.views.filter((view) => view.group === grouping.reference)).toHaveLength(1);
  expect(persisted.views.find((view) => view.id === generatedId)?.name).toBe("Renamed remotely");
});
