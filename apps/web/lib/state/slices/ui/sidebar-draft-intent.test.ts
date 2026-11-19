import { beforeEach, expect, it, vi } from "vitest";
import { workspaceId } from "@/lib/types/ids";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type {
  SidebarWorkspaceStateApi,
  UserSettingsResponse,
} from "@/lib/types/http-user-settings";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { toApiSidebarDraft, toApiSidebarView } from "./sidebar-view-wire";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
});
const MULTI = "plugin:tags:multi-tags";

it.each(["field edit after compaction", "compaction after acknowledged field edit"])(
  "merges independent draft intent: %s",
  async (schedule) => {
    const base = { ...createDefaultSidebarView("base", "Tag"), group: "plugin:tags:tags" as const };
    const keys = ["kept", "retired"];
    let server: SidebarWorkspaceStateApi = {
      views: [toApiSidebarView(base)],
      active_view_id: base.id,
      draft: toApiSidebarDraft({
        baseViewId: base.id,
        filters: [],
        sort: base.sort,
        group: MULTI,
        groupIndent: true,
        collapsedGroups: keys,
      }),
    };
    let revision = 1;
    const acknowledge = (): UserSettingsResponse => ({
      settings: {
        user_id: "default-user",
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: revision++,
        sidebar_views_by_workspace: { ws: structuredClone(server) },
      },
    });
    const store = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: server }),
    });
    const first = Promise.withResolvers<UserSettingsResponse>();
    let writes = 0;
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      const patch = payload.sidebar_view_state!;
      const views = new Map(server.views.map((view) => [view.id, view]));
      for (const view of patch.views_upsert ?? []) views.set(view.id, view);
      for (const view of patch.views_update ?? []) if (views.has(view.id)) views.set(view.id, view);
      server = {
        ...server,
        views: [...views.values()],
        ...("draft" in patch ? { draft: patch.draft! } : {}),
      };
      if (++writes === 1) return first.promise;
      return acknowledge();
    });
    if (schedule === "field edit after compaction") {
      const compacted = store
        .getState()
        .reconcileSidebarGroupDisclosure(base.id, MULTI, keys, ["kept"], { workspaceId: "ws" });
      store.getState().updateSidebarDraft({ groupIndent: false });
      first.resolve(acknowledge());
      await compacted;
    } else {
      store.getState().renameSidebarView(base.id, "Renamed");
      const compacted = store
        .getState()
        .reconcileSidebarGroupDisclosure(base.id, MULTI, keys, ["kept"], { workspaceId: "ws" });
      server.draft = { ...server.draft!, group_indent: false };
      first.resolve(acknowledge());
      await compacted;
    }
    await store.getState().waitForSidebarViewSync("ws");
    expect(server.draft).toMatchObject({ group_indent: false, collapsed_groups: ["kept"] });
    const reloaded = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: server }),
    });
    expect(reloaded.getState().sidebarViewsByWorkspace.ws.draft).toMatchObject({
      groupIndent: false,
      collapsedGroups: ["kept"],
    });
  },
);
