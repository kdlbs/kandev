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
});
