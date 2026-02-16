'use client';

import { useEffect } from 'react';
import { fetchUserSettings, listEditors } from '@/lib/api';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useAppStore } from '@/components/state-provider';

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
      // Future: subscribe to editor update events.
      client.subscribeUser();
    }
  }, []);

  useEffect(() => {
    if (loaded || loading) {
      return;
    }
    setEditorsLoading(true);
    listEditors({ cache: 'no-store' })
      .then((response) => {
        setEditors(response.editors ?? []);
      })
      .catch(() => {
        setEditors([]);
      })
      .finally(() => {
        setEditorsLoading(false);
      });
  }, [loaded, loading, setEditors, setEditorsLoading]);

  useEffect(() => {
    if (userSettingsLoaded) return;
    fetchUserSettings({ cache: 'no-store' })
      .then((data) => {
        if (!data?.settings) return;
        setUserSettings({
          workspaceId: data.settings.workspace_id || null,
          workflowId: data.settings.workflow_filter_id || null,
          kanbanViewMode: data.settings.kanban_view_mode || null,
          repositoryIds: data.settings.repository_ids ?? [],
          preferredShell: data.settings.preferred_shell || null,
          shellOptions: data.shell_options ?? [],
          defaultEditorId: data.settings.default_editor_id || null,
          enablePreviewOnClick: data.settings.enable_preview_on_click ?? false,
          chatSubmitKey: data.settings.chat_submit_key ?? 'cmd_enter',
          reviewAutoMarkOnScroll: data.settings.review_auto_mark_on_scroll ?? true,
          lspAutoStartLanguages: data.settings.lsp_auto_start_languages ?? [],
          lspAutoInstallLanguages: data.settings.lsp_auto_install_languages ?? [],
          lspServerConfigs: data.settings.lsp_server_configs ?? {},
          loaded: true,
        });
      })
      .catch(() => {
        // Ignore settings fetch errors for now.
      });
  }, [setUserSettings, userSettingsLoaded]);

  return {
    editors,
    loaded,
    loading,
  };
}
