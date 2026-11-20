import {
  applyDraftMutationForTest,
  applyDraftSaveForTest,
  applyViewMutationsForTest,
} from "./sidebar-view-test-helpers";
import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import type {
  SidebarWorkspaceStateApi,
  UserSettingsResponse,
} from "@/lib/types/http-user-settings";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { toApiSidebarView } from "./sidebar-view-wire";
import { workspaceId } from "@/lib/types/ids";

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
    applyViewMutationsForTest(views, patch);
    server = {
      views: (patch.view_order ?? [...views.keys()]).map((id) => views.get(id)!),
      active_view_id: patch.active_view_id ?? server.active_view_id,
      draft: applyDraftMutationForTest(
        views,
        patch.active_view_id ?? server.active_view_id,
        server.draft,
        patch,
      ),
    };
    server = applyDraftSaveForTest(server, patch);
    return {
      settings: {
        user_id: "default-user",
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: ++revision,
        sidebar_views_by_workspace: { ws: server },
      },
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
    else if (finish === "save as")
      resumed.getState().saveSidebarDraftAs("Multi copy", { baseViewId: base.id, group: MULTI });
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

it.each([
  { copy: false, disclosure: false },
  { copy: true, disclosure: false },
  { copy: false, disclosure: true },
  { copy: true, disclosure: true },
])(
  "does not admit Save behind an unacknowledged edit: $copy/$disclosure",
  async ({ copy, disclosure }) => {
    const { reload, base } = draftWorkspace();
    const store = reload();
    store.getState().updateSidebarDraft({ group: MULTI });
    await store.getState().waitForSidebarViewSync("ws");
    const failed = Promise.withResolvers<Awaited<ReturnType<typeof updateUserSettings>>>();
    vi.mocked(updateUserSettings).mockImplementationOnce(() => failed.promise);
    if (disclosure) store.getState().toggleSidebarGroupCollapsed(base.id, MULTI_KEY);
    else store.getState().updateSidebarDraft({ sort: { key: "title", direction: "asc" } });
    try {
      if (copy) store.getState().saveSidebarDraftAs("Copy", { baseViewId: base.id, group: MULTI });
      else store.getState().saveSidebarDraftOverwrite();
      expect(store.getState().sidebarViewsByWorkspace.ws.draft).not.toBeNull();
    } finally {
      failed.reject(new ApiError("Edit rejected before commit", 503, {}));
      await store.getState().waitForSidebarViewSync("ws");
    }
    const persisted = reload().getState().sidebarViewsByWorkspace.ws;
    expect(persisted.draft).not.toBeNull();
    expect(persisted.views.find((view) => view.id === persisted.activeViewId)?.group).toBe(TAG);
    expect(store.getState().sidebarViewsByWorkspace.ws.syncError).not.toBeNull();
  },
);

it.each([false, true])("Save as rejects a replaced naming owner: replaced=%s", async (replaced) => {
  const { reload } = draftWorkspace();
  const store = reload();
  store.getState().updateSidebarDraft({ group: MULTI });
  await store.getState().waitForSidebarViewSync("ws");
  const owner = { baseViewId: "base", group: MULTI } as const;
  if (replaced) store.getState().updateSidebarDraft({ group: "state" });
  else store.getState().toggleSidebarGroupCollapsed("base", MULTI_KEY);
  await store.getState().waitForSidebarViewSync("ws");
  await store.getState().saveSidebarDraftAs("Named copy", owner);
  const persisted = reload().getState().sidebarViewsByWorkspace.ws;
  if (replaced) {
    expect(persisted.views.map((view) => view.name)).toEqual(["Tag"]);
    expect(persisted.draft?.group).toBe("state");
  } else {
    expect(persisted.views.find((view) => view.name === "Named copy")?.collapsedGroups).toEqual([
      MULTI_KEY,
    ]);
    expect(persisted.draft).toBeNull();
  }
});
