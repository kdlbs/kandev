import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type {
  SidebarWorkspaceStateApi,
  UserSettingsResponse,
} from "@/lib/types/http-user-settings";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { toApiSidebarView } from "./sidebar-view-wire";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
});

const TAG = "plugin:tags:tags";
const MULTI = "plugin:tags:multi-tags";
const TAG_KEY = "tag-disclosure";
const MULTI_KEY = "multi-disclosure";

function draftWorkspace() {
  const base = {
    ...createDefaultSidebarView("base", "Tag"),
    group: TAG as `plugin:${string}:${string}`,
    collapsedGroups: [TAG_KEY],
  };
  let server: SidebarWorkspaceStateApi = {
    views: [toApiSidebarView(base)],
    active_view_id: base.id,
    draft: null,
  };
  let revision = 0;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    const patch = payload.sidebar_view_state!;
    const views = new Map(server.views.map((view) => [view.id, view]));
    for (const id of patch.view_ids_delete ?? []) views.delete(id);
    for (const view of patch.views_upsert ?? []) views.set(view.id, view);
    server = {
      views: (patch.view_order ?? [...views.keys()]).map((id) => views.get(id)!),
      active_view_id: patch.active_view_id ?? server.active_view_id,
      draft: "draft" in patch ? patch.draft! : server.draft,
    };
    return {
      settings: { revision: ++revision, sidebar_views_by_workspace: { ws: server } },
    } as UserSettingsResponse;
  });
  const reload = () =>
    createAppStore({
      workspaces: { activeId: "ws", items: [] },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: server }),
    });
  return { reload, base };
}

it.each(["discard", "save as", "overwrite"])(
  "owns changed-group preview disclosure through %s and reload",
  async (finish) => {
    const { reload, base } = draftWorkspace();
    const store = reload();
    store.getState().updateSidebarDraft({ group: MULTI });
    await store.getState().waitForSidebarViewSync("ws");
    store.getState().toggleSidebarGroupCollapsed(base.id, MULTI_KEY);
    await store.getState().waitForSidebarViewSync("ws");
    let current = store.getState().sidebarViewsByWorkspace.ws;
    expect(current.views[0].collapsedGroups).toEqual([TAG_KEY]);
    expect(current.draft).toMatchObject({ group: MULTI, collapsedGroups: [MULTI_KEY] });
    store.getState().toggleSidebarGroupCollapsed(base.id, MULTI_KEY);
    await store.getState().waitForSidebarViewSync("ws");
    expect(store.getState().sidebarViewsByWorkspace.ws.draft).toMatchObject({
      collapsedGroups: [],
    });
    store.getState().toggleSidebarGroupCollapsed(base.id, MULTI_KEY);
    await store.getState().waitForSidebarViewSync("ws");
    const resumed = reload();
    expect(resumed.getState().sidebarViewsByWorkspace.ws.draft).toMatchObject({
      collapsedGroups: [MULTI_KEY],
    });
    if (finish === "discard") resumed.getState().discardSidebarDraft();
    else if (finish === "save as") resumed.getState().saveSidebarDraftAs("Multi copy");
    else resumed.getState().saveSidebarDraftOverwrite();
    await resumed.getState().waitForSidebarViewSync("ws");
    current = reload().getState().sidebarViewsByWorkspace.ws;
    expect(current.draft).toBeNull();
    const selected = current.views.find((view) => view.id === current.activeViewId)!;
    expect(selected.group).toBe(finish === "discard" ? TAG : MULTI);
    expect(selected.collapsedGroups).toEqual(finish === "discard" ? [TAG_KEY] : [MULTI_KEY]);
    if (finish === "save as")
      expect(current.views.find((view) => view.id === base.id)?.collapsedGroups).toEqual([TAG_KEY]);
  },
);
