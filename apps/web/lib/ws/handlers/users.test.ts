import { describe, expect, it } from "vitest";
import { createStore } from "zustand/vanilla";
import { registerUsersHandlers } from "./users";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";

function makeStore() {
  return createStore<AppState>(() => structuredClone(defaultState) as AppState);
}

function userSettingsMessage(
  payload: Partial<BackendMessageMap["user.settings.updated"]["payload"]>,
): BackendMessageMap["user.settings.updated"] {
  return {
    type: "notification",
    action: "user.settings.updated",
    payload: {
      user_id: "default",
      workspace_id: "workspace",
      repository_ids: [],
      ...payload,
    },
  };
}

describe("user settings websocket handler", () => {
  it("preserves local collapsed groups when syncing sidebar views", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        activeViewId: "view-1",
        views: [
          {
            id: "view-1",
            name: "Local",
            filters: [],
            sort: { key: "state", direction: "asc" },
            group: "state",
            collapsedGroups: ["state:todo"],
          },
        ],
      },
    }));

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        sidebar_views: [
          {
            id: "view-1",
            name: "Remote",
            filters: [],
            sort: { key: "updatedAt", direction: "desc" },
            group: "workflow",
            collapsed_groups: [],
          },
        ],
        sidebar_active_view_id: "view-1",
      }),
    );

    expect(store.getState().sidebarViews.views[0]).toMatchObject({
      id: "view-1",
      name: "Remote",
      collapsedGroups: ["state:todo"],
    });
  });

  it("applies draft clears even when the broadcast has no sidebar views", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarViews: {
        ...state.sidebarViews,
        draft: {
          baseViewId: "view-1",
          filters: [],
          sort: { key: "state", direction: "asc" },
          group: "state",
        },
      },
    }));

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ sidebar_views: [], sidebar_draft: null }),
    );

    expect(store.getState().sidebarViews.draft).toBeNull();
  });

  it("does not mark empty task-create last-used broadcasts as synced", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ task_create_last_used: {} }),
    );

    expect(store.getState().userSettings.taskCreateLastUsed).toEqual({
      repositoryId: null,
      branch: null,
      agentProfileId: null,
      executorProfileId: null,
      synced: false,
    });
  });

  it("marks task-create last-used broadcasts as synced when a field is present", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ task_create_last_used: { repository_id: "repo-1" } }),
    );

    expect(store.getState().userSettings.taskCreateLastUsed).toMatchObject({
      repositoryId: "repo-1",
      synced: true,
    });
  });
});

describe("user settings websocket sidebar settings", () => {
  it("preserves userSettings sidebar fields when payload omits them", () => {
    const store = makeStore();
    const sidebarView = {
      id: "all",
      name: "All",
      filters: [],
      sort: { key: "state" as const, direction: "asc" as const },
      group: "state" as const,
      collapsedGroups: [],
    };
    store.setState((state) => ({
      ...state,
      userSettings: {
        ...state.userSettings,
        sidebarViews: [sidebarView],
        sidebarActiveViewId: "all",
        sidebarDraft: {
          baseViewId: "all",
          filters: [],
          sort: { key: "state", direction: "asc" },
          group: "state",
        },
        sidebarTaskPrefs: {
          pinnedTaskIds: ["task-1"],
          orderedTaskIds: ["task-2"],
          subtaskOrderByParentId: { parent: ["child"] },
        },
      },
    }));

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));

    expect(store.getState().userSettings.sidebarActiveViewId).toBe("all");
    expect(store.getState().userSettings.sidebarDraft).toMatchObject({ baseViewId: "all" });
    expect(store.getState().userSettings.sidebarTaskPrefs).toEqual({
      pinnedTaskIds: ["task-1"],
      orderedTaskIds: ["task-2"],
      subtaskOrderByParentId: { parent: ["child"] },
    });
  });

  it("preserves pending local sidebar task prefs when server broadcasts stale prefs", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarTaskPrefs: {
        pinnedTaskIds: ["local"],
        orderedTaskIds: ["local"],
        subtaskOrderByParentId: {},
        syncPending: true,
      },
    }));

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        sidebar_task_prefs: {
          pinned_task_ids: ["server"],
          ordered_task_ids: ["server"],
          subtask_order_by_parent_id: {},
        },
      }),
    );

    expect(store.getState().sidebarTaskPrefs).toMatchObject({
      pinnedTaskIds: ["local"],
      orderedTaskIds: ["local"],
      syncPending: true,
    });
  });

  it("applies server sidebar task prefs after a failed sync is no longer pending", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      sidebarTaskPrefs: {
        pinnedTaskIds: ["local"],
        orderedTaskIds: ["local"],
        subtaskOrderByParentId: {},
        syncError: "Failed to sync",
        syncPending: false,
      },
    }));

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        sidebar_task_prefs: {
          pinned_task_ids: ["server"],
          ordered_task_ids: ["server"],
          subtask_order_by_parent_id: {},
        },
      }),
    );

    expect(store.getState().sidebarTaskPrefs).toMatchObject({
      pinnedTaskIds: ["server"],
      orderedTaskIds: ["server"],
      syncError: "Failed to sync",
    });
  });
});
