import {
  DEFAULT_TASKS_LIST_GROUP,
  DEFAULT_TASKS_LIST_SORT,
  parseTasksListGroup,
  parseTasksListSort,
} from "@/lib/tasks/tasks-list-options";
import { fromApiSidebarDraft, fromApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import type { SidebarView, SidebarViewDraft } from "@/lib/state/slices/ui/sidebar-view-types";
import { type UserSettingsState } from "@/lib/state/slices/settings/types";
import type { SidebarTaskPrefsApi, UserSettings, UserSettingsResponse } from "@/lib/types/http";
import type {
  LspStatusLocation,
  LastSeenDisplay,
  MCPTaskAgentProfileDefault,
  StartupPage,
} from "@/lib/types/http-user-settings";

export type UserSettingsData = Omit<Partial<UserSettings>, "workspace_id"> & {
  workspace_id?: string;
};

/** Builds a fresh UserSettingsState with default values. */
export function createDefaultUserSettings(): UserSettingsState {
  return {
    revision: null,
    workspaceId: null,
    workflowId: null,
    kanbanViewMode: null,
    startupPage: "task_overview",
    repositoryIds: [],
    tasksListSort: DEFAULT_TASKS_LIST_SORT,
    tasksListGroup: DEFAULT_TASKS_LIST_GROUP,
    tasksListShowDetails: false,
    preferredShell: null,
    shellOptions: [],
    defaultEditorId: null,
    enablePreviewOnClick: false,
    chatSubmitKey: "cmd_enter",
    reviewAutoMarkOnScroll: true,
    confirmTaskArchive: true,
    preventAutoStartAgentOnOpen: false,
    unreadDivider: false,
    agentGeneratedTaskTitles: true,
    mcpTaskAgentProfileDefault: "current_task",
    showAnchoredPromptBar: false,
    showScrollToLastPrompt: true,
    showScrollToStart: false,
    showTranscriptAutoScrollControl: false,
    showTodoListPanel: false,
    showTodoListPanelOnlyWhenNotEmpty: false,
    showReleaseNotification: true,
    releaseNotesLastSeenVersion: null,
    lspAutoStartLanguages: [],
    lspAutoInstallLanguages: [],
    lspServerConfigs: {},
    lspStatusLocation: "toolbar",
    savedLayouts: [],
    sidebarViews: [],
    sidebarActiveViewId: null,
    sidebarDraft: null,
    sidebarTaskPrefs: { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} },
    taskCreateLastUsed: {
      repositoryId: null,
      branch: null,
      agentProfileId: null,
      executorProfileId: null,
      workflowIdsByWorkspace: {},
      synced: false,
    },
    jiraSavedViews: undefined,
    jiraTaskPresets: undefined,
    githubSavedPresets: undefined,
    githubDefaultQueryPresets: undefined,
    gitlabSavedPresets: undefined,
    azureDevOpsBrowsePreferences: undefined,
    defaultUtilityAgentId: null,
    keyboardShortcuts: {},
    terminalLinkBehavior: "new_tab",
    terminalFontFamily: null,
    terminalFontSize: null,
    changesPanelLayout: "tree",
    lastSeenDisplay: "absolute",
    systemMetricsDisplay: { showInTopbar: false, simplified: false },
    appStatusBarEnabled: false,
    appStatusBarOrder: { leftItemIds: [], rightItemIds: [] },
    quickChatTabOrderByWorkspace: {},
    hiddenWorkflowStepIds: {},
    workflowIdsWithAutoHideEmptySteps: [],
    loaded: false,
  };
}

/** Parses the terminal link behavior, defaulting to "new_tab". */
export function parseTerminalLinkBehavior(value: string | undefined): "new_tab" | "browser_panel" {
  return value === "browser_panel" ? "browser_panel" : "new_tab";
}

/** Parses the changes panel layout, defaulting to "tree". */
export function parseChangesPanelLayout(value: string | undefined): "flat" | "tree" {
  return value === "flat" ? "flat" : "tree";
}

/** Parses the last-seen display format, defaulting to "absolute". */
export function parseLastSeenDisplay(value: string | undefined): LastSeenDisplay {
  return value === "relative" ? "relative" : "absolute";
}

/** Parses the MCP task agent profile default, defaulting to "current_task". */
export function parseMCPTaskAgentProfileDefault(
  value: string | undefined,
): MCPTaskAgentProfileDefault {
  return value === "workspace_default" ? "workspace_default" : "current_task";
}

/** Parses the startup page preference, defaulting to "task_overview". */
export function parseStartupPage(value: string | undefined): StartupPage {
  return value === "last_task" ? "last_task" : "task_overview";
}

/** Parses the LSP status location, defaulting to "toolbar". */
export function parseLspStatusLocation(value: string | undefined): LspStatusLocation {
  return value === "status_bar" ? "status_bar" : "toolbar";
}

/** Parses the system metrics display fields with defaults. */
export function parseSystemMetricsDisplay(value: UserSettingsData["system_metrics_display"]) {
  return {
    showInTopbar: value?.show_in_topbar ?? false,
    simplified: value?.simplified ?? false,
  };
}

/** Parses the app status bar order fields with empty defaults. */
export function parseAppStatusBarOrder(value: UserSettingsData["app_status_bar_order"]) {
  return {
    leftItemIds: value?.left_item_ids ?? [],
    rightItemIds: value?.right_item_ids ?? [],
  };
}

/** Maps terminal-related API fields onto state, falling back to current values. */
function buildTerminalFields(s: UserSettingsData, current: UserSettingsState) {
  return {
    terminalLinkBehavior:
      s.terminal_link_behavior === undefined
        ? current.terminalLinkBehavior
        : parseTerminalLinkBehavior(s.terminal_link_behavior),
    terminalFontFamily:
      s.terminal_font_family === undefined
        ? current.terminalFontFamily
        : s.terminal_font_family || null,
    terminalFontSize:
      s.terminal_font_size === undefined ? current.terminalFontSize : s.terminal_font_size || null,
    changesPanelLayout:
      s.changes_panel_layout === undefined
        ? current.changesPanelLayout
        : parseChangesPanelLayout(s.changes_panel_layout),
  };
}

/** Maps the system metrics display field onto state, falling back to current. */
function buildSystemMetricsDisplayFields(
  s: UserSettingsData | undefined,
  current: UserSettingsState,
) {
  return {
    systemMetricsDisplay:
      s?.system_metrics_display === undefined
        ? current.systemMetricsDisplay
        : parseSystemMetricsDisplay(s.system_metrics_display),
  };
}

/** Parses sidebar task preferences with empty-array defaults. */
function parseSidebarTaskPrefs(value: SidebarTaskPrefsApi | undefined) {
  return {
    pinnedTaskIds: value?.pinned_task_ids ?? [],
    orderedTaskIds: value?.ordered_task_ids ?? [],
    subtaskOrderByParentId: value?.subtask_order_by_parent_id ?? {},
  };
}

/** Returns true when the task-create last-used payload carries any value. */
export function taskCreateLastUsedHasValue(
  value: UserSettingsData["task_create_last_used"] | undefined,
) {
  return Boolean(
    value?.repository_id ||
    value?.branch ||
    value?.agent_profile_id ||
    value?.executor_profile_id ||
    Object.keys(value?.workflow_ids_by_workspace ?? {}).length > 0,
  );
}

/** Parses task-create last-used fields, deriving the synced flag from presence. */
function parseTaskCreateLastUsed(value: UserSettingsData["task_create_last_used"] | undefined) {
  return {
    repositoryId: value?.repository_id || null,
    branch: value?.branch || null,
    agentProfileId: value?.agent_profile_id || null,
    executorProfileId: value?.executor_profile_id || null,
    workflowIdsByWorkspace: value?.workflow_ids_by_workspace ?? {},
    synced: taskCreateLastUsedHasValue(value),
  };
}

/** Returns current when value is undefined, otherwise maps it. */
function mapDefined<TInput, TOutput>(
  value: TInput | undefined,
  current: TOutput,
  map: (defined: TInput) => TOutput,
): TOutput {
  return value === undefined ? current : map(value);
}

/** Maps an optional string, normalizing empty strings to null. */
function mapNullableString(value: string | undefined, current: string | null) {
  return mapDefined(value, current, (defined) => defined || null);
}

/** Maps identity-related API fields onto state, falling back to current values. */
function buildIdentityFields(s: UserSettingsData, current: UserSettingsState) {
  return {
    workspaceId: mapNullableString(s.workspace_id, current.workspaceId),
    workflowId: mapNullableString(s.workflow_filter_id, current.workflowId),
    kanbanViewMode: mapNullableString(s.kanban_view_mode, current.kanbanViewMode),
    repositoryIds: s.repository_ids ?? current.repositoryIds,
    tasksListSort: mapDefined(s.tasks_list_sort, current.tasksListSort, parseTasksListSort),
    tasksListGroup: mapDefined(s.tasks_list_group, current.tasksListGroup, parseTasksListGroup),
    tasksListShowDetails: s.tasks_list_show_details ?? current.tasksListShowDetails,
    preferredShell: mapNullableString(s.preferred_shell, current.preferredShell),
    defaultEditorId: mapNullableString(s.default_editor_id, current.defaultEditorId),
    defaultUtilityAgentId: mapNullableString(
      s.default_utility_agent_id,
      current.defaultUtilityAgentId,
    ),
  };
}

/** Maps behavior-related API fields onto state, falling back to current values. */
function buildBehaviorFields(s: UserSettingsData, current: UserSettingsState) {
  return {
    enablePreviewOnClick: s.enable_preview_on_click ?? current.enablePreviewOnClick,
    chatSubmitKey: s.chat_submit_key ?? current.chatSubmitKey,
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? current.reviewAutoMarkOnScroll,
    confirmTaskArchive: s.confirm_task_archive ?? current.confirmTaskArchive,
    preventAutoStartAgentOnOpen:
      s.prevent_auto_start_agent_on_open ?? current.preventAutoStartAgentOnOpen,
    unreadDivider: s.unread_divider ?? current.unreadDivider,
    agentGeneratedTaskTitles: s.agent_generated_task_titles ?? current.agentGeneratedTaskTitles,
    mcpTaskAgentProfileDefault: mapDefined(
      s.mcp_task_agent_profile_default,
      current.mcpTaskAgentProfileDefault,
      parseMCPTaskAgentProfileDefault,
    ),
    startupPage: mapDefined(s.startup_page, current.startupPage, parseStartupPage),
    keyboardShortcuts: s.keyboard_shortcuts ?? current.keyboardShortcuts,
  };
}

/** Maps appearance-related API fields onto state, falling back to current values. */
function buildAppearanceFields(s: UserSettingsData, current: UserSettingsState) {
  return {
    showAnchoredPromptBar: s.show_anchored_prompt_bar ?? current.showAnchoredPromptBar,
    showScrollToLastPrompt: s.show_scroll_to_last_prompt ?? current.showScrollToLastPrompt,
    showScrollToStart: s.show_scroll_to_start ?? current.showScrollToStart,
    showTranscriptAutoScrollControl:
      s.show_transcript_auto_scroll_control ?? current.showTranscriptAutoScrollControl,
    showTodoListPanel: s.show_todo_list_panel ?? current.showTodoListPanel,
    showTodoListPanelOnlyWhenNotEmpty:
      s.show_todo_list_panel_only_when_not_empty ?? current.showTodoListPanelOnlyWhenNotEmpty,
    showReleaseNotification: s.show_release_notification ?? current.showReleaseNotification,
    releaseNotesLastSeenVersion: mapNullableString(
      s.release_notes_last_seen_version,
      current.releaseNotesLastSeenVersion,
    ),
    lastSeenDisplay: mapDefined(s.last_seen_display, current.lastSeenDisplay, parseLastSeenDisplay),
  };
}

/** Maps the core user-settings API fields onto state, falling back to current values. */
export function buildCoreFields(
  s: UserSettingsData,
  current: UserSettingsState = createDefaultUserSettings(),
) {
  return {
    revision: s.revision ?? current.revision,
    ...buildIdentityFields(s, current),
    ...buildBehaviorFields(s, current),
    ...buildAppearanceFields(s, current),
    savedLayouts: s.saved_layouts ?? current.savedLayouts,
    sidebarViews: mapDefined(s.sidebar_views, current.sidebarViews, (views) =>
      views.map(fromApiSidebarView),
    ) as SidebarView[],
    sidebarActiveViewId: mapNullableString(s.sidebar_active_view_id, current.sidebarActiveViewId),
    sidebarDraft: mapDefined(s.sidebar_draft, current.sidebarDraft, (draft) =>
      draft ? (fromApiSidebarDraft(draft) as SidebarViewDraft) : null,
    ),
    sidebarTaskPrefs: mapDefined(
      s.sidebar_task_prefs,
      current.sidebarTaskPrefs,
      parseSidebarTaskPrefs,
    ),
    taskCreateLastUsed: mapDefined(
      s.task_create_last_used,
      current.taskCreateLastUsed,
      parseTaskCreateLastUsed,
    ),
    jiraSavedViews: mapDefined(s.jira_saved_views, current.jiraSavedViews, (value) => value),
    jiraTaskPresets: mapDefined(s.jira_task_presets, current.jiraTaskPresets, (value) => value),
    githubSavedPresets: mapDefined(
      s.github_saved_presets,
      current.githubSavedPresets,
      (value) => value,
    ),
    githubDefaultQueryPresets: mapDefined(
      s.github_default_query_presets,
      current.githubDefaultQueryPresets,
      (value) => value,
    ),
    gitlabSavedPresets: mapDefined(
      s.gitlab_saved_presets,
      current.gitlabSavedPresets,
      (value) => value,
    ),
    azureDevOpsBrowsePreferences: mapDefined(
      s.azure_devops_browse_preferences,
      current.azureDevOpsBrowsePreferences,
      (value) => value,
    ),
    appStatusBarOrder: mapDefined(
      s.app_status_bar_order,
      current.appStatusBarOrder,
      parseAppStatusBarOrder,
    ),
    appStatusBarEnabled: s.app_status_bar_enabled ?? current.appStatusBarEnabled,
    quickChatTabOrderByWorkspace:
      s.quick_chat_tab_order_by_workspace ?? current.quickChatTabOrderByWorkspace,
    hiddenWorkflowStepIds: s.kanban_hidden_step_ids ?? current.hiddenWorkflowStepIds,
    workflowIdsWithAutoHideEmptySteps:
      s.workflow_ids_with_auto_hide_empty_steps ?? current.workflowIdsWithAutoHideEmptySteps,
    ...buildTerminalFields(s, current),
    ...buildSystemMetricsDisplayFields(s, current),
  };
}

/** Maps LSP-related API fields onto state, falling back to current values. */
export function buildLspFields(
  s: UserSettingsData | undefined,
  current: UserSettingsState = createDefaultUserSettings(),
) {
  return {
    lspAutoStartLanguages: s?.lsp_auto_start_languages ?? current.lspAutoStartLanguages,
    lspAutoInstallLanguages: s?.lsp_auto_install_languages ?? current.lspAutoInstallLanguages,
    lspServerConfigs: s?.lsp_server_configs ?? current.lspServerConfigs,
    lspStatusLocation:
      s?.lsp_status_location === undefined
        ? current.lspStatusLocation
        : parseLspStatusLocation(s.lsp_status_location),
  };
}

/** Maps API settings data onto a complete UserSettingsState, marking it loaded. */
export function mapUserSettingsData(
  settings: UserSettingsData,
  current: UserSettingsState = createDefaultUserSettings(),
): UserSettingsState {
  return {
    ...current,
    ...buildCoreFields(settings, current),
    ...buildLspFields(settings, current),
    loaded: true,
  };
}

/**
 * Maps a `fetchUserSettings()` API response into the shape expected by `AppState["userSettings"]`.
 * Use in SSR pages to build `initialState.userSettings`.
 */
export function mapUserSettingsResponse(
  response: UserSettingsResponse | null,
  current: UserSettingsState = createDefaultUserSettings(),
) {
  const s = response?.settings;
  const shellOptions = response?.shell_options ?? current.shellOptions;
  if (!s) {
    return { ...current, shellOptions, loaded: false };
  }
  return {
    ...mapUserSettingsData(s, current),
    revision: s.revision ?? null,
    shellOptions,
  };
}
