import { waitFor } from "@testing-library/react";
import { applyDraftMutationForTest, applyViewMutationsForTest } from "./sidebar-view-test-helpers";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import { workspaceId } from "@/lib/types/ids";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
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
const USER_ID = "default-user";

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
        user_id: USER_ID,
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
      applyViewMutationsForTest(views, patch);
      server = {
        ...server,
        views: [...views.values()],
        draft: applyDraftMutationForTest(views, server.active_view_id, server.draft, patch),
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

it("a queued discard cannot clear a newer different-group draft", async () => {
  const base = createDefaultSidebarView("base", "Base");
  let revision = 1;
  let server: SidebarWorkspaceStateApi = {
    views: [toApiSidebarView(base)],
    active_view_id: base.id,
    draft: toApiSidebarDraft({
      baseViewId: base.id,
      filters: [],
      sort: base.sort,
      group: MULTI,
      groupIndent: true,
      collapsedGroups: ["kept"],
    }),
  };
  const acknowledge = (): UserSettingsResponse => ({
    settings: {
      user_id: USER_ID,
      workspace_id: workspaceId("ws"),
      repository_ids: [],
      updated_at: "",
      revision,
      sidebar_views_by_workspace: { ws: structuredClone(server) },
    },
  });
  const store = createAppStore({
    workspaces: { items: [], activeId: "ws" },
    sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: server }, {}, revision),
  });
  store.setState((state) => ({ userSettings: { ...state.userSettings, revision } }));
  const first = Promise.withResolvers<UserSettingsResponse>();
  let initial: UserSettingsResponse | undefined;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    const patch = payload.sidebar_view_state!;
    const views = new Map(server.views.map((view) => [view.id, view]));
    applyViewMutationsForTest(views, patch);
    server = {
      ...server,
      views: [...views.values()],
      draft: applyDraftMutationForTest(views, server.active_view_id, server.draft, patch),
    };
    revision++;
    if (!initial) {
      initial = acknowledge();
      return first.promise;
    }
    return acknowledge();
  });
  store.getState().renameSidebarView(base.id, "Local rename");
  store.getState().discardSidebarDraft();
  server.draft = { ...server.draft!, group: "plugin:tags:other", collapsed_groups: ["remote-key"] };
  revision++;
  registerUsersHandlers(store)["user.settings.updated"]!({
    type: "notification",
    action: "user.settings.updated",
    payload: acknowledge().settings,
  });
  first.resolve(initial!);
  await store.getState().waitForSidebarViewSync("ws");
  expect(server.draft).toMatchObject({
    group: "plugin:tags:other",
    collapsed_groups: ["remote-key"],
  });
  expect(store.getState().sidebarViewsByWorkspace.ws.draft?.group).toBe("plugin:tags:other");
});

it.each([
  ["group", 1],
  ["group", 2],
  ["active view", 1],
  ["active view", 2],
] as const)(
  "settles a failed %s predecessor across %s no-op successors to server state",
  async (change, successors) => {
    const base = createDefaultSidebarView("base", "Base");
    const other = createDefaultSidebarView("other", "Other");
    const server: SidebarWorkspaceStateApi = {
      views: [base, other].map(toApiSidebarView),
      active_view_id: base.id,
      draft: null,
    };
    const ack = (): UserSettingsResponse => ({
      settings: {
        user_id: USER_ID,
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: 1,
        sidebar_views_by_workspace: { ws: structuredClone(server) },
      },
    });
    const store = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: server }, {}, 1),
    });
    const failed = Promise.withResolvers<UserSettingsResponse>();
    vi.mocked(updateUserSettings)
      .mockImplementation(async (payload) => {
        const views = new Map(server.views.map((view) => [view.id, view]));
        server.draft = applyDraftMutationForTest(
          views,
          server.active_view_id,
          server.draft,
          payload.sidebar_view_state!,
        );
        return ack();
      })
      .mockImplementationOnce(() => failed.promise);
    if (change === "group") store.getState().updateSidebarDraft({ group: MULTI });
    else store.getState().setSidebarActiveView(other.id);
    store.getState().updateSidebarDraft({ groupIndent: false });
    if (successors === 2) store.getState().updateSidebarDraft({ groupIndent: true });
    failed.reject(new ApiError("Predecessor was not committed", 500, {}));
    await store.getState().waitForSidebarViewSync("ws");
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(server.active_view_id);
    expect(store.getState().sidebarViewsByWorkspace.ws.draft).toEqual(
      mapSidebarWorkspaces({ ws: server }).ws.draft,
    );
    expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBeTruthy();
  },
);

it.each(
  (
    [
      ["selection", 400],
      ["selection", 500],
      ["save", 400],
      ["save", 500],
      ["rename", 400],
      ["rename", 500],
      ["move", 400],
      ["move", 409],
    ] as const
  ).flatMap(([transition, status]) => [false, true].map((skip) => ({ transition, status, skip }))),
)(
  "retains failed $transition status $status through compaction skipped=$skip",
  async ({ transition, status, skip }) => {
    const base = {
      ...createDefaultSidebarView("base", "Tag"),
      group: "plugin:tags:tags" as const,
      collapsedGroups: ["kept", "retired"],
    };
    let server: SidebarWorkspaceStateApi = {
      views: [toApiSidebarView(base), toApiSidebarView(createDefaultSidebarView("other", "Other"))],
      active_view_id: base.id,
      draft:
        transition === "save"
          ? toApiSidebarDraft({
              baseViewId: base.id,
              filters: [],
              sort: { ...base.sort, direction: "desc" },
              group: base.group,
              groupIndent: true,
              collapsedGroups: [],
            })
          : null,
    };
    const store = createAppStore({
      workspaces: { items: [], activeId: "ws" },
      sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: server }, {}, 1),
    });
    const first = Promise.withResolvers<UserSettingsResponse>();
    vi.mocked(updateUserSettings)
      .mockImplementation(async (payload) => {
        const views = new Map(server.views.map((view) => [view.id, view]));
        applyViewMutationsForTest(views, payload.sidebar_view_state!);
        server = { ...server, views: [...views.values()] };
        return {
          settings: {
            user_id: USER_ID,
            workspace_id: workspaceId("ws"),
            repository_ids: [],
            updated_at: "",
            revision: 2,
            sidebar_views_by_workspace: { ws: structuredClone(server) },
          },
        };
      })
      .mockImplementationOnce(() => first.promise);
    if (transition === "selection")
      store.getState().selectSidebarPluginGrouping({ reference: MULTI, label: "Multi-tag" });
    else if (transition === "save") store.getState().saveSidebarDraftOverwrite();
    else if (transition === "rename") store.getState().renameSidebarView(base.id, "Renamed");
    else store.getState().reorderSidebarViews(base.id, "other");
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
    let current = true;
    const compacted = store
      .getState()
      .reconcileSidebarGroupDisclosure(base.id, base.group, base.collapsedGroups, ["kept"], {
        workspaceId: "ws",
        isCurrent: () => current,
      });
    if (skip) {
      registerUsersHandlers(store)["user.settings.updated"]!({
        type: "notification",
        action: "user.settings.updated",
        payload: {
          user_id: USER_ID,
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: 2,
          sidebar_views_by_workspace: { ws: structuredClone(server) },
        },
      });
      current = false;
    }
    first.reject(new ApiError("Transition refused", status, {}));
    await compacted;
    await store.getState().waitForSidebarViewSync("ws");
    const state = store.getState().sidebarViewsByWorkspace.ws;
    expect(state.activeViewId).toBe(base.id);
    expect(state.views[0].collapsedGroups).toEqual(skip ? base.collapsedGroups : ["kept"]);
    expect(state.draft).toEqual(mapSidebarWorkspaces({ ws: server }).ws.draft);
    expect(state.transitionPending).toBe(false);
    if (transition === "rename" && !skip && status !== 400) {
      expect(state.views[0].name).toBe("Renamed");
      expect(state.syncError).toBeNull();
    } else {
      if (transition === "rename" && !skip && status === 400)
        expect(state.views[0].name).toBe("Tag");
      expect(state.syncError).toBeTruthy();
    }
  },
);
