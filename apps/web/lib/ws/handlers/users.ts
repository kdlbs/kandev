import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { UserSettingsUpdatedPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import {
  parseChangesPanelLayout,
  parseSystemMetricsDisplay,
  taskCreateLastUsedHasValue,
  parseVoiceMode,
  parseMCPTaskAgentProfileDefault,
} from "@/lib/ssr/user-settings";
import { fromApiSidebarDraft, fromApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import { migrateView } from "@/lib/state/slices/ui/ui-slice";

export function registerUsersHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "user.settings.updated": (message) => {
      store.setState((state) => ({
        ...state,
        sidebarViews: buildSidebarViewsState(state, message.payload),
        sidebarTaskPrefs: buildSidebarTaskPrefsState(state, message.payload),
        userSettings: buildUserSettingsState(state, message.payload),
      }));
    },
  };
}

function buildUserSettingsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  return {
    ...state.userSettings,
    ...buildBehaviorSettings(payload),
    ...buildSidebarSettings(state, payload),
    ...buildLspSettings(payload),
    ...buildSyncedLocalSettings(state, payload),
    defaultUtilityAgentId: payload.default_utility_agent_id || null,
    keyboardShortcuts: payload.keyboard_shortcuts ?? {},
    changesPanelLayout: parseChangesPanelLayout(payload.changes_panel_layout),
    systemMetricsDisplay: parseSystemMetricsDisplay(payload.system_metrics_display),
    voiceMode: parseVoiceMode(payload.voice_mode),
    loaded: true,
  };
}

function buildLspSettings(payload: UserSettingsUpdatedPayload) {
  return {
    lspAutoStartLanguages: payload.lsp_auto_start_languages ?? [],
    lspAutoInstallLanguages: payload.lsp_auto_install_languages ?? [],
  };
}

function buildSyncedLocalSettings(state: AppState, payload: UserSettingsUpdatedPayload) {
  return {
    savedLayouts: payload.saved_layouts ?? [],
    taskCreateLastUsed:
      payload.task_create_last_used === undefined
        ? state.userSettings.taskCreateLastUsed
        : {
            repositoryId: payload.task_create_last_used?.repository_id || null,
            branch: payload.task_create_last_used?.branch || null,
            agentProfileId: payload.task_create_last_used?.agent_profile_id || null,
            executorProfileId: payload.task_create_last_used?.executor_profile_id || null,
            synced: taskCreateLastUsedHasValue(payload.task_create_last_used),
          },
    jiraSavedViews: payload.jira_saved_views,
    jiraTaskPresets: payload.jira_task_presets,
    githubSavedPresets: payload.github_saved_presets,
    githubDefaultQueryPresets: payload.github_default_query_presets,
    gitlabSavedPresets: payload.gitlab_saved_presets,
  };
}

function buildBehaviorSettings(payload: UserSettingsUpdatedPayload) {
  return {
    preferredShell: payload.preferred_shell || null,
    defaultEditorId: payload.default_editor_id || null,
    enablePreviewOnClick: payload.enable_preview_on_click ?? false,
    chatSubmitKey: (payload.chat_submit_key as "enter" | "cmd_enter") ?? "cmd_enter",
    reviewAutoMarkOnScroll: payload.review_auto_mark_on_scroll ?? true,
    confirmTaskArchive: payload.confirm_task_archive ?? true,
    mcpTaskAgentProfileDefault: parseMCPTaskAgentProfileDefault(
      payload.mcp_task_agent_profile_default,
    ),
    showReleaseNotification: payload.show_release_notification ?? true,
    releaseNotesLastSeenVersion: (payload.release_notes_last_seen_version as string) || null,
    terminalLinkBehavior:
      payload.terminal_link_behavior === "browser_panel"
        ? ("browser_panel" as const)
        : ("new_tab" as const),
  };
}

function buildSidebarSettings(state: AppState, payload: UserSettingsUpdatedPayload) {
  const sidebarViews =
    payload.sidebar_views === undefined
      ? state.userSettings.sidebarViews
      : (payload.sidebar_views ?? []).map(fromApiSidebarView).map(migrateView);
  return {
    sidebarViews,
    sidebarActiveViewId:
      payload.sidebar_active_view_id === undefined
        ? state.userSettings.sidebarActiveViewId
        : payload.sidebar_active_view_id || null,
    sidebarDraft: parseSidebarDraftForSettings(state, payload),
    sidebarTaskPrefs:
      payload.sidebar_task_prefs === undefined
        ? state.userSettings.sidebarTaskPrefs
        : {
            pinnedTaskIds: payload.sidebar_task_prefs.pinned_task_ids ?? [],
            orderedTaskIds: payload.sidebar_task_prefs.ordered_task_ids ?? [],
            subtaskOrderByParentId: payload.sidebar_task_prefs.subtask_order_by_parent_id ?? {},
          },
  };
}

function parseSidebarDraftForSettings(state: AppState, payload: UserSettingsUpdatedPayload) {
  if (payload.sidebar_draft === undefined) return state.userSettings.sidebarDraft;
  if (payload.sidebar_draft === null) return null;
  return fromApiSidebarDraft(payload.sidebar_draft);
}

function buildSidebarTaskPrefsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  if (!payload.sidebar_task_prefs) return state.sidebarTaskPrefs;
  if (state.sidebarTaskPrefs.syncPending) return state.sidebarTaskPrefs;
  return {
    pinnedTaskIds: payload.sidebar_task_prefs.pinned_task_ids ?? [],
    orderedTaskIds: payload.sidebar_task_prefs.ordered_task_ids ?? [],
    subtaskOrderByParentId: payload.sidebar_task_prefs.subtask_order_by_parent_id ?? {},
    syncError: state.sidebarTaskPrefs.syncError,
  };
}

function buildSidebarViewsState(state: AppState, payload: UserSettingsUpdatedPayload) {
  const views = (payload.sidebar_views ?? []).map(fromApiSidebarView).map(migrateView);
  const draft = parseSidebarDraftForViews(state, payload);
  if (views.length === 0) return { ...state.sidebarViews, draft };
  const collapsedById = new Map(
    state.sidebarViews.views.map((view) => [view.id, view.collapsedGroups]),
  );
  const mergedViews = views.map((view) => ({
    ...view,
    collapsedGroups: collapsedById.get(view.id) ?? view.collapsedGroups,
  }));
  const activeViewId =
    payload.sidebar_active_view_id &&
    mergedViews.some((v) => v.id === payload.sidebar_active_view_id)
      ? payload.sidebar_active_view_id
      : state.sidebarViews.activeViewId;
  return {
    ...state.sidebarViews,
    views: mergedViews,
    activeViewId: mergedViews.some((v) => v.id === activeViewId) ? activeViewId : mergedViews[0].id,
    draft,
  };
}

function parseSidebarDraftForViews(state: AppState, payload: UserSettingsUpdatedPayload) {
  if (payload.sidebar_draft === undefined) return state.sidebarViews.draft;
  if (payload.sidebar_draft === null) return null;
  return fromApiSidebarDraft(payload.sidebar_draft);
}
