import { beforeEach, describe, expect, it, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { createUISlice } from "./ui-slice";
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

const KEY = "kandev.sidebar.collapsedSubtasks";
const TASK_A = "task-a";
const TASK_B = "task-b";
const SIDEBAR_VIEWS_KEY = "kandev.sidebar.views";

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
