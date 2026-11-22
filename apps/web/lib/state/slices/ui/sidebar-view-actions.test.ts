import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { create, type StoreApi, type UseBoundStore } from "zustand";
import { immer } from "zustand/middleware/immer";
import { ApiError } from "@/lib/api/client";
import { fetchUserSettings, updateUserSettings } from "@/lib/api/domains/settings-api";
import { DEFAULT_VIEW } from "./sidebar-view-builtins";
import { createUISlice } from "./ui-slice";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import type { UISlice } from "./types";
import { workspaceId } from "@/lib/types/ids";
import { toApiSidebarView } from "./sidebar-view-wire";
import { mapSidebarWorkspaces } from "./sidebar-workspace-state";
import {
  applyDraftMutationForTest,
  applyDraftSaveForTest,
  applyViewMutationsForTest,
} from "./sidebar-view-test-helpers";
import type { SidebarWorkspaceStateApi } from "@/lib/types/http-user-settings";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));

type UIStore = UseBoundStore<StoreApi<UISlice>>;

const CREATE_FAILED = "create failed";
const RENAME_FAILED = "rename failed";
const DRAFT_WRITE_FAILED = "draft write failed";
const CREATE_RESPONSE_LOST = "create response lost";
const VIEW_LIMIT_REACHED = "view limit reached";
const RENAMED_VIEW = "Renamed view";

type SettingsResponse = Awaited<ReturnType<typeof updateUserSettings>>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

function makeStore(): UIStore {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args), workspaces: { activeId: "ws" } })),
  );
}

function makeView(id: string, name: string): SidebarView {
  return {
    id,
    name,
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
    groupIndent: true,
    collapsedGroups: [],
  };
}

function seedSidebar(
  store: UIStore,
  views: SidebarView[],
  draft: SidebarViewDraft | null = null,
): void {
  store.setState((state) => ({
    ...state,
    sidebarViewsByWorkspace: {
      ws: {
        ...state.sidebarViews,
        views,
        activeViewId: views[0].id,
        draft,
      },
    },
  }));
}

beforeEach(() => {
  window.localStorage.clear();
  vi.mocked(fetchUserSettings).mockReset();
  vi.mocked(fetchUserSettings).mockResolvedValue({
    settings: {},
  } as Awaited<ReturnType<typeof fetchUserSettings>>);
  vi.mocked(updateUserSettings).mockReset();
  vi.mocked(updateUserSettings).mockResolvedValue({
    settings: {},
  } as Awaited<ReturnType<typeof updateUserSettings>>);
});

describe("createSidebarView semantics", () => {
  it("appends and activates an independent canonical view with the first available name", () => {
    const store = makeStore();
    const customized = makeView("custom", "Custom");
    customized.filters = [
      { id: "custom-filter", dimension: "titleMatch", op: "matches", value: "custom" },
    ];
    customized.sort = { key: "updatedAt", direction: "desc" };
    customized.group = "state";
    customized.collapsedGroups = ["active"];
    seedSidebar(store, [
      customized,
      makeView("new-1", "New view"),
      makeView("new-3", "New view 3"),
    ]);

    const createdId = store.getState().createSidebarView();
    const sidebar = store.getState().sidebarViewsByWorkspace.ws;
    const created = sidebar.views.at(-1);

    expect(created).toEqual({
      id: expect.stringMatching(/^view-/),
      name: "New view 2",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "repository",
      groupIndent: true,
      collapsedGroups: [],
      taskRow: {
        detailsEnabled: true,
        detailOrder: ["relative_time", "repository", "pull_request_number"],
        visibleDetails: ["relative_time", "repository", "pull_request_number"],
        trailing: "git_changes",
      },
    });
    expect(sidebar.activeViewId).toBe(createdId);
    expect(sidebar.draft).toBeNull();
    expect(created?.filters).not.toBe(DEFAULT_VIEW.filters);
    expect(created?.sort).not.toBe(DEFAULT_VIEW.sort);
    expect(created?.collapsedGroups).not.toBe(DEFAULT_VIEW.collapsedGroups);
  });

  it("does nothing while the active view has an unsaved draft", () => {
    const store = makeStore();
    const draft: SidebarViewDraft = {
      baseViewId: "all",
      filters: [],
      sort: { key: "title", direction: "asc" },
      group: "workflow",
      groupIndent: true,
    };
    seedSidebar(store, [makeView("all", "All tasks")], draft);

    expect(store.getState().createSidebarView()).toBeNull();
    expect(store.getState().sidebarViewsByWorkspace.ws.draft).toEqual(draft);
    expect(store.getState().sidebarViewsByWorkspace.ws.views).toHaveLength(1);
    expect(updateUserSettings).not.toHaveBeenCalled();
  });

  it("does nothing at the saved-view limit", () => {
    const store = makeStore();
    const views = Array.from({ length: 50 }, (_, index) =>
      makeView(`view-${index}`, `View ${index}`),
    );
    seedSidebar(store, views);

    expect(store.getState().createSidebarView()).toBeNull();
    expect(store.getState().sidebarViewsByWorkspace.ws.views).toHaveLength(50);
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(views[0].id);
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.2
describe("copied sidebar view disclosure", () => {
  it.each(["save as", "duplicate"])(
    "preserves independent plugin-group disclosure through %s and its journal",
    async (operation) => {
      const store = makeStore();
      const source = makeView("tag-source", "Tag");
      const group = "plugin:acme:labels";
      const collapsed = `${group}:value:ready`;
      source.group = group;
      source.collapsedGroups = [collapsed];
      seedSidebar(store, [source]);
      let persisted: SidebarWorkspaceStateApi = {
        views: [toApiSidebarView(source)],
        active_view_id: source.id,
        draft: null,
      };
      let revision = 0;
      vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
        const patch = payload.sidebar_view_state!;
        const views = new Map(persisted.views.map((view) => [view.id, view]));
        applyViewMutationsForTest(views, patch);
        const active = patch.active_view_id ?? persisted.active_view_id;
        persisted = applyDraftSaveForTest(
          {
            views: [...views.values()],
            active_view_id: active,
            draft: applyDraftMutationForTest(views, active, persisted.draft, patch),
          },
          patch,
        );
        return {
          settings: {
            user_id: "default-user",
            workspace_id: workspaceId("ws"),
            repository_ids: [],
            updated_at: "",
            revision: ++revision,
            sidebar_views_by_workspace: { ws: persisted },
          },
        };
      });
      if (operation === "save as") {
        store.getState().updateSidebarDraft({ groupIndent: false });
        await store.getState().waitForSidebarViewSync("ws");
        store.getState().saveSidebarDraftAs("Copy", { baseViewId: source.id, group });
      } else {
        store.getState().duplicateSidebarView(source.id, "Copy");
      }
      const state = store.getState().sidebarViewsByWorkspace.ws;
      const copied = state.views.find((view) => view.id === state.activeViewId)!;
      expect(copied.group).toBe(group);
      expect(copied.collapsedGroups).toEqual([collapsed]);
      expect(copied.collapsedGroups).not.toBe(state.views[0].collapsedGroups);
      await store.getState().waitForSidebarViewSync("ws");
      expect(
        mapSidebarWorkspaces({ ws: persisted }).ws.views.find((view) => view.id === copied.id),
      ).toMatchObject({ group, collapsedGroups: [collapsed] });
      store.getState().toggleSidebarGroupCollapsed(copied.id, collapsed);
      const expanded = store.getState().sidebarViewsByWorkspace.ws;
      expect(expanded.views.find((view) => view.id === copied.id)?.collapsedGroups).toEqual([]);
      expect(expanded.views.find((view) => view.id === source.id)?.collapsedGroups).toEqual([
        collapsed,
      ]);
      await store.getState().waitForSidebarViewSync("ws");
      expect(
        mapSidebarWorkspaces({ ws: persisted }).ws.views.find((view) => view.id === source.id)
          ?.collapsedGroups,
      ).toEqual([collapsed]);
    },
  );
});

describe("sidebar group indentation preference", () => {
  it("persists explicit false in drafts, saves, and duplicate views", async () => {
    const store = makeStore();
    const source = makeView("all", "All tasks");
    source.groupIndent = false;
    seedSidebar(store, [source]);
    let revision = 0;
    let server: SidebarWorkspaceStateApi = {
      views: [toApiSidebarView(source)],
      active_view_id: source.id,
      draft: null,
    };
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      const patch = payload.sidebar_view_state!;
      const views = new Map(server.views.map((view) => [view.id, view]));
      applyViewMutationsForTest(views, patch);
      const active = patch.active_view_id ?? server.active_view_id;
      server = {
        views: [...views.values()],
        active_view_id: active,
        draft: applyDraftMutationForTest(views, active, server.draft, patch),
      };
      server = applyDraftSaveForTest(server, patch);
      return {
        settings: {
          user_id: "default-user",
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: ++revision,
          sidebar_views_by_workspace: { ws: structuredClone(server) },
        },
      };
    });
    store.getState().updateSidebarDraft({ groupIndent: false });
    await store.getState().waitForSidebarViewSync("ws");
    expect(mapSidebarWorkspaces({ ws: server }, {}, revision).ws.draft?.groupIndent).toBe(false);
    store.getState().saveSidebarDraftOverwrite();
    await store.getState().waitForSidebarViewSync("ws");
    store.getState().duplicateSidebarView("all", "Copy");
    await store.getState().waitForSidebarViewSync("ws");
    const restored = mapSidebarWorkspaces({ ws: server }, {}, revision).ws;
    expect(restored.views.map((view) => [view.name, view.groupIndent])).toEqual([
      ["All tasks", false],
      ["Copy", false],
    ]);
  });
});

describe("createSidebarView queued rollback", () => {
  it("reconciles a committed create before a queued rename so a remote delete stays deleted", async () => {
    const store = makeStore();
    const base = makeView("base", "Base");
    const other = makeView("other", "Other");
    seedSidebar(store, [base, other]);
    const persisted = new Map([base, other].map((view) => [view.id, toApiSidebarView(view)]));
    let activeViewId = base.id;
    let revision = 1;
    const currentSettings = () =>
      ({
        settings: {
          user_id: "default-user",
          workspace_id: workspaceId("ws"),
          repository_ids: [],
          updated_at: "",
          revision: revision++,
          sidebar_views_by_workspace: {
            ws: {
              views: [...persisted.values()],
              active_view_id: activeViewId,
              draft: null,
            },
          },
        },
      }) as SettingsResponse;
    const createResponse = deferred<SettingsResponse>();
    let createdId = "";
    let firstCreateAttempt = true;
    vi.mocked(updateUserSettings).mockImplementation(async (payload) => {
      const patch = payload.sidebar_view_state!;
      applyViewMutationsForTest(persisted, patch);
      if (patch.active_view_id) activeViewId = patch.active_view_id;
      if (patch.views_create?.some((view) => view.id === createdId)) {
        if (firstCreateAttempt) {
          firstCreateAttempt = false;
          await createResponse.promise;
        }
        throw new TypeError(CREATE_RESPONSE_LOST);
      }
      return currentSettings();
    });
    vi.mocked(fetchUserSettings).mockImplementation(async () => currentSettings());

    createdId = store.getState().createSidebarView()!;
    store.getState().renameSidebarView(base.id, "Renamed base");
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(1));

    persisted.delete(createdId);
    activeViewId = base.id;
    createResponse.reject(new ApiError(CREATE_RESPONSE_LOST, 503, {}));

    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false),
    );

    const queuedPatch = vi.mocked(updateUserSettings).mock.calls[1][0].sidebar_view_state!;
    expect(fetchUserSettings).toHaveBeenCalledTimes(1);
    expect(queuedPatch.views_create).toBeUndefined();
    expect(queuedPatch.active_view_id).not.toBe(createdId);
    expect(persisted.has(createdId)).toBe(false);
    expect(activeViewId).toBe(base.id);
    expect(persisted.get(base.id)?.name).toBe("Renamed base");
    expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(CREATE_RESPONSE_LOST);
  });

  it("does not replay a view-limit rejected create with a queued independent rename", async () => {
    const store = makeStore();
    const views = Array.from({ length: 49 }, (_, index) =>
      makeView(`view-${index}`, `View ${index}`),
    );
    seedSidebar(store, views);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new ApiError(VIEW_LIMIT_REACHED, 400, {}))
      .mockResolvedValueOnce({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().renameSidebarView("view-0", RENAMED_VIEW);

    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncPending).toBe(false),
    );

    const queuedPatch = vi.mocked(updateUserSettings).mock.calls[1][0].sidebar_view_state!;
    expect(queuedPatch.views_create).toBeUndefined();
    expect(queuedPatch.views_update).toEqual(
      expect.arrayContaining([expect.objectContaining({ id: "view-0", name: RENAMED_VIEW })]),
    );
    expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(VIEW_LIMIT_REACHED);
  });
});

describe("createSidebarView queued failure rollback", () => {
  it("rolls back to the pre-create state when create and immediate rename both fail", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new ApiError(CREATE_FAILED, 500, {}))
      .mockRejectedValueOnce(new ApiError(RENAME_FAILED, 500, {}));

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().renameSidebarView(createdId!, RENAMED_VIEW);

    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(RENAME_FAILED),
    );
    const views = store.getState().sidebarViewsByWorkspace.ws.views;
    expect(views).toHaveLength(1);
    expect(views[0]).toMatchObject(original);
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(original.id);
  });

  it("keeps the automatic view when create syncs but its immediate rename fails", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings)
      .mockResolvedValueOnce({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>)
      .mockRejectedValueOnce(new ApiError(RENAME_FAILED, 500, {}));

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().renameSidebarView(createdId!, RENAMED_VIEW);

    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(RENAME_FAILED),
    );
    expect(store.getState().sidebarViewsByWorkspace.ws.views).toEqual([
      original,
      expect.objectContaining({ id: createdId, name: "New view" }),
    ]);
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(createdId);
  });

  it("restores a valid active view when two queued creates fail", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: {
        user_id: "user",
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: 1,
        sidebar_views_by_workspace: {
          ws: { views: [toApiSidebarView(original)], active_view_id: original.id, draft: null },
        },
      },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new ApiError("first create failed", 500, {}))
      .mockRejectedValueOnce(new ApiError("second create failed", 500, {}));

    const firstCreatedId = store.getState().createSidebarView();
    store.getState().createSidebarView();
    store.getState().setSidebarActiveView(firstCreatedId!);

    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe("second create failed"),
    );
    const views = store.getState().sidebarViewsByWorkspace.ws.views;
    expect(views).toHaveLength(1);
    expect(views[0]).toMatchObject(original);
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(original.id);
  });
});

describe("createSidebarView failure isolation", () => {
  it("restores the previous sidebar state when creation fails to sync", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(CREATE_FAILED, 500, {}));

    store.getState().createSidebarView();
    expect(store.getState().sidebarViewsByWorkspace.ws.views).toHaveLength(2);

    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(CREATE_FAILED),
    );
    const views = store.getState().sidebarViewsByWorkspace.ws.views;
    expect(views).toHaveLength(1);
    expect(views[0]).toMatchObject(original);
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(original.id);
  });

  it("clears a draft based on a created view that fails to sync", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(fetchUserSettings).mockResolvedValue({
      settings: {
        user_id: "user",
        workspace_id: workspaceId("ws"),
        repository_ids: [],
        updated_at: "",
        revision: 1,
        sidebar_views_by_workspace: {
          ws: { views: [toApiSidebarView(original)], active_view_id: original.id, draft: null },
        },
      },
    } as Awaited<ReturnType<typeof fetchUserSettings>>);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(CREATE_FAILED, 500, {}));

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().updateSidebarDraft({ group: "state" });

    await waitFor(() =>
      expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(CREATE_FAILED),
    );
    const views = store.getState().sidebarViewsByWorkspace.ws.views;
    expect(views).toHaveLength(1);
    expect(views[0]).toMatchObject(original);
    expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(original.id);
    expect(store.getState().sidebarViewsByWorkspace.ws.draft).toBeNull();
  });

  it("rolls back independently when another store mutates at the same time", async () => {
    const failedStore = makeStore();
    const successfulStore = makeStore();
    seedSidebar(failedStore, [makeView("failed-all", "All tasks")]);
    seedSidebar(successfulStore, [makeView("successful-all", "All tasks")]);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new ApiError("isolated failure", 500, {}))
      .mockResolvedValueOnce({ settings: {} } as Awaited<ReturnType<typeof updateUserSettings>>);

    failedStore.getState().createSidebarView();
    successfulStore.getState().createSidebarView();

    await waitFor(() =>
      expect(failedStore.getState().sidebarViewsByWorkspace.ws.syncError).toBe("isolated failure"),
    );
    expect(failedStore.getState().sidebarViewsByWorkspace.ws.views).toHaveLength(1);
    expect(successfulStore.getState().sidebarViewsByWorkspace.ws.views).toHaveLength(2);
  });
});

describe("sidebar draft write failure isolation", () => {
  it("restores the confirmed draft after a task-row write fails", async () => {
    const store = makeStore();
    seedSidebar(store, [makeView("all", "All tasks")]);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(DRAFT_WRITE_FAILED, 500, {}));

    store.getState().updateSidebarDraft({
      taskRow: {
        detailsEnabled: false,
        detailOrder: ["relative_time", "repository", "pull_request_number"],
        visibleDetails: [],
        trailing: "none",
      },
    });

    expect(store.getState().sidebarViewsByWorkspace.ws.draft).not.toBeNull();
    await waitFor(() => {
      expect(store.getState().sidebarViewsByWorkspace.ws.syncError).toBe(DRAFT_WRITE_FAILED);
      expect(store.getState().sidebarViewsByWorkspace.ws.draft).toBeNull();
    });
  });
});

describe("sidebar write ordering", () => {
  it("does not resurrect a failed create when its queued draft also fails", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    const createResponse = deferred<SettingsResponse>();
    const draftResponse = deferred<SettingsResponse>();
    vi.mocked(updateUserSettings)
      .mockImplementationOnce(() => createResponse.promise)
      .mockImplementationOnce(() => draftResponse.promise);

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().updateSidebarDraft({ group: "state" });

    createResponse.reject(new ApiError(CREATE_FAILED, 500, {}));
    await waitFor(() => expect(updateUserSettings).toHaveBeenCalledTimes(2));
    draftResponse.reject(new ApiError(DRAFT_WRITE_FAILED, 500, {}));

    await waitFor(() => {
      expect(store.getState().sidebarViewsByWorkspace.ws.views).toEqual([original]);
      expect(store.getState().sidebarViewsByWorkspace.ws.activeViewId).toBe(original.id);
      expect(store.getState().sidebarViewsByWorkspace.ws.draft).toBeNull();
    });
  });
});
