import { describe, expect, it } from "vitest";
import { createStore } from "zustand/vanilla";
import { registerUsersHandlers } from "./users";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";

/** Creates a vanilla Zustand store seeded with a structured clone of the default app state. */
function makeStore() {
  return createStore<AppState>(() => structuredClone(defaultState) as AppState);
}

/** Builds a `user.settings.updated` notification frame merging the given payload over default values. */
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

function recentUseMessage(
  payload: Partial<BackendMessageMap["user.agent_profile_recent_use.updated"]["payload"]>,
): BackendMessageMap["user.agent_profile_recent_use.updated"] {
  return {
    type: "notification",
    action: "user.agent_profile_recent_use.updated",
    payload: {
      context: "task_create",
      profile_ids: [],
      revision: 1,
      updated_at: "2026-08-27T12:00:00Z",
      ...payload,
    },
  };
}

describe("agent profile recent-use websocket sync", () => {
  it("keeps newer context revisions and accepts independent contexts", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      agentProfileRecentUse: {
        loaded: true,
        records: {
          task_create: { profileIds: ["profile-new"], revision: 2, updatedAt: "later" },
        },
      },
    }));

    const handler = registerUsersHandlers(store)["user.agent_profile_recent_use.updated"];
    handler?.(
      recentUseMessage({
        profile_ids: ["profile-old"],
        revision: 1,
        updated_at: "older",
      }),
    );
    expect(store.getState().agentProfileRecentUse.records.task_create?.profileIds).toEqual([
      "profile-new",
    ]);

    handler?.(
      recentUseMessage({
        context: "quick_chat",
        profile_ids: ["profile-chat"],
        revision: 1,
      }),
    );
    expect(store.getState().agentProfileRecentUse.records.quick_chat?.profileIds).toEqual([
      "profile-chat",
    ]);
  });
});

describe("startup page websocket sync", () => {
  it("applies startup page preferences and normalizes unknown values", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        startup_page: "last_task",
      } as unknown as Partial<BackendMessageMap["user.settings.updated"]["payload"]>),
    );
    expect(store.getState().userSettings.startupPage).toBe("last_task");

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        startup_page: "unexpected",
      } as unknown as Partial<BackendMessageMap["user.settings.updated"]["payload"]>),
    );
    expect(store.getState().userSettings.startupPage).toBe("task_overview");
  });
});

describe("status bar visibility websocket sync", () => {
  it("updates status bar visibility and preserves it when omitted", () => {
    const store = makeStore();

    expect(
      (store.getState().userSettings as unknown as Record<string, unknown>).appStatusBarEnabled,
    ).toBe(false);

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        app_status_bar_enabled: true,
      } as unknown as Partial<BackendMessageMap["user.settings.updated"]["payload"]>),
    );
    expect(
      (store.getState().userSettings as unknown as Record<string, unknown>).appStatusBarEnabled,
    ).toBe(true);

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(
      (store.getState().userSettings as unknown as Record<string, unknown>).appStatusBarEnabled,
    ).toBe(true);
  });

  it("ignores older settings revisions and accepts newer ones", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      userSettings: {
        ...state.userSettings,
        appStatusBarEnabled: false,
        revision: 2,
      },
    }));
    const handler = registerUsersHandlers(store)["user.settings.updated"];

    handler?.(
      userSettingsMessage({
        app_status_bar_enabled: true,
        revision: 1,
      }),
    );
    expect(store.getState().userSettings).toMatchObject({
      appStatusBarEnabled: false,
      revision: 2,
    });

    handler?.(
      userSettingsMessage({
        app_status_bar_enabled: true,
        revision: 3,
      }),
    );
    expect(store.getState().userSettings).toMatchObject({
      appStatusBarEnabled: true,
      revision: 3,
    });
  });
});

describe("last seen display websocket sync", () => {
  it("applies a relative value and normalizes unknown values", () => {
    const store = makeStore();
    expect(store.getState().userSettings.lastSeenDisplay).toBe("absolute");

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ last_seen_display: "relative" }),
    );
    expect(store.getState().userSettings.lastSeenDisplay).toBe("relative");

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        last_seen_display: "unexpected",
      } as unknown as Partial<BackendMessageMap["user.settings.updated"]["payload"]>),
    );
    expect(store.getState().userSettings.lastSeenDisplay).toBe("absolute");
  });

  it("preserves the current value when omitted", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      userSettings: { ...state.userSettings, lastSeenDisplay: "relative" },
    }));
    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.lastSeenDisplay).toBe("relative");
  });

  it("ignores older settings revisions", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      userSettings: { ...state.userSettings, lastSeenDisplay: "absolute", revision: 2 },
    }));
    const handler = registerUsersHandlers(store)["user.settings.updated"];
    handler?.(userSettingsMessage({ last_seen_display: "relative", revision: 1 }));
    expect(store.getState().userSettings.lastSeenDisplay).toBe("absolute");
  });
});

describe("user settings websocket handler", () => {
  it("updates LSP status location, normalizes unknown values, and preserves omissions", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ lsp_status_location: "status_bar" }),
    );
    expect(store.getState().userSettings.lspStatusLocation).toBe("status_bar");

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.lspStatusLocation).toBe("status_bar");

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        lsp_status_location: "sidebar",
      } as unknown as Partial<BackendMessageMap["user.settings.updated"]["payload"]>),
    );
    expect(store.getState().userSettings.lspStatusLocation).toBe("toolbar");
  });

  it("updates the List detail preference and preserves it when omitted", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ tasks_list_show_details: true }),
    );
    expect(store.getState().userSettings.tasksListShowDetails).toBe(true);

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.tasksListShowDetails).toBe(true);
  });

  it("normalizes the simplified metrics preference from live updates", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        system_metrics_display: { show_in_topbar: true, simplified: true } as never,
      }),
    );

    expect(store.getState().userSettings.systemMetricsDisplay).toEqual({
      showInTopbar: true,
      simplified: true,
    });
  });

  it("replaces portable status order when present and preserves it when omitted", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        app_status_bar_order: {
          left_item_ids: ["builtin:metrics"],
          right_item_ids: ["builtin:connection"],
        },
      }),
    );
    expect(store.getState().userSettings.appStatusBarOrder).toEqual({
      leftItemIds: ["builtin:metrics"],
      rightItemIds: ["builtin:connection"],
    });

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.appStatusBarOrder).toEqual({
      leftItemIds: ["builtin:metrics"],
      rightItemIds: ["builtin:connection"],
    });
  });

  it("applies valid MCP task profile preferences and normalizes unknown values", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        mcp_task_agent_profile_default: "workspace_default",
      }),
    );
    expect(store.getState().userSettings.mcpTaskAgentProfileDefault).toBe("workspace_default");

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        mcp_task_agent_profile_default: "unexpected",
      } as unknown as Partial<BackendMessageMap["user.settings.updated"]["payload"]>),
    );
    expect(store.getState().userSettings.mcpTaskAgentProfileDefault).toBe("current_task");
  });

  it("applies archive confirmation preferences and preserves them when omitted", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ confirm_task_archive: false }),
    );
    expect(store.getState().userSettings.confirmTaskArchive).toBe(false);

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.confirmTaskArchive).toBe(false);
  });

  it("syncs the agent-generated title preference", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ agent_generated_task_titles: true }),
    );
    expect(store.getState().userSettings.agentGeneratedTaskTitles).toBe(true);

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.agentGeneratedTaskTitles).toBe(true);
  });
});

describe("user settings websocket transcript navigation", () => {
  it("syncs transcript navigation preferences and uses the documented defaults", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        show_anchored_prompt_bar: false,
        show_scroll_to_last_prompt: false,
        show_scroll_to_start: false,
      }),
    );
    expect(store.getState().userSettings).toMatchObject({
      showAnchoredPromptBar: false,
      showScrollToLastPrompt: false,
      showScrollToStart: false,
    });

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings).toMatchObject({
      showAnchoredPromptBar: false,
      showScrollToLastPrompt: false,
      showScrollToStart: false,
      showTranscriptAutoScrollControl: false,
    });
  });
});

describe("todo list panel websocket sync", () => {
  it("syncs the todo list panel preference and preserves it when omitted", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ show_todo_list_panel: true }),
    );
    expect(store.getState().userSettings.showTodoListPanel).toBe(true);

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.showTodoListPanel).toBe(true);
  });

  it("syncs the not-empty sub-option and preserves it when omitted", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ show_todo_list_panel_only_when_not_empty: true }),
    );
    expect(store.getState().userSettings.showTodoListPanelOnlyWhenNotEmpty).toBe(true);

    registerUsersHandlers(store)["user.settings.updated"]?.(userSettingsMessage({}));
    expect(store.getState().userSettings.showTodoListPanelOnlyWhenNotEmpty).toBe(true);
  });
});

describe("user settings websocket partial updates", () => {
  it("preserves normalized preferences omitted from a partial live update", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        unread_divider: true,
        show_transcript_auto_scroll_control: true,
      }),
    );
    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ preferred_shell: "zsh" }),
    );

    expect(store.getState().userSettings).toMatchObject({
      unreadDivider: true,
      showTranscriptAutoScrollControl: true,
    });
  });
});

describe("user settings websocket sidebar sync", () => {
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

describe("user settings websocket task-create last-used", () => {
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
      workflowIdsByWorkspace: {},
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

  it("maps workspace-scoped workflow history from broadcasts", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        task_create_last_used: {
          workflow_ids_by_workspace: {
            "workspace-1": "workflow-1",
          },
        },
      }),
    );

    expect(store.getState().userSettings.taskCreateLastUsed).toMatchObject({
      workflowIdsByWorkspace: { "workspace-1": "workflow-1" },
      synced: true,
    });
  });

  it("preserves task-create last-used state when broadcasts omit it", () => {
    const store = makeStore();
    store.setState((state) => ({
      ...state,
      userSettings: {
        ...state.userSettings,
        taskCreateLastUsed: {
          repositoryId: "repo-1",
          branch: "main",
          agentProfileId: "agent-1",
          executorProfileId: "exec-1",
          workflowIdsByWorkspace: {},
          synced: true,
        },
      },
    }));

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({ keyboard_shortcuts: {} }),
    );

    expect(store.getState().userSettings.taskCreateLastUsed).toEqual({
      repositoryId: "repo-1",
      branch: "main",
      agentProfileId: "agent-1",
      executorProfileId: "exec-1",
      workflowIdsByWorkspace: {},
      synced: true,
    });
  });
});

describe("user settings websocket sidebar draft migration", () => {
  it("preserves an archived clause in a live draft", () => {
    const store = makeStore();

    registerUsersHandlers(store)["user.settings.updated"]?.(
      userSettingsMessage({
        sidebar_views: [],
        sidebar_draft: {
          base_view_id: "all",
          filters: [{ id: "archived", dimension: "archived", op: "is", value: true }],
          sort: { key: "state", direction: "asc" },
          group: "none",
        },
      }),
    );

    expect(store.getState().sidebarViews.draft?.filters).toEqual([
      { id: "archived", dimension: "archived", op: "is", value: true },
    ]);
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
