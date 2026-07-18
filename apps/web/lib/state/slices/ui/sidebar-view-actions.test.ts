import { waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { create, type StoreApi, type UseBoundStore } from "zustand";
import { immer } from "zustand/middleware/immer";
import { ApiError } from "@/lib/api/client";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { DEFAULT_VIEW } from "./sidebar-view-builtins";
import { createUISlice } from "./ui-slice";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import type { UISlice } from "./types";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));

type UIStore = UseBoundStore<StoreApi<UISlice>>;

const CREATE_FAILED = "create failed";
const RENAME_FAILED = "rename failed";

function makeStore(): UIStore {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

function makeView(id: string, name: string): SidebarView {
  return {
    id,
    name,
    filters: [],
    sort: { key: "state", direction: "asc" },
    group: "none",
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
    sidebarViews: {
      ...state.sidebarViews,
      views,
      activeViewId: views[0].id,
      draft,
    },
  }));
}

beforeEach(() => {
  window.localStorage.clear();
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
    const sidebar = store.getState().sidebarViews;
    const created = sidebar.views.at(-1);

    expect(created).toEqual({
      id: expect.stringMatching(/^view-/),
      name: "New view 2",
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "repository",
      collapsedGroups: [],
    });
    expect(sidebar.activeViewId).toBe(createdId);
    expect(sidebar.draft).toBeNull();
    expect(created?.filters).not.toBe(DEFAULT_VIEW.filters);
    expect(created?.sort).not.toBe(DEFAULT_VIEW.sort);
    expect(created?.collapsedGroups).not.toBe(DEFAULT_VIEW.collapsedGroups);
    expect(updateUserSettings).toHaveBeenCalledWith({
      sidebar_views: [
        expect.objectContaining({ id: "custom", name: "Custom" }),
        expect.objectContaining({ id: "new-1", name: "New view" }),
        expect.objectContaining({ id: "new-3", name: "New view 3" }),
        expect.objectContaining({ id: createdId, name: "New view 2", group: "repository" }),
      ],
      sidebar_active_view_id: createdId,
      sidebar_draft: null,
    });
  });

  it("does nothing while the active view has an unsaved draft", () => {
    const store = makeStore();
    const draft: SidebarViewDraft = {
      baseViewId: "all",
      filters: [],
      sort: { key: "title", direction: "asc" },
      group: "workflow",
    };
    seedSidebar(store, [makeView("all", "All tasks")], draft);

    expect(store.getState().createSidebarView()).toBeNull();
    expect(store.getState().sidebarViews.draft).toEqual(draft);
    expect(store.getState().sidebarViews.views).toHaveLength(1);
    expect(updateUserSettings).not.toHaveBeenCalled();
  });

  it("does nothing at the saved-view limit", () => {
    const store = makeStore();
    const views = Array.from({ length: 50 }, (_, index) =>
      makeView(`view-${index}`, `View ${index}`),
    );
    seedSidebar(store, views);

    expect(store.getState().createSidebarView()).toBeNull();
    expect(store.getState().sidebarViews.views).toHaveLength(50);
    expect(store.getState().sidebarViews.activeViewId).toBe(views[0].id);
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});

describe("createSidebarView queued rollback", () => {
  it("rolls back to the pre-create state when create and immediate rename both fail", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new ApiError(CREATE_FAILED, 500, {}))
      .mockRejectedValueOnce(new ApiError(RENAME_FAILED, 500, {}));

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().renameSidebarView(createdId!, "Renamed view");

    await waitFor(() => expect(store.getState().sidebarViews.syncError).toBe(RENAME_FAILED));
    expect(store.getState().sidebarViews.views).toEqual([original]);
    expect(store.getState().sidebarViews.activeViewId).toBe(original.id);
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
    store.getState().renameSidebarView(createdId!, "Renamed view");

    await waitFor(() => expect(store.getState().sidebarViews.syncError).toBe(RENAME_FAILED));
    expect(store.getState().sidebarViews.views).toEqual([
      original,
      expect.objectContaining({ id: createdId, name: "New view" }),
    ]);
    expect(store.getState().sidebarViews.activeViewId).toBe(createdId);
  });

  it("restores a valid active view when two queued creates fail", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new ApiError("first create failed", 500, {}))
      .mockRejectedValueOnce(new ApiError("second create failed", 500, {}));

    const firstCreatedId = store.getState().createSidebarView();
    store.getState().createSidebarView();
    store.getState().setSidebarActiveView(firstCreatedId!);

    await waitFor(() =>
      expect(store.getState().sidebarViews.syncError).toBe("second create failed"),
    );
    expect(store.getState().sidebarViews.views).toEqual([original]);
    expect(store.getState().sidebarViews.activeViewId).toBe(original.id);
  });
});

describe("createSidebarView failure isolation", () => {
  it("restores the previous sidebar state when creation fails to sync", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(CREATE_FAILED, 500, {}));

    store.getState().createSidebarView();
    expect(store.getState().sidebarViews.views).toHaveLength(2);

    await waitFor(() => expect(store.getState().sidebarViews.syncError).toBe(CREATE_FAILED));
    expect(store.getState().sidebarViews.views).toEqual([original]);
    expect(store.getState().sidebarViews.activeViewId).toBe(original.id);
  });

  it("clears a draft based on a created view that fails to sync", async () => {
    const store = makeStore();
    const original = makeView("all", "All tasks");
    seedSidebar(store, [original]);
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new ApiError(CREATE_FAILED, 500, {}));

    const createdId = store.getState().createSidebarView();
    expect(createdId).not.toBeNull();
    store.getState().updateSidebarDraft({ group: "state" });

    await waitFor(() => expect(store.getState().sidebarViews.syncError).toBe(CREATE_FAILED));
    expect(store.getState().sidebarViews.views).toEqual([original]);
    expect(store.getState().sidebarViews.activeViewId).toBe(original.id);
    expect(store.getState().sidebarViews.draft).toBeNull();
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
      expect(failedStore.getState().sidebarViews.syncError).toBe("isolated failure"),
    );
    expect(failedStore.getState().sidebarViews.views).toHaveLength(1);
    expect(successfulStore.getState().sidebarViews.views).toHaveLength(2);
  });
});
