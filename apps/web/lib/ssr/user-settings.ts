import { fromApiSidebarView } from "@/lib/state/slices/ui/sidebar-view-wire";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { DEFAULT_VOICE_MODE_STATE, type VoiceModeState } from "@/lib/state/slices/settings/types";
import type { SavedLayout, UserSettingsResponse } from "@/lib/types/http";
import type { VoiceModeSettings } from "@/lib/types/http-voice";

export type UserSettingsData = NonNullable<UserSettingsResponse["settings"]>;

export function parseTerminalLinkBehavior(value: string | undefined): "new_tab" | "browser_panel" {
  return value === "browser_panel" ? "browser_panel" : "new_tab";
}

export function parseChangesPanelLayout(value: string | undefined): "flat" | "tree" {
  return value === "flat" ? "flat" : "tree";
}

export function parseSystemMetricsDisplay(value: UserSettingsData["system_metrics_display"]) {
  return { showInTopbar: value?.show_in_topbar ?? false };
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

function buildIdentityFields(s: UserSettingsData) {
  return {
    workspaceId: s.workspace_id || null,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds: s.repository_ids ?? [],
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
      preferredShell: null,
      shellOptions,
      defaultEditorId: null,
      enablePreviewOnClick: false,
      chatSubmitKey: "cmd_enter" as const,
      reviewAutoMarkOnScroll: true,
      showReleaseNotification: true,
      releaseNotesLastSeenVersion: null,
      savedLayouts: [] as SavedLayout[],
      sidebarViews: [] as SidebarView[],
      defaultUtilityAgentId: null,
      keyboardShortcuts: {} as Record<string, { key: string; modifiers?: Record<string, boolean> }>,
      terminalLinkBehavior: "new_tab" as const,
      terminalFontFamily: null,
      terminalFontSize: null,
      changesPanelLayout: "tree" as const,
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
