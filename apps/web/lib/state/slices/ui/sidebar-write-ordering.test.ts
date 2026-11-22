import { waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import { workspaceId } from "@/lib/types/ids";
import { registerUsersHandlers } from "@/lib/ws/handlers/users";
import type {
  SidebarWorkspaceStateApi,
  UserSettingsResponse,
  UserSettingsUpdatePayload,
} from "@/lib/types/http-user-settings";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { toApiSidebarView } from "./sidebar-view-wire";
import { applyViewMutationsForTest } from "./sidebar-view-test-helpers";

const SETTINGS_UPDATED = "user.settings.updated";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
beforeEach(() => {
  vi.mocked(updateUserSettings).mockReset();
});
const USER = "default-user";
const GROUP = "plugin:tags:tags";

function fixture(extraIds: string[] = []) {
  const base = {
    ...createDefaultSidebarView("base", "A"),
    group: GROUP as `plugin:${string}:${string}`,
  };
  const other = createDefaultSidebarView("other", "Other");
  let state: SidebarWorkspaceStateApi = {
    views: [base, other, ...extraIds.map((id) => createDefaultSidebarView(id, id))].map(
      toApiSidebarView,
    ),
    active_view_id: base.id,
    draft: null,
  };
  let revision = 1;
  const acknowledge = (): UserSettingsResponse => ({
    settings: {
      user_id: USER,
      workspace_id: workspaceId("ws"),
      repository_ids: [],
      updated_at: "",
      revision,
      sidebar_views_by_workspace: { ws: structuredClone(state) },
    },
  });
  const store = createAppStore({
    workspaces: { items: [], activeId: "ws" },
    sidebarViewsByWorkspace: mapSidebarWorkspaces({ ws: state }, {}, revision),
  });
  store.setState((current) => ({ userSettings: { ...current.userSettings, revision } }));
  const apply = (payload: UserSettingsUpdatePayload) => {
    const patch = payload.sidebar_view_state!;
    const views = new Map(state.views.map((view) => [view.id, view]));
    applyViewMutationsForTest(views, patch);
    const order = patch.view_order ?? [...views.keys()];
    if (order.length !== views.size || order.some((id) => !views.has(id)))
      throw new ApiError("Collection changed after dispatch", 400, {});
    state = {
      ...state,
      views: order.map((id) => views.get(id)!),
      active_view_id: patch.active_view_id ?? state.active_view_id,
    };
    revision++;
    return acknowledge();
  };
  return {
    base,
    other,
    store,
    apply,
    acknowledge,
    current: () => state,
    reorder: (ids: string[]) => {
      state = { ...state, views: ids.map((id) => state.views.find((view) => view.id === id)!) };
      revision++;
    },
    append: () => {
      const remote = toApiSidebarView(createDefaultSidebarView("remote", "Remote"));
      state = { ...state, views: [...state.views, remote] };
      revision++;
    },
  };
}

it.each([
  { delivered: false, field: "disclosure" },
  { delivered: true, field: "disclosure" },
  { delivered: false, field: "name" },
  { delivered: true, field: "name" },
])(
  "inverse $field intent survives an uncertain committed ACK: WS=$delivered",
  async ({ delivered, field }) => {
    const f = fixture();
    const first = Promise.withResolvers<UserSettingsResponse>();
    let committed: UserSettingsResponse | undefined;
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      const response = f.apply(payload);
      if (!committed) {
        committed = response;
        return first.promise;
      }
      return response;
    });
    if (field === "disclosure") f.store.getState().toggleSidebarGroupCollapsed(f.base.id, "kept");
    else f.store.getState().renameSidebarView(f.base.id, "B");
    await waitFor(() => expect(committed).toBeDefined());
    if (field === "disclosure") f.store.getState().toggleSidebarGroupCollapsed(f.base.id, "kept");
    else f.store.getState().renameSidebarView(f.base.id, "A");
    if (delivered)
      registerUsersHandlers(f.store)[SETTINGS_UPDATED]!({
        type: "notification",
        action: SETTINGS_UPDATED,
        payload: committed!.settings,
      });
    first.reject(new ApiError("Proxy lost committed ACK", 503, {}));
    await f.store.getState().waitForSidebarViewSync("ws");
    const reloaded = mapSidebarWorkspaces({ ws: f.current() });
    expect(reloaded.ws.views.find((view) => view.id === f.base.id)).toMatchObject({
      name: "A",
      collapsedGroups: [],
    });
    expect(f.store.getState().sidebarViewsByWorkspace.ws.syncError).toBeNull();
  },
);

it("queued reorder uses the acknowledged remote append without replacing field intent", async () => {
  const f = fixture();
  const first = Promise.withResolvers<UserSettingsResponse>();
  let predecessor: UserSettingsUpdatePayload | undefined;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    if (!predecessor) {
      predecessor = payload;
      return first.promise;
    }
    return f.apply(payload);
  });
  f.store.getState().renameSidebarView(f.base.id, "Local rename");
  await waitFor(() => expect(predecessor).toBeDefined());
  f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
  f.append();
  first.resolve(f.apply(predecessor!));
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(f.store.getState().sidebarViewsByWorkspace.ws.syncError).toBeNull();
  const reloaded = mapSidebarWorkspaces({ ws: f.current() }).ws;
  expect(reloaded.views.map((view) => view.id)).toEqual([f.other.id, f.base.id, "remote"]);
  expect(reloaded.views.find((view) => view.id === f.base.id)?.name).toBe("Local rename");
});

it("pending provider selection rejects dependent actions until the authoritative view arrives", async () => {
  const f = fixture();
  const first = Promise.withResolvers<UserSettingsResponse>();
  const selected = {
    ...createDefaultSidebarView("selected", "Selected"),
    group: GROUP as `plugin:${string}:${string}`,
  };
  const response = f.acknowledge();
  response.settings.revision = 2;
  response.settings.sidebar_views_by_workspace = {
    ws: {
      views: [...f.current().views, toApiSidebarView(selected)],
      active_view_id: selected.id,
      draft: null,
    },
  };
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(first.promise)
    .mockImplementation(async (payload) => f.apply(payload));
  const pending = f.store
    .getState()
    .selectSidebarPluginGrouping({ reference: GROUP, label: "Tag" });
  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  try {
    f.store.getState().setSidebarActiveView(f.other.id);
    f.store.getState().updateSidebarDraft({ group: "state" });
    expect(f.store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(f.base.id);
    expect(f.store.getState().sidebarViewsByWorkspace.ws.draft).toBeNull();
    expect(f.store.getState().sidebarViewsByWorkspace.ws.transitionPending).toBe(true);
    first.resolve(response);
    await pending;
    expect(f.store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(selected.id);
    expect(f.store.getState().sidebarViewsByWorkspace.ws.transitionPending).toBe(false);
  } finally {
    first.resolve(response);
    await pending;
    await f.store.getState().waitForSidebarViewSync("ws");
  }
});

it("a queued drag preserves an acknowledged disjoint remote drag", async () => {
  const f = fixture(["c", "d"]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  let predecessor: UserSettingsUpdatePayload | undefined;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    if (!predecessor) {
      predecessor = payload;
      return first.promise;
    }
    return f.apply(payload);
  });
  f.store.getState().renameSidebarView(f.base.id, "Renamed");
  await waitFor(() => expect(predecessor).toBeDefined());
  f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
  f.reorder([f.base.id, f.other.id, "d", "c"]);
  first.resolve(f.apply(predecessor!));
  await f.store.getState().waitForSidebarViewSync("ws");
  const persisted = mapSidebarWorkspaces({ ws: f.current() }).ws;
  expect(persisted.views.map((view) => view.id)).toEqual([f.other.id, f.base.id, "d", "c"]);
  expect(persisted.views.find((view) => view.id === f.base.id)?.name).toBe("Renamed");
});

it("a queued drag reports a removed destination without restoring it", async () => {
  const f = fixture(["c", "d"]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  let predecessor: UserSettingsUpdatePayload | undefined;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    if (!predecessor) {
      predecessor = payload;
      return first.promise;
    }
    return f.apply(payload);
  });
  f.store.getState().renameSidebarView(f.base.id, "Renamed");
  await waitFor(() => expect(predecessor).toBeDefined());
  f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
  f.reorder([f.base.id, "c", "d"]);
  first.resolve(f.apply(predecessor!));
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(f.store.getState().sidebarViewsByWorkspace.ws.syncError).not.toBeNull();
  expect(f.current().views.map((view) => view.id)).toEqual([f.base.id, "c", "d"]);
  expect(f.current().views[0].name).toBe("Renamed");
});

it("a retired queued drag does not abort the following independent rename", async () => {
  const f = fixture(["c", "d"]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  let predecessor: UserSettingsUpdatePayload | undefined;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    if (!predecessor) {
      predecessor = payload;
      return first.promise;
    }
    return f.apply(payload);
  });
  f.store.getState().renameSidebarView(f.base.id, "Renamed");
  await waitFor(() => expect(predecessor).toBeDefined());
  f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
  f.store.getState().renameSidebarView("c", "Independent");
  f.reorder([f.base.id, "c", "d"]);
  first.resolve(f.apply(predecessor!));
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(f.current().views.map((view) => view.id)).toEqual([f.base.id, "c", "d"]);
  expect(f.current().views.find((view) => view.id === "c")?.name).toBe("Independent");
  expect(mapSidebarWorkspaces({ ws: f.current() }).ws.views[0].name).toBe("Renamed");
});

it("an uncertain drag with a now-deleted endpoint is not replayed by its successor", async () => {
  const f = fixture(["c", "d"]);
  const failed = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings).mockImplementationOnce(() => failed.promise);
  vi.mocked(updateUserSettings).mockImplementation((payload) => Promise.resolve(f.apply(payload)));
  f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  f.store.getState().renameSidebarView("c", "Independent");
  f.reorder([f.base.id, "c", "d"]);
  registerUsersHandlers(f.store)[SETTINGS_UPDATED]!({
    type: "notification",
    action: SETTINGS_UPDATED,
    payload: f.acknowledge().settings,
  });
  failed.reject(new ApiError("Move response was lost", 503, {}));
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(f.current().views.map((view) => view.id)).toEqual([f.base.id, "c", "d"]);
  expect(f.current().views.find((view) => view.id === "c")?.name).toBe("Independent");
  expect(f.store.getState().sidebarViewsByWorkspace.ws.syncError).toBeTruthy();
});

it.each([400, 409, 408, 429, 503])(
  "queued rename cannot revive a definitively rejected move: status=%s",
  async (status) => {
    const f = fixture(["c", "d"]);
    const failed = Promise.withResolvers<UserSettingsResponse>();
    vi.mocked(updateUserSettings).mockImplementation((payload) =>
      Promise.resolve(f.apply(payload)),
    );
    vi.mocked(updateUserSettings).mockImplementationOnce(() => failed.promise);
    f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
    f.store.getState().renameSidebarView("c", "Independent");
    const remoteOrder = [f.base.id, "c", "d", f.other.id];
    f.reorder(remoteOrder);
    registerUsersHandlers(f.store)[SETTINGS_UPDATED]!({
      type: "notification",
      action: SETTINGS_UPDATED,
      payload: f.acknowledge().settings,
    });
    failed.reject(new ApiError("Order changed after dispatch", status, {}));
    await f.store.getState().waitForSidebarViewSync("ws");
    const persisted = mapSidebarWorkspaces({ ws: f.current() }).ws;
    expect(persisted.views.map((view) => view.id)).toEqual(
      status === 400 || status === 409 ? remoteOrder : ["c", "d", f.other.id, f.base.id],
    );
    expect(persisted.views.find((view) => view.id === "c")?.name).toBe("Independent");
  },
);

it("a definite rejection retires an older uncertain move before the next sparse successor", async () => {
  const f = fixture(["c", "d"]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  const second = Promise.withResolvers<UserSettingsResponse>();
  const arrived = Promise.withResolvers<void>();
  vi.mocked(updateUserSettings)
    .mockImplementation((payload) => Promise.resolve(f.apply(payload)))
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => {
      arrived.resolve();
      return second.promise;
    });
  f.store.getState().reorderSidebarViews(f.base.id, f.other.id);
  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  f.store.getState().renameSidebarView("c", "Independent C");
  f.store.getState().renameSidebarView("d", "Independent D");
  const remoteOrder = [f.base.id, "c", "d", f.other.id];
  f.reorder(remoteOrder);
  registerUsersHandlers(f.store)[SETTINGS_UPDATED]!({
    type: "notification",
    action: SETTINGS_UPDATED,
    payload: f.acknowledge().settings,
  });
  first.reject(new ApiError("Move response was lost", 503, {}));
  await arrived.promise;
  second.reject(new ApiError("Replayed order was rejected", 400, {}));
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(f.current().views.map((view) => view.id)).toEqual(remoteOrder);
  expect(f.current().views.find((view) => view.id === "c")?.name).toBe("Independent C");
  expect(f.current().views.find((view) => view.id === "d")?.name).toBe("Independent D");
});

it("an intermediate local ACK does not retire intent that a later view write heals", async () => {
  const f = fixture(["c"]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings)
    .mockImplementation((payload) => Promise.resolve(f.apply(payload)))
    .mockImplementationOnce(() => first.promise);
  f.store.getState().renameSidebarView(f.base.id, "Recovered");
  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  f.store.getState().setSidebarActiveView(f.other.id);
  f.store.getState().renameSidebarView("c", "Independent");
  first.reject(new ApiError("Rename was not committed", 500, {}));
  await f.store.getState().waitForSidebarViewSync("ws");
  expect(f.current().views.find((view) => view.id === f.base.id)?.name).toBe("Recovered");
  expect(f.current().views.find((view) => view.id === "c")?.name).toBe("Independent");
  expect(f.store.getState().sidebarViewsByWorkspace.ws.syncError).toBeNull();
});
