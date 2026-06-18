import { beforeEach, describe, expect, it, vi } from "vitest";
import { create, type StoreApi, type UseBoundStore } from "zustand";
import { waitFor } from "@testing-library/react";
import { immer } from "zustand/middleware/immer";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { createUISlice } from "./ui-slice";
import { APP_SIDEBAR_EXPANDED_WIDTH } from "@/components/app-sidebar/app-sidebar-constants";
import type { SidebarViewDraft } from "./sidebar-view-types";
import type { UISlice } from "./types";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createUISlice as any)(...a) })),
  );
}

type UIStore = UseBoundStore<StoreApi<UISlice>>;

const KEY = "kandev.sidebar.collapsedSubtasks";
const TASK_A = "task-a";
const TASK_B = "task-b";
const SIDEBAR_VIEWS_KEY = "kandev.sidebar.views";
const SIDEBAR_ACTIVE_VIEW_KEY = "kandev.sidebar.activeViewId";
const SIDEBAR_DRAFT_KEY = "kandev.sidebar.draft";
const BACKEND_DOWN = "backend down";

function makeSidebarView(id: string, name: string) {
  return {
    id,
    name,
    filters: [],
    sort: { key: "state" as const, direction: "asc" as const },
    group: "none" as const,
    collapsedGroups: [],
  };
}

describe("toggleSubtaskCollapsed", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("hydrates initial state from sessionStorage", () => {
    window.sessionStorage.setItem(KEY, JSON.stringify(["task-hydrated"]));
    const store = makeStore();
    expect(store.getState().collapsedSubtaskParents).toEqual(["task-hydrated"]);
  });

  it("adds a parent id on first toggle and persists it", () => {
    const store = makeStore();
    store.getState().toggleSubtaskCollapsed(TASK_A);

    expect(store.getState().collapsedSubtaskParents).toEqual([TASK_A]);
    expect(JSON.parse(window.sessionStorage.getItem(KEY) ?? "null")).toEqual([TASK_A]);
  });

  it("removes a parent id on second toggle", () => {
    const store = makeStore();
    store.getState().toggleSubtaskCollapsed(TASK_A);
    store.getState().toggleSubtaskCollapsed(TASK_A);

    expect(store.getState().collapsedSubtaskParents).toEqual([]);
    expect(JSON.parse(window.sessionStorage.getItem(KEY) ?? "null")).toEqual([]);
  });

  it("tracks multiple parents independently", () => {
    const store = makeStore();
    store.getState().toggleSubtaskCollapsed(TASK_A);
    store.getState().toggleSubtaskCollapsed(TASK_B);

    expect(store.getState().collapsedSubtaskParents).toEqual([TASK_A, TASK_B]);

    store.getState().toggleSubtaskCollapsed(TASK_A);
    expect(store.getState().collapsedSubtaskParents).toEqual([TASK_B]);
  });
});

describe("sidebar task prefs (pin + manual order)", () => {
  const PINNED_KEY = "kandev.sidebar.pinnedTaskIds";
  const ORDER_KEY = "kandev.sidebar.orderedTaskIds";

  beforeEach(() => {
    window.localStorage.clear();
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  it("hydrates pinned + ordered from localStorage", () => {
    window.localStorage.setItem(PINNED_KEY, JSON.stringify(["t1"]));
    window.localStorage.setItem(ORDER_KEY, JSON.stringify(["t2", "t1"]));
    const store = makeStore();
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t1"]);
    expect(store.getState().sidebarTaskPrefs.orderedTaskIds).toEqual(["t2", "t1"]);
  });

  it("togglePinnedTask adds, removes, and persists", () => {
    const store = makeStore();
    store.getState().togglePinnedTask("t1");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t1"]);
    expect(JSON.parse(window.localStorage.getItem(PINNED_KEY) ?? "null")).toEqual(["t1"]);

    store.getState().togglePinnedTask("t2");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t1", "t2"]);

    store.getState().togglePinnedTask("t1");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t2"]);
    expect(JSON.parse(window.localStorage.getItem(PINNED_KEY) ?? "null")).toEqual(["t2"]);
  });

  it("setSidebarTaskOrder replaces and persists", () => {
    const store = makeStore();
    store.getState().setSidebarTaskOrder(["a", "b", "c"]);
    expect(store.getState().sidebarTaskPrefs.orderedTaskIds).toEqual(["a", "b", "c"]);
    expect(JSON.parse(window.localStorage.getItem(ORDER_KEY) ?? "null")).toEqual(["a", "b", "c"]);

    store.getState().setSidebarTaskOrder(["c", "a"]);
    expect(store.getState().sidebarTaskPrefs.orderedTaskIds).toEqual(["c", "a"]);
    expect(JSON.parse(window.localStorage.getItem(ORDER_KEY) ?? "null")).toEqual(["c", "a"]);
  });

  it("removeTaskFromSidebarPrefs strips the id from both arrays and persists", () => {
    const store = makeStore();
    store.getState().togglePinnedTask("t1");
    store.getState().togglePinnedTask("t2");
    store.getState().setSidebarTaskOrder(["t1", "t2", "t3"]);

    store.getState().removeTaskFromSidebarPrefs("t1");

    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t2"]);
    expect(store.getState().sidebarTaskPrefs.orderedTaskIds).toEqual(["t2", "t3"]);
    expect(JSON.parse(window.localStorage.getItem(PINNED_KEY) ?? "null")).toEqual(["t2"]);
    expect(JSON.parse(window.localStorage.getItem(ORDER_KEY) ?? "null")).toEqual(["t2", "t3"]);

    // Subsequent togglePinnedTask must NOT bring "t1" back from a stale draft.
    store.getState().togglePinnedTask("t3");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t2", "t3"]);
    expect(JSON.parse(window.localStorage.getItem(PINNED_KEY) ?? "null")).toEqual(["t2", "t3"]);
  });

  it("removeTaskFromSidebarPrefs is a no-op for unknown ids", () => {
    const store = makeStore();
    store.getState().togglePinnedTask("t1");
    const before = window.localStorage.getItem(PINNED_KEY);
    store.getState().removeTaskFromSidebarPrefs("ghost");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["t1"]);
    expect(window.localStorage.getItem(PINNED_KEY)).toBe(before);
  });

  it("records sync errors and clears them after a later successful sync", async () => {
    const store = makeStore();
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new Error(BACKEND_DOWN));

    store.getState().togglePinnedTask("t1");

    await waitFor(() => {
      expect(store.getState().sidebarTaskPrefs.syncError).toBe(BACKEND_DOWN);
    });

    vi.mocked(updateUserSettings).mockResolvedValueOnce({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);

    store.getState().togglePinnedTask("t2");

    await waitFor(() => {
      expect(store.getState().sidebarTaskPrefs.syncError).toBeNull();
    });
  });

  it("clears task preference sync errors on demand", async () => {
    const store = makeStore();
    vi.mocked(updateUserSettings).mockRejectedValueOnce(new Error(BACKEND_DOWN));
    store.getState().togglePinnedTask("t1");

    await waitFor(() => {
      expect(store.getState().sidebarTaskPrefs.syncError).toBe(BACKEND_DOWN);
    });

    store.getState().clearSidebarTaskPrefsSyncError();

    expect(store.getState().sidebarTaskPrefs.syncError).toBeNull();
  });
});

describe("sidebar view sync rollback", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.mocked(updateUserSettings).mockResolvedValue({
      settings: {},
    } as Awaited<ReturnType<typeof updateUserSettings>>);
  });

  function seedViews(store: UIStore) {
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        views: [makeSidebarView("view-a", "View A"), makeSidebarView("view-b", "View B")],
        activeViewId: "view-a",
        draft: null,
      },
    }));
  }

  it("does not roll back an active view changed after a failed view mutation", async () => {
    const store = makeStore();
    seedViews(store);
    vi.mocked(updateUserSettings)
      .mockRejectedValueOnce(new Error("rename failed"))
      .mockResolvedValueOnce({
        settings: {},
      } as Awaited<ReturnType<typeof updateUserSettings>>);

    store.getState().renameSidebarView("view-a", "Renamed");
    store.getState().setSidebarActiveView("view-b");

    await waitFor(() => {
      expect(store.getState().sidebarViews.syncError).toBe("rename failed");
    });

    expect(store.getState().sidebarViews.activeViewId).toBe("view-b");
    expect(store.getState().sidebarViews.views.find((v) => v.id === "view-a")?.name).toBe("View A");
  });
});

describe("setSubtaskOrder", () => {
  const SUB_KEY = "kandev.sidebar.subtaskOrderByParentId";
  const PARENT_A = "parent-a";
  const PARENT_B = "parent-b";

  beforeEach(() => {
    window.localStorage.clear();
  });

  it("hydrates subtaskOrderByParentId from localStorage", () => {
    window.localStorage.setItem(SUB_KEY, JSON.stringify({ [PARENT_A]: ["c1", "c2"] }));
    const store = makeStore();
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({
      [PARENT_A]: ["c1", "c2"],
    });
  });

  it("setSubtaskOrder writes per-parent order and persists", () => {
    const store = makeStore();
    store.getState().setSubtaskOrder(PARENT_A, ["c2", "c1"]);
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({
      [PARENT_A]: ["c2", "c1"],
    });
    expect(JSON.parse(window.localStorage.getItem(SUB_KEY) ?? "null")).toEqual({
      [PARENT_A]: ["c2", "c1"],
    });

    store.getState().setSubtaskOrder(PARENT_B, ["d1"]);
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({
      [PARENT_A]: ["c2", "c1"],
      [PARENT_B]: ["d1"],
    });
  });

  it("setSubtaskOrder with empty array clears the parent entry", () => {
    const store = makeStore();
    store.getState().setSubtaskOrder(PARENT_A, ["c1"]);
    store.getState().setSubtaskOrder(PARENT_A, []);
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({});
    expect(JSON.parse(window.localStorage.getItem(SUB_KEY) ?? "null")).toEqual({});
  });

  it("removeTaskFromSidebarPrefs drops the task as a parent key and from sibling lists", () => {
    const store = makeStore();
    store.getState().setSubtaskOrder(PARENT_A, ["c1", "c2"]);
    store.getState().setSubtaskOrder(PARENT_B, ["c1", "d1"]);

    // Removing c1 — it's both a subtask under p1 and p2.
    store.getState().removeTaskFromSidebarPrefs("c1");
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({
      [PARENT_A]: ["c2"],
      [PARENT_B]: ["d1"],
    });

    // Removing the parent itself wipes its entry.
    store.getState().removeTaskFromSidebarPrefs(PARENT_A);
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({
      [PARENT_B]: ["d1"],
    });
  });

  it("removeTaskFromSidebarPrefs drops the parent key if removing its last subtask", () => {
    const store = makeStore();
    store.getState().setSubtaskOrder(PARENT_A, ["c1"]);
    store.getState().removeTaskFromSidebarPrefs("c1");
    expect(store.getState().sidebarTaskPrefs.subtaskOrderByParentId).toEqual({});
    expect(JSON.parse(window.localStorage.getItem(SUB_KEY) ?? "null")).toEqual({});
  });
});

describe("appSidebar actions", () => {
  const COLLAPSED_KEY = "kandev.appSidebar.collapsed";
  const SECTION_KEY = "kandev.appSidebar.sectionExpanded";

  beforeEach(() => {
    window.localStorage.clear();
  });

  it("hydrates default state when localStorage is empty", () => {
    const store = makeStore();
    expect(store.getState().appSidebar.collapsed).toBe(false);
    expect(store.getState().appSidebar.width).toBe(APP_SIDEBAR_EXPANDED_WIDTH);
    expect(store.getState().appSidebar.sectionExpanded.tasks).toBe(true);
    expect(store.getState().appSidebar.sectionExpanded["office-work"]).toBe(true);
    expect(store.getState().appSidebar.sectionExpanded["office-workspace"]).toBe(true);
    expect(store.getState().appSidebar.sectionExpanded.projects).toBe(true);
    expect(store.getState().appSidebar.sectionExpanded.agents).toBe(true);
  });

  it("hydrates collapsed flag from localStorage", () => {
    window.localStorage.setItem(COLLAPSED_KEY, JSON.stringify(true));
    const store = makeStore();
    expect(store.getState().appSidebar.collapsed).toBe(true);
  });

  it("toggleAppSidebar flips the collapsed flag and persists it", () => {
    const store = makeStore();
    store.getState().toggleAppSidebar();
    expect(store.getState().appSidebar.collapsed).toBe(true);
    expect(JSON.parse(window.localStorage.getItem(COLLAPSED_KEY) ?? "null")).toBe(true);

    store.getState().toggleAppSidebar();
    expect(store.getState().appSidebar.collapsed).toBe(false);
    expect(JSON.parse(window.localStorage.getItem(COLLAPSED_KEY) ?? "null")).toBe(false);
  });

  it("setAppSidebarCollapsed writes the requested value and persists it", () => {
    const store = makeStore();
    store.getState().setAppSidebarCollapsed(true);
    expect(store.getState().appSidebar.collapsed).toBe(true);
    expect(JSON.parse(window.localStorage.getItem(COLLAPSED_KEY) ?? "null")).toBe(true);
  });

  it("toggleAppSidebarSection flips per-section state and persists the map", () => {
    const store = makeStore();
    store.getState().toggleAppSidebarSection("projects");
    expect(store.getState().appSidebar.sectionExpanded.projects).toBe(false);
    const persisted = JSON.parse(window.localStorage.getItem(SECTION_KEY) ?? "{}");
    expect(persisted.projects).toBe(false);

    store.getState().toggleAppSidebarSection("projects");
    expect(store.getState().appSidebar.sectionExpanded.projects).toBe(true);
  });

  it("toggleAppSidebarSection honors the caller default for missing section keys", () => {
    const store = makeStore();
    store.setState((draft) => {
      delete draft.appSidebar.sectionExpanded["future-section"];
    });

    store.getState().toggleAppSidebarSection("future-section", true);

    expect(store.getState().appSidebar.sectionExpanded["future-section"]).toBe(false);
    const persisted = JSON.parse(window.localStorage.getItem(SECTION_KEY) ?? "{}");
    expect(persisted["future-section"]).toBe(false);
  });

  it("settingsMode defaults off and is never read from storage", () => {
    window.localStorage.setItem("kandev.appSidebar.settingsMode", JSON.stringify(true));
    const store = makeStore();
    expect(store.getState().appSidebar.settingsMode).toBe(false);
  });

  it("toggleAppSidebarSettingsMode flips the flag without persisting it", () => {
    const store = makeStore();
    store.getState().toggleAppSidebarSettingsMode();
    expect(store.getState().appSidebar.settingsMode).toBe(true);
    expect(window.localStorage.getItem("kandev.appSidebar.settingsMode")).toBeNull();

    store.getState().toggleAppSidebarSettingsMode();
    expect(store.getState().appSidebar.settingsMode).toBe(false);
  });

  it("entering settings mode while collapsed force-expands the rail", () => {
    window.localStorage.setItem(COLLAPSED_KEY, JSON.stringify(true));
    const store = makeStore();
    expect(store.getState().appSidebar.collapsed).toBe(true);

    store.getState().toggleAppSidebarSettingsMode();
    expect(store.getState().appSidebar.settingsMode).toBe(true);
    expect(store.getState().appSidebar.collapsed).toBe(false);
    expect(JSON.parse(window.localStorage.getItem(COLLAPSED_KEY) ?? "null")).toBe(false);
  });

  it("leaving settings mode leaves the collapsed flag untouched", () => {
    const store = makeStore();
    store.getState().toggleAppSidebarSettingsMode(); // on (expands)
    store.getState().toggleAppSidebarSettingsMode(); // off
    expect(store.getState().appSidebar.settingsMode).toBe(false);
    expect(store.getState().appSidebar.collapsed).toBe(false);
  });
});

describe("reorderSidebarViews", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.mocked(updateUserSettings).mockClear();
  });

  it("reorders by id, persists the order, and syncs the backend payload", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        views: [
          makeSidebarView("all", "All"),
          makeSidebarView("one", "One"),
          makeSidebarView("two", "Two"),
        ],
        activeViewId: "two",
        draft: null,
      },
    }));

    store.getState().reorderSidebarViews("two", "one");

    expect(store.getState().sidebarViews.views.map((v) => v.id)).toEqual(["all", "two", "one"]);
    expect(JSON.parse(window.localStorage.getItem(SIDEBAR_VIEWS_KEY) ?? "[]")).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: "all" }),
        expect.objectContaining({ id: "two" }),
        expect.objectContaining({ id: "one" }),
      ]),
    );
    expect(
      JSON.parse(window.localStorage.getItem(SIDEBAR_VIEWS_KEY) ?? "[]").map(
        (v: { id: string }) => v.id,
      ),
    ).toEqual(["all", "two", "one"]);
    expect(updateUserSettings).toHaveBeenCalledWith({
      sidebar_views: [
        expect.objectContaining({ id: "all" }),
        expect.objectContaining({ id: "two" }),
        expect.objectContaining({ id: "one" }),
      ],
      sidebar_active_view_id: "two",
      sidebar_draft: null,
    });
  });

  it("keeps the active view and draft while reordering", () => {
    const draft: SidebarViewDraft = {
      baseViewId: "one",
      filters: [{ id: "c1", dimension: "titleMatch", op: "matches", value: "bug" }],
      sort: { key: "title", direction: "asc" },
      group: "workflow",
    };
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        views: [
          makeSidebarView("all", "All"),
          makeSidebarView("one", "One"),
          makeSidebarView("two", "Two"),
        ],
        activeViewId: "one",
        draft,
      },
    }));

    store.getState().reorderSidebarViews("two", "all");

    expect(store.getState().sidebarViews.views.map((v) => v.id)).toEqual(["two", "all", "one"]);
    expect(store.getState().sidebarViews.activeViewId).toBe("one");
    expect(store.getState().sidebarViews.draft).toEqual(draft);
  });

  it("no-ops when ids are equal or missing", () => {
    const store = makeStore();
    const views = [
      makeSidebarView("all", "All"),
      makeSidebarView("one", "One"),
      makeSidebarView("two", "Two"),
    ];
    store.setState((state) => ({
      ...state,
      sidebarViews: { ...state.sidebarViews, views, activeViewId: "all", draft: null },
    }));

    store.getState().reorderSidebarViews("one", "one");
    store.getState().reorderSidebarViews("missing", "one");
    store.getState().reorderSidebarViews("one", "missing");

    expect(store.getState().sidebarViews.views.map((v) => v.id)).toEqual(["all", "one", "two"]);
    expect(updateUserSettings).not.toHaveBeenCalled();
  });
});

describe("sidebar view backend state", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.mocked(updateUserSettings).mockClear();
  });

  it("syncs active view changes to backend user settings", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        views: [makeSidebarView("all", "All"), makeSidebarView("mine", "Mine")],
        activeViewId: "all",
        draft: {
          baseViewId: "all",
          filters: [],
          sort: { key: "state", direction: "asc" },
          group: "state",
        },
      },
    }));

    store.getState().setSidebarActiveView("mine");

    expect(store.getState().sidebarViews.activeViewId).toBe("mine");
    expect(window.localStorage.getItem(SIDEBAR_ACTIVE_VIEW_KEY)).toBe(JSON.stringify("mine"));
    expect(window.localStorage.getItem(SIDEBAR_DRAFT_KEY)).toBeNull();
    expect(updateUserSettings).toHaveBeenCalledWith({
      sidebar_active_view_id: "mine",
      sidebar_draft: null,
    });
  });

  it("syncs filter sort and group drafts to backend user settings", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        views: [makeSidebarView("all", "All")],
        activeViewId: "all",
        draft: null,
      },
    }));

    store.getState().updateSidebarDraft({
      sort: { key: "updatedAt", direction: "desc" },
      group: "workflow",
    });

    expect(updateUserSettings).toHaveBeenCalledWith({
      sidebar_active_view_id: "all",
      sidebar_draft: {
        base_view_id: "all",
        filters: [],
        sort: { key: "updatedAt", direction: "desc" },
        group: "workflow",
      },
    });
  });

  it("includes active view and draft state when syncing saved view mutations", () => {
    const draft: SidebarViewDraft = {
      baseViewId: "all",
      filters: [],
      sort: { key: "updatedAt", direction: "desc" },
      group: "state",
    };
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        views: [makeSidebarView("all", "All"), makeSidebarView("two", "Two")],
        activeViewId: "all",
        draft,
      },
    }));

    store.getState().reorderSidebarViews("two", "all");

    expect(updateUserSettings).toHaveBeenCalledWith({
      sidebar_views: [
        expect.objectContaining({ id: "two" }),
        expect.objectContaining({ id: "all" }),
      ],
      sidebar_active_view_id: "all",
      sidebar_draft: {
        base_view_id: "all",
        filters: [],
        sort: { key: "updatedAt", direction: "desc" },
        group: "state",
      },
    });
  });
});
