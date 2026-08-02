import {
  DEFAULT_TASKS_LIST_GROUP,
  DEFAULT_TASKS_LIST_SORT,
  parseTasksListGroup,
  parseTasksListSort,
} from "@/lib/tasks/tasks-list-options";
import { fromApiSidebarDraft, fromApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import type { SidebarView, SidebarViewDraft } from "@/lib/state/slices/ui/sidebar-view-types";
import {
  DEFAULT_VOICE_MODE_STATE,
  type UserSettingsState,
  type VoiceModeState,
} from "@/lib/state/slices/settings/types";
import type { SidebarTaskPrefsApi, UserSettings, UserSettingsResponse } from "@/lib/types/http";
import type { MCPTaskAgentProfileDefault, StartupPage } from "@/lib/types/http-user-settings";
import type { VoiceModeSettings } from "@/lib/types/http-voice";

export type UserSettingsData = Omit<Partial<UserSettings>, "workspace_id"> & {
  workspace_id?: string;
};

export function createDefaultUserSettings(): UserSettingsState {
  return {
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
    unreadDivider: false,
    agentGeneratedTaskTitles: false,
    mcpTaskAgentProfileDefault: "current_task",
    showAnchoredPromptBar: false,
    showScrollToLastPrompt: true,
    showScrollToStart: false,
    showTranscriptAutoScrollControl: false,
    showReleaseNotification: true,
    releaseNotesLastSeenVersion: null,
    lspAutoStartLanguages: [],
    lspAutoInstallLanguages: [],
    lspServerConfigs: {},
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
      synced: false,
    },
    jiraSavedViews: undefined,
    jiraTaskPresets: undefined,
    githubSavedPresets: undefined,
    githubDefaultQueryPresets: undefined,
    gitlabSavedPresets: undefined,
    defaultUtilityAgentId: null,
    keyboardShortcuts: {},
    terminalLinkBehavior: "new_tab",
    terminalFontFamily: null,
    terminalFontSize: null,
    changesPanelLayout: "tree",
    systemMetricsDisplay: { showInTopbar: false, simplified: false },
    appStatusBarOrder: { leftItemIds: [], rightItemIds: [] },
    voiceMode: { ...DEFAULT_VOICE_MODE_STATE },
    loaded: false,
  };
}

export function parseTerminalLinkBehavior(value: string | undefined): "new_tab" | "browser_panel" {
  return value === "browser_panel" ? "browser_panel" : "new_tab";
}

export function parseChangesPanelLayout(value: string | undefined): "flat" | "tree" {
  return value === "flat" ? "flat" : "tree";
}

export function parseMCPTaskAgentProfileDefault(
  value: string | undefined,
): MCPTaskAgentProfileDefault {
  return value === "workspace_default" ? "workspace_default" : "current_task";
}

export function parseStartupPage(value: string | undefined): StartupPage {
  return value === "last_task" ? "last_task" : "task_overview";
}

export function parseSystemMetricsDisplay(value: UserSettingsData["system_metrics_display"]) {
  return {
    showInTopbar: value?.show_in_topbar ?? false,
    simplified: value?.simplified ?? false,
  };
}

export function parseAppStatusBarOrder(value: UserSettingsData["app_status_bar_order"]) {
  return {
    leftItemIds: value?.left_item_ids ?? [],
    rightItemIds: value?.right_item_ids ?? [],
  };
}

/**
 * Maps the backend's snake_case VoiceMode payload into the camelCase shape
 * the store and UI use. Missing or partial payloads fall back to the defaults
 * so an old user row (written before VoiceMode existed) doesn't surface as
 * an empty string the radio groups can't render. `enabled` defaults to true
 * for users who haven't toggled it — voice mode is opt-out, not opt-in.
 */
export function parseVoiceMode(value: VoiceModeSettings | undefined): VoiceModeState {
  if (!value) return { ...DEFAULT_VOICE_MODE_STATE };
  return {
    enabled: typeof value.enabled === "boolean" ? value.enabled : true,
    engine: value.engine || DEFAULT_VOICE_MODE_STATE.engine,
    language: value.language || DEFAULT_VOICE_MODE_STATE.language,
    mode: value.mode || DEFAULT_VOICE_MODE_STATE.mode,
    autoSend: typeof value.auto_send === "boolean" ? value.auto_send : false,
    whisperWebModel: value.whisper_web_model || DEFAULT_VOICE_MODE_STATE.whisperWebModel,
  };
}

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

function buildVoiceModeFields(s: UserSettingsData, current: UserSettingsState) {
  return {
    voiceMode: s.voice_mode === undefined ? current.voiceMode : parseVoiceMode(s.voice_mode),
  };
}

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

function parseSidebarTaskPrefs(value: SidebarTaskPrefsApi | undefined) {
  return {
    pinnedTaskIds: value?.pinned_task_ids ?? [],
    orderedTaskIds: value?.ordered_task_ids ?? [],
    subtaskOrderByParentId: value?.subtask_order_by_parent_id ?? {},
  };
}

export function taskCreateLastUsedHasValue(
  value: UserSettingsData["task_create_last_used"] | undefined,
) {
  return Boolean(
    value?.repository_id || value?.branch || value?.agent_profile_id || value?.executor_profile_id,
  );
}

function parseTaskCreateLastUsed(value: UserSettingsData["task_create_last_used"] | undefined) {
  return {
    repositoryId: value?.repository_id || null,
    branch: value?.branch || null,
    agentProfileId: value?.agent_profile_id || null,
    executorProfileId: value?.executor_profile_id || null,
    synced: taskCreateLastUsedHasValue(value),
  };
}

function mapDefined<TInput, TOutput>(
  value: TInput | undefined,
  current: TOutput,
  map: (defined: TInput) => TOutput,
): TOutput {
  return value === undefined ? current : map(value);
}

function mapNullableString(value: string | undefined, current: string | null) {
  return mapDefined(value, current, (defined) => defined || null);
}

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

function buildBehaviorFields(s: UserSettingsData, current: UserSettingsState) {
  return {
    enablePreviewOnClick: s.enable_preview_on_click ?? current.enablePreviewOnClick,
    chatSubmitKey: s.chat_submit_key ?? current.chatSubmitKey,
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? current.reviewAutoMarkOnScroll,
    confirmTaskArchive: s.confirm_task_archive ?? current.confirmTaskArchive,
    unreadDivider: s.unread_divider ?? current.unreadDivider,
    agentGeneratedTaskTitles: s.agent_generated_task_titles ?? false,
    mcpTaskAgentProfileDefault: mapDefined(
      s.mcp_task_agent_profile_default,
      current.mcpTaskAgentProfileDefault,
      parseMCPTaskAgentProfileDefault,
    ),
    startupPage: mapDefined(s.startup_page, current.startupPage, parseStartupPage),
    showAnchoredPromptBar: s.show_anchored_prompt_bar ?? current.showAnchoredPromptBar,
    showScrollToLastPrompt: s.show_scroll_to_last_prompt ?? current.showScrollToLastPrompt,
    showScrollToStart: s.show_scroll_to_start ?? current.showScrollToStart,
    showTranscriptAutoScrollControl:
      s.show_transcript_auto_scroll_control ?? current.showTranscriptAutoScrollControl,
    showReleaseNotification: s.show_release_notification ?? current.showReleaseNotification,
    releaseNotesLastSeenVersion: mapNullableString(
      s.release_notes_last_seen_version,
      current.releaseNotesLastSeenVersion,
    ),
    keyboardShortcuts: s.keyboard_shortcuts ?? current.keyboardShortcuts,
  };
}

export function buildCoreFields(
  s: UserSettingsData,
  current: UserSettingsState = createDefaultUserSettings(),
) {
  return {
    ...buildIdentityFields(s, current),
    ...buildBehaviorFields(s, current),
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
    appStatusBarOrder: mapDefined(
      s.app_status_bar_order,
      current.appStatusBarOrder,
      parseAppStatusBarOrder,
    ),
    ...buildTerminalFields(s, current),
    ...buildSystemMetricsDisplayFields(s, current),
    ...buildVoiceModeFields(s, current),
  };
}

export function buildLspFields(
  s: UserSettingsData | undefined,
  current: UserSettingsState = createDefaultUserSettings(),
) {
  return {
    lspAutoStartLanguages: s?.lsp_auto_start_languages ?? current.lspAutoStartLanguages,
    lspAutoInstallLanguages: s?.lsp_auto_install_languages ?? current.lspAutoInstallLanguages,
    lspServerConfigs: s?.lsp_server_configs ?? current.lspServerConfigs,
  };
}

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
    shellOptions,
  };
}
