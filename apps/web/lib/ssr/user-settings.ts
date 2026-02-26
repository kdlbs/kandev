import type { SavedLayout, UserSettingsResponse } from "@/lib/types/http";

export type UserSettingsData = NonNullable<UserSettingsResponse["settings"]>;

export function buildCoreFields(s: UserSettingsData) {
  return {
    workspaceId: s.workspace_id || null,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds: s.repository_ids ?? [],
    preferredShell: s.preferred_shell || null,
    defaultEditorId: s.default_editor_id || null,
    enablePreviewOnClick: s.enable_preview_on_click ?? false,
    chatSubmitKey: s.chat_submit_key ?? "cmd_enter",
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? true,
    showReleaseNotification: s.show_release_notification ?? true,
    releaseNotesLastSeenVersion: s.release_notes_last_seen_version || null,
    savedLayouts: s.saved_layouts ?? [],
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
