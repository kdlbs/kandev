'use client';

import { useEffect } from 'react';
import { fetchUserSettings, listEditors } from '@/lib/api';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';
import type { UserSettingsState } from '@/lib/state/slices/settings/types';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mapUserSettingsResponse(data: any): UserSettingsState {
  const s = data.settings;
  return {
    workspaceId: s.workspace_id || null,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds: s.repository_ids ?? [],
    preferredShell: s.preferred_shell || null,
    shellOptions: data.shell_options ?? [],
    defaultEditorId: s.default_editor_id || null,
    enablePreviewOnClick: s.enable_preview_on_click ?? false,
    chatSubmitKey: s.chat_submit_key ?? 'cmd_enter',
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? true,
    lspAutoStartLanguages: s.lsp_auto_start_languages ?? [],
    lspAutoInstallLanguages: s.lsp_auto_install_languages ?? [],
    lspServerConfigs: s.lsp_server_configs ?? {},
    savedLayouts: s.saved_layouts ?? [],
    loaded: true,
  };
}

export function useEditors() {
  const editors = useAppStore((state) => state.editors.items);
  const loaded = useAppStore((state) => state.editors.loaded);
  const loading = useAppStore((state) => state.editors.loading);
  const setEditors = useAppStore((state) => state.setEditors);
  const setEditorsLoading = useAppStore((state) => state.setEditorsLoading);
  const userSettingsLoaded = useAppStore((state) => state.userSettings.loaded);
  const setUserSettings = useAppStore((state) => state.setUserSettings);

  useEffect(() => {
    const client = getWebSocketClient();
    if (client) {
      client.subscribeUser();
    }
  }, []);

  useEffect(() => {
    if (loaded || loading) return;
    setEditorsLoading(true);
    listEditors({ cache: 'no-store' })
      .then((response) => { setEditors(response.editors ?? []); })
      .catch(() => { setEditors([]); })
      .finally(() => { setEditorsLoading(false); });
  }, [loaded, loading, setEditors, setEditorsLoading]);

  useEffect(() => {
    if (userSettingsLoaded) return;
    fetchUserSettings({ cache: 'no-store' })
      .then((data) => {
        if (!data?.settings) return;
        setUserSettings(mapUserSettingsResponse(data));
      })
      .catch(() => {
        // Ignore settings fetch errors for now.
      });
  }, [setUserSettings, userSettingsLoaded]);

  return { editors, loaded, loading };
}
