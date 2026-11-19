import { waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { create, type StateCreator } from "zustand";
import { immer } from "zustand/middleware/immer";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { createUISlice } from "./ui-slice";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import type { UISlice } from "./types";
import type { UserSettingsResponse, SidebarViewApi } from "@/lib/types/http-user-settings";
import { toApiSidebarView } from "./sidebar-view-wire";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { ApiError } from "@/lib/api/client";

vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: vi.fn() }));
beforeEach(() => {
  window.localStorage.clear();
  vi.mocked(updateUserSettings).mockReset();
});

function makeStore() {
  // This partial store supplies the UI slice and the workspace used by its actions.
  const createTestSlice = createUISlice as unknown as StateCreator<
    UISlice,
    [["zustand/immer", never]],
    [],
    UISlice
  >;
  return create<UISlice>()(
    immer((...args) => ({ ...createTestSlice(...args), workspaces: { activeId: "ws" } })),
  );
}

it("copies and edits large disclosures without resending the collection", async () => {
  const keys = Array.from(
    { length: 1000 },
    (_, i) => `plugin-group:${i.toString(16).padStart(64, "0")}`,
  );
  const views = Array.from({ length: 27 }, (_, i) => ({
    ...createDefaultSidebarView(`copy-${i}`, `Copy ${i}`),
    group: "plugin:tags:tags" as const,
    collapsedGroups: [...keys],
  }));
  const store = makeStore();
  store.setState({
    sidebarViewsByWorkspace: {
      ws: { views, activeViewId: "copy-0", draft: null, syncError: null },
    },
  });
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    if (new TextEncoder().encode(JSON.stringify(payload)).byteLength > 2 * 1024 * 1024)
      throw new Error("HTTP 413");
    return { settings: {} } as UserSettingsResponse;
  });
  store.getState().duplicateSidebarView("copy-0", "Independent copy");
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  const copied = store
    .getState()
    .sidebarViewsByWorkspace.ws.views.find((view) => view.name === "Independent copy");
  expect(copied).toBeDefined();
  store.getState().toggleSidebarGroupCollapsed(copied!.id, keys[0]);
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  const saved = store.getState().sidebarViewsByWorkspace.ws;
  expect(saved.syncError).toBeNull();
  expect(saved.views.find((view) => view.id === "copy-0")?.collapsedGroups).toEqual(keys);
  expect(saved.views.find((view) => view.id === copied!.id)?.collapsedGroups).toEqual(
    keys.slice(1),
  );
});

it("a queued successor persists earlier unsaved intent against the acknowledged snapshot", async () => {
  const store = makeStore();
  const base = createDefaultSidebarView("base", "Base");
  store.setState({
    sidebarViewsByWorkspace: {
      ws: { views: [base], activeViewId: base.id, draft: null, syncError: null },
    },
  });
  const persisted = new Map<string, SidebarViewApi>([[base.id, toApiSidebarView(base)]]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(first.promise)
    .mockImplementation(async (payload) => {
      const patch = payload.sidebar_view_state!;
      for (const view of patch.views_upsert ?? []) persisted.set(view.id, view);
      for (const view of patch.views_update ?? [])
        if (persisted.has(view.id)) persisted.set(view.id, view);
      const order = patch.view_order ?? [...persisted.keys()];
      const views = order.map((id) => {
        const view = persisted.get(id);
        if (!view) throw new Error("view order refers to an unpersisted view");
        return view;
      });
      return {
        settings: {
          revision: 1,
          sidebar_views_by_workspace: {
            ws: { views, active_view_id: patch.active_view_id!, draft: null },
          },
        },
      } as UserSettingsResponse;
    });
  const created = store.getState().createSidebarView()!;
  store.getState().renameSidebarView("base", "Renamed");
  first.reject(new ApiError("first write failed", 500, {}));
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  expect(persisted.get("base")?.name).toBe("Renamed");
  expect(persisted.has(created)).toBe(true);
  expect(store.getState().sidebarViewsByWorkspace.ws.views.map((view) => view.id)).toEqual([
    "base",
    created,
  ]);
});

it.each(["add", "delete"])(
  "rebases queued local edits without undoing a remote view %s",
  async (remoteChange) => {
    const store = makeStore();
    const base = createDefaultSidebarView("base", "Base");
    const remote = createDefaultSidebarView("remote", "Remote");
    const created = createDefaultSidebarView("created-remotely", "Created elsewhere");
    store.setState({
      sidebarViewsByWorkspace: {
        ws: { views: [base, remote], activeViewId: base.id, draft: null, syncError: null },
      },
    });
    const persisted = new Map([base, remote].map((view) => [view.id, toApiSidebarView(view)]));
    let active = base.id;
    let revision = 1;
    const acknowledge = () =>
      ({
        settings: {
          revision: revision++,
          sidebar_views_by_workspace: {
            ws: { views: [...persisted.values()], active_view_id: active, draft: null },
          },
        },
      }) as UserSettingsResponse;
    const apply = (payload: Parameters<typeof updateUserSettings>[0]) => {
      const patch = payload.sidebar_view_state!;
      for (const id of patch.view_ids_delete ?? []) persisted.delete(id);
      for (const view of patch.views_upsert ?? []) persisted.set(view.id, view);
      for (const view of patch.views_update ?? [])
        if (persisted.has(view.id)) persisted.set(view.id, view);
      if (patch.active_view_id) active = patch.active_view_id;
      return acknowledge();
    };
    const first = Promise.withResolvers<UserSettingsResponse>();
    vi.mocked(updateUserSettings)
      .mockReturnValueOnce(first.promise)
      .mockImplementation(async (payload) => apply(payload));
    store.getState().renameSidebarView(base.id, "Locally renamed");
    store.getState().toggleSidebarGroupCollapsed(base.id, "workflow:local");
    if (remoteChange === "add") {
      persisted.set(created.id, toApiSidebarView(created));
      active = created.id;
    } else persisted.delete(remote.id);
    first.resolve(apply(vi.mocked(updateUserSettings).mock.calls[0][0]));
    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false),
    );
    expect(persisted.has(created.id)).toBe(remoteChange === "add");
    expect(persisted.has(remote.id)).toBe(remoteChange !== "delete");
    expect(persisted.get(base.id)).toMatchObject({
      name: "Locally renamed",
      collapsed_groups: ["workflow:local"],
    });
    if (remoteChange === "add") expect(active).toBe(created.id);
    const reloaded = makeStore();
    reloaded.setState({
      sidebarViewsByWorkspace: mapSidebarWorkspaces(
        acknowledge().settings.sidebar_views_by_workspace,
      ),
    });
    expect(reloaded.getState().sidebarViewsByWorkspace.ws.views.map((view) => view.id)).toEqual([
      ...persisted.keys(),
    ]);
    expect(reloaded.getState().sidebarViewsByWorkspace.ws.views[0]).toMatchObject({
      name: "Locally renamed",
      collapsedGroups: ["workflow:local"],
    });
  },
);

it("does not replay a discarded failed creation on a later unrelated edit", async () => {
  const store = makeStore();
  const base = createDefaultSidebarView("base", "Base");
  store.setState({
    sidebarViewsByWorkspace: {
      ws: { views: [base], activeViewId: base.id, draft: null, syncError: null },
    },
  });
  const persisted = new Map([[base.id, toApiSidebarView(base)]]);
  const first = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(first.promise)
    .mockImplementation(async (payload) => {
      for (const view of payload.sidebar_view_state?.views_upsert ?? [])
        persisted.set(view.id, view);
      for (const view of payload.sidebar_view_state?.views_update ?? [])
        if (persisted.has(view.id)) persisted.set(view.id, view);
      return {
        settings: {
          sidebar_views_by_workspace: {
            ws: { views: [...persisted.values()], active_view_id: base.id, draft: null },
          },
        },
      } as UserSettingsResponse;
    });
  const failed = store.getState().createSidebarView()!;
  store.getState().setSidebarActiveView(base.id);
  first.reject(new ApiError("creation refused", 500, {}));
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  expect(persisted.has(failed)).toBe(false);
  store.getState().renameSidebarView(base.id, "After rollback");
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  expect(persisted.has(failed)).toBe(false);
  expect(persisted.get(base.id)?.name).toBe("After rollback");
});
