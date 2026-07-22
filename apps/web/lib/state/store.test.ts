import { describe, expect, it } from "vitest";
import { createAppStore, type AppState } from "./store";

describe("createAppStore", () => {
  it("retains sidebar boot settings after slice initialization", () => {
    const store = createAppStore({
      userSettings: {
        sidebarViews: [
          {
            id: "server",
            name: "Server",
            filters: [],
            sort: { key: "state", direction: "asc" },
            group: "none",
            collapsedGroups: [],
          },
        ],
        sidebarActiveViewId: "server",
        sidebarTaskPrefs: {
          pinnedTaskIds: ["task-1"],
          orderedTaskIds: ["task-1"],
          subtaskOrderByParentId: {},
        },
        loaded: true,
      },
    } as unknown as Partial<AppState>);

    expect(store.getState().sidebarViews.activeViewId).toBe("server");
    expect(store.getState().sidebarTaskPrefs.pinnedTaskIds).toEqual(["task-1"]);
  });

  it("starts with no task-scoped Review PR overrides", () => {
    const store = createAppStore();

    expect(store.getState().reviewPRSelection).toEqual({ selectedKeyByTaskId: {} });
  });

  it("stores Review PR overrides independently by task", () => {
    const store = createAppStore();
    const state = store.getState();

    state.setReviewPRSelection("task-1", "acme/widget/1");
    state.setReviewPRSelection("task-2", "acme/widget/2");

    expect(store.getState().reviewPRSelection.selectedKeyByTaskId).toEqual({
      "task-1": "acme/widget/1",
      "task-2": "acme/widget/2",
    });
  });
});
