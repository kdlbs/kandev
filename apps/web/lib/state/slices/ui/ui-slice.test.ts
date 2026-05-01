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
