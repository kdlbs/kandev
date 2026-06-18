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
});
