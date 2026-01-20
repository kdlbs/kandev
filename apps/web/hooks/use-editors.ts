'use client';

import { useEffect } from 'react';
import { fetchUserSettings, listEditors } from '@/lib/http';
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
          boardId: data.settings.board_id || null,
          repositoryIds: data.settings.repository_ids ?? [],
          preferredShell: data.settings.preferred_shell || null,
          shellOptions: data.shell_options ?? [],
          defaultEditorId: data.settings.default_editor_id || null,
          enablePreviewOnClick: data.settings.enable_preview_on_click ?? false,
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
