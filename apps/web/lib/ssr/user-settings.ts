import {
  DEFAULT_TASKS_LIST_GROUP,
  DEFAULT_TASKS_LIST_SORT,
  parseTasksListGroup,
  parseTasksListSort,
} from "@/lib/tasks/tasks-list-options";
import { fromApiSidebarDraft, fromApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import type { SidebarView, SidebarViewDraft } from "@/lib/state/slices/ui/sidebar-view-types";
import { DEFAULT_VOICE_MODE_STATE, type VoiceModeState } from "@/lib/state/slices/settings/types";
import type { SavedLayout, SidebarTaskPrefsApi, UserSettingsResponse } from "@/lib/types/http";
import type { MCPTaskAgentProfileDefault } from "@/lib/types/http-user-settings";
import type { VoiceModeSettings } from "@/lib/types/http-voice";

export type UserSettingsData = NonNullable<UserSettingsResponse["settings"]>;

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

function buildTerminalFields(s: UserSettingsData) {
  return {
    terminalLinkBehavior: parseTerminalLinkBehavior(s.terminal_link_behavior),
    terminalFontFamily: s.terminal_font_family || null,
    terminalFontSize: s.terminal_font_size || null,
    changesPanelLayout: parseChangesPanelLayout(s.changes_panel_layout),
  };
}

function buildVoiceModeFields(s: UserSettingsData) {
  return { voiceMode: parseVoiceMode(s.voice_mode) };
}

function buildSystemMetricsDisplayFields(s: UserSettingsData | undefined) {
  return {
    systemMetricsDisplay: parseSystemMetricsDisplay(s?.system_metrics_display),
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

function buildIdentityFields(s: UserSettingsData) {
  return {
    workspaceId: s.workspace_id || null,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds: s.repository_ids ?? [],
    tasksListSort: parseTasksListSort(s.tasks_list_sort),
    tasksListGroup: parseTasksListGroup(s.tasks_list_group),
    preferredShell: s.preferred_shell || null,
    defaultEditorId: s.default_editor_id || null,
    defaultUtilityAgentId: s.default_utility_agent_id || null,
  };
}

function buildBehaviorFields(s: UserSettingsData) {
  return {
    enablePreviewOnClick: s.enable_preview_on_click ?? false,
    chatSubmitKey: s.chat_submit_key ?? "cmd_enter",
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? true,
    confirmTaskArchive: s.confirm_task_archive ?? true,
    mcpTaskAgentProfileDefault: parseMCPTaskAgentProfileDefault(s.mcp_task_agent_profile_default),
    showReleaseNotification: s.show_release_notification ?? true,
    releaseNotesLastSeenVersion: s.release_notes_last_seen_version || null,
    keyboardShortcuts: s.keyboard_shortcuts ?? {},
  };
}

export function buildCoreFields(s: UserSettingsData) {
  return {
    ...buildIdentityFields(s),
    ...buildBehaviorFields(s),
    savedLayouts: s.saved_layouts ?? [],
    sidebarViews: (s.sidebar_views ?? []).map(fromApiSidebarView) as SidebarView[],
    sidebarActiveViewId: s.sidebar_active_view_id || null,
    sidebarDraft: s.sidebar_draft
      ? (fromApiSidebarDraft(s.sidebar_draft) as SidebarViewDraft)
      : null,
    sidebarTaskPrefs: parseSidebarTaskPrefs(s.sidebar_task_prefs),
    taskCreateLastUsed: parseTaskCreateLastUsed(s.task_create_last_used),
    jiraSavedViews: s.jira_saved_views,
    jiraTaskPresets: s.jira_task_presets,
    githubSavedPresets: s.github_saved_presets,
    githubDefaultQueryPresets: s.github_default_query_presets,
    gitlabSavedPresets: s.gitlab_saved_presets,
    appStatusBarOrder: parseAppStatusBarOrder(s.app_status_bar_order),
    ...buildTerminalFields(s),
    ...buildSystemMetricsDisplayFields(s),
    ...buildVoiceModeFields(s),
  };
}

export function buildLspFields(s: UserSettingsData | undefined) {
  return {
    lspAutoStartLanguages: s?.lsp_auto_start_languages ?? [],
    lspAutoInstallLanguages: s?.lsp_auto_install_languages ?? [],
    lspServerConfigs: s?.lsp_server_configs ?? {},
  };
}

/**
 * Maps a `fetchUserSettings()` API response into the shape expected by `AppState["userSettings"]`.
 * Use in SSR pages to build `initialState.userSettings`.
 */
export function mapUserSettingsResponse(response: UserSettingsResponse | null) {
  const s = response?.settings;
  const shellOptions = response?.shell_options ?? [];
  if (!s) {
    return {
      workspaceId: null,
      workflowId: null,
      kanbanViewMode: null,
      repositoryIds: [] as string[],
      tasksListSort: DEFAULT_TASKS_LIST_SORT,
      tasksListGroup: DEFAULT_TASKS_LIST_GROUP,
      preferredShell: null,
      shellOptions,
      defaultEditorId: null,
      enablePreviewOnClick: false,
      chatSubmitKey: "cmd_enter" as const,
      reviewAutoMarkOnScroll: true,
      confirmTaskArchive: true,
      mcpTaskAgentProfileDefault: "current_task" as const,
      showReleaseNotification: true,
      releaseNotesLastSeenVersion: null,
      savedLayouts: [] as SavedLayout[],
      sidebarViews: [] as SidebarView[],
      sidebarActiveViewId: null,
      sidebarDraft: null,
      sidebarTaskPrefs: parseSidebarTaskPrefs(undefined),
      taskCreateLastUsed: parseTaskCreateLastUsed(undefined),
      jiraSavedViews: undefined,
      jiraTaskPresets: undefined,
      githubSavedPresets: undefined,
      githubDefaultQueryPresets: undefined,
      gitlabSavedPresets: undefined,
      defaultUtilityAgentId: null,
      keyboardShortcuts: {} as Record<string, { key: string; modifiers?: Record<string, boolean> }>,
      terminalLinkBehavior: "new_tab" as const,
      terminalFontFamily: null,
      terminalFontSize: null,
      changesPanelLayout: "tree" as const,
      appStatusBarOrder: parseAppStatusBarOrder(undefined),
      ...buildSystemMetricsDisplayFields(undefined),
      voiceMode: { ...DEFAULT_VOICE_MODE_STATE },
      ...buildLspFields(undefined),
      loaded: false,
    };
  }
  return {
    ...buildCoreFields(s),
    shellOptions,
    ...buildLspFields(s),
    loaded: true,
  };
}
