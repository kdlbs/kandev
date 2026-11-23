import { waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { create, type StateCreator } from "zustand";
import { immer } from "zustand/middleware/immer";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { createUISlice } from "./ui-slice";
import { createDefaultSidebarView } from "./sidebar-view-builtins";
import type { UISlice } from "./types";
import type { UserSettingsResponse, SidebarViewApi } from "@/lib/types/http-user-settings";
import { toApiSidebarView } from "./sidebar-view-wire";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import { ApiError } from "@/lib/api/client";

import { applyViewMutationsForTest } from "./sidebar-view-test-helpers";
import { workspaceId } from "@/lib/types/ids";

const USER_ID = "default-user";
vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
}));
beforeEach(() => {
  window.localStorage.clear();
  vi.mocked(fetchUserSettings).mockReset();
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

function mockCurrentSettings(persisted: Map<string, SidebarViewApi>, activeViewId: () => string) {
  vi.mocked(fetchUserSettings).mockImplementation(
    async () =>
      ({
        settings: {
          user_id: USER_ID,
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: 1,
          sidebar_views_by_workspace: {
            ws: { views: [...persisted.values()], active_view_id: activeViewId(), draft: null },
          },
        },
      }) as UserSettingsResponse,
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
  let activeViewId = base.id;
  const first = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(first.promise)
    .mockImplementation(async (payload) => {
      const patch = payload.sidebar_view_state!;
      applyViewMutationsForTest(persisted, patch);
      const order = patch.view_order ?? [...persisted.keys()];
      const views = order.map((id) => {
        const view = persisted.get(id);
        if (!view) throw new Error("view order refers to an unpersisted view");
        return view;
      });
      return {
        settings: {
          user_id: USER_ID,
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: 1,
          sidebar_views_by_workspace: {
            ws: { views, active_view_id: patch.active_view_id ?? activeViewId, draft: null },
          },
        },
      } as UserSettingsResponse;
    });
  const created = store.getState().createSidebarView()!;
  store.getState().renameSidebarView("base", "Renamed");
  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  const createPatch = vi.mocked(updateUserSettings).mock.calls[0][0].sidebar_view_state!;
  applyViewMutationsForTest(persisted, createPatch);
  activeViewId = createPatch.active_view_id ?? activeViewId;
  mockCurrentSettings(persisted, () => activeViewId);
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
          user_id: USER_ID,
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: revision++,
          sidebar_views_by_workspace: {
            ws: { views: [...persisted.values()], active_view_id: active, draft: null },
          },
        },
      }) as UserSettingsResponse;
    const apply = (payload: Parameters<typeof updateUserSettings>[0]) => {
      const patch = payload.sidebar_view_state!;
      applyViewMutationsForTest(persisted, patch);
      if (patch.active_view_id) active = patch.active_view_id;
      return acknowledge();
    };
    const first = Promise.withResolvers<UserSettingsResponse>();
    vi.mocked(updateUserSettings)
      .mockReturnValueOnce(first.promise)
      .mockImplementation(async (payload) => apply(payload));
    store.getState().renameSidebarView(base.id, "Locally renamed");
    store.getState().toggleSidebarGroupCollapsed(base.id, "workflow:local");
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
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
  const activeViewId = base.id;
  mockCurrentSettings(persisted, () => activeViewId);
  const first = Promise.withResolvers<UserSettingsResponse>();
  vi.mocked(updateUserSettings)
    .mockReturnValueOnce(first.promise)
    .mockImplementation(async (payload) => {
      applyViewMutationsForTest(persisted, payload.sidebar_view_state!);
      return {
        settings: {
          user_id: USER_ID,
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          sidebar_views_by_workspace: {
            ws: { views: [...persisted.values()], active_view_id: base.id, draft: null },
          },
        },
      } as UserSettingsResponse;
    });
  const failed = store.getState().createSidebarView()!;
  store.getState().setSidebarActiveView(base.id);
  await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));
  first.reject(new ApiError("creation refused", 500, {}));
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  expect(persisted.has(failed)).toBe(false);
  store.getState().renameSidebarView(base.id, "After rollback");
  await waitFor(() => expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false));
  expect(persisted.has(failed)).toBe(false);
  expect(persisted.get(base.id)?.name).toBe("After rollback");
});

it("a successor keeps edits when an already committed creation loses its acknowledgement", async () => {
  const base = createDefaultSidebarView("base", "Base");
  const store = makeStore();
  store.setState({
    sidebarViewsByWorkspace: {
      ws: { views: [base], activeViewId: base.id, draft: null, syncError: null },
    },
  });
  const persisted = new Map([[base.id, toApiSidebarView(base)]]);
  let activeViewId = base.id;
  mockCurrentSettings(persisted, () => activeViewId);
  const first = Promise.withResolvers<UserSettingsResponse>();
  let writes = 0;
  vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
    const patch = payload.sidebar_view_state!;
    applyViewMutationsForTest(persisted, patch);
    activeViewId = patch.active_view_id ?? activeViewId;
    if (++writes === 1) return first.promise;
    return {
      settings: {
        user_id: USER_ID,
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: writes,
        sidebar_views_by_workspace: {
          ws: { views: [...persisted.values()], active_view_id: activeViewId, draft: null },
        },
      },
    };
  });
  const created = store.getState().createSidebarView()!;
  await waitFor(() => expect(writes).toBe(1));
  persisted.set(created, { ...persisted.get(created)!, name: "Remote rename" });
  store.getState().toggleSidebarGroupCollapsed(created, "kept");
  first.reject(new ApiError("Proxy failed after commit", 503, {}));
  await store.getState().waitForSidebarViewSync("ws");
  expect(persisted.get(created)).toMatchObject({
    name: "Remote rename",
    collapsed_groups: ["kept"],
  });
});
