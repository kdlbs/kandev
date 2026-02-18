import { useCallback, useEffect, useMemo, useRef } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { fetchUserSettings, updateUserSettings } from '@/lib/api';
import { mapSelectedRepositoryIds } from '@/lib/kanban/filters';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import type { Repository } from '@/lib/types/http';
import type { UserSettingsState } from '@/lib/state/slices/settings/types';

type DisplaySettings = UserSettingsState;

type UseUserDisplaySettingsInput = {
  workspaceId: string | null;
  workflowId: string | null;
  onWorkspaceChange?: (workspaceId: string | null) => void;
  onWorkflowChange?: (workflowId: string | null) => void;
};

type CommitPayload = {
  workspaceId: string | null;
  workflowId: string | null;
  repositoryIds: string[];
  preferredShell?: string | null;
  enablePreviewOnClick?: boolean;
  kanbanViewMode?: string | null;
};

function buildNormalizedSettings(next: CommitPayload, current: DisplaySettings): DisplaySettings {
  const repositoryIds = Array.from(new Set(next.repositoryIds)).sort();
  return {
    workspaceId: next.workspaceId,
    workflowId: next.workflowId,
    kanbanViewMode: next.kanbanViewMode !== undefined ? next.kanbanViewMode : (current.kanbanViewMode ?? null),
    repositoryIds,
    preferredShell: next.preferredShell ?? current.preferredShell ?? null,
    shellOptions: current.shellOptions ?? [],
    defaultEditorId: current.defaultEditorId ?? null,
    enablePreviewOnClick: next.enablePreviewOnClick ?? current.enablePreviewOnClick,
    chatSubmitKey: current.chatSubmitKey ?? 'cmd_enter',
    reviewAutoMarkOnScroll: current.reviewAutoMarkOnScroll ?? true,
    lspAutoStartLanguages: current.lspAutoStartLanguages ?? [],
    lspAutoInstallLanguages: current.lspAutoInstallLanguages ?? [],
    lspServerConfigs: current.lspServerConfigs ?? {},
    savedLayouts: current.savedLayouts ?? [],
    loaded: true,
  };
}

function isSettingsUnchanged(normalized: DisplaySettings, current: DisplaySettings): boolean {
  if (!current.loaded) return false;
  return (
    normalized.workspaceId === current.workspaceId &&
    normalized.workflowId === current.workflowId &&
    normalized.enablePreviewOnClick === current.enablePreviewOnClick &&
    normalized.kanbanViewMode === current.kanbanViewMode &&
    normalized.repositoryIds.length === current.repositoryIds.length &&
    normalized.repositoryIds.every((id, index) => id === current.repositoryIds[index])
  );
}

function persistSettingsPayload(payload: Record<string, unknown>) {
  const client = getWebSocketClient();
  if (!client) {
    updateUserSettings(payload, { cache: 'no-store' }).catch(() => { /* ignore */ });
    return;
  }
  client.request('user.settings.update', payload).catch(() => {
    updateUserSettings(payload, { cache: 'no-store' }).catch(() => { /* ignore */ });
  });
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mapFetchedUserSettings(data: any): DisplaySettings {
  const s = data.settings;
  const repositoryIds = Array.from(new Set<string>(s.repository_ids ?? [])).sort();
  return {
    workspaceId: s.workspace_id || null,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds,
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

export function useUserDisplaySettings({
  workspaceId,
  workflowId,
  onWorkspaceChange,
  onWorkflowChange,
}: UseUserDisplaySettingsInput) {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, true);
  const userSettingsRef = useRef(userSettings);
  useEffect(() => { userSettingsRef.current = userSettings; });

  const settingsLoadedOnMountRef = useRef(userSettings.loaded);

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const current = userSettingsRef.current;
      const normalized = buildNormalizedSettings(next, current);
      if (isSettingsUnchanged(normalized, current)) return;
      setUserSettings(normalized);
      const payload = {
        workspace_id: normalized.workspaceId ?? '',
        workflow_filter_id: normalized.workflowId ?? '',
        repository_ids: normalized.repositoryIds,
        enable_preview_on_click: normalized.enablePreviewOnClick,
        kanban_view_mode: normalized.kanbanViewMode ?? '',
      };
      persistSettingsPayload(payload);
    },
    [setUserSettings]
  );

  useEffect(() => {
    if (userSettings.loaded) return;
    fetchUserSettings({ cache: 'no-store' })
      .then((data) => {
        if (!data?.settings) return;
        setUserSettings(mapFetchedUserSettings(data));
      })
      .catch(() => { /* Ignore settings fetch errors for now. */ });
  }, [setUserSettings, userSettings.loaded]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (settingsLoadedOnMountRef.current) return;
    settingsLoadedOnMountRef.current = true;
    if (userSettings.workspaceId && userSettings.workspaceId !== workspaceId) {
      onWorkspaceChange?.(userSettings.workspaceId);
    }
  }, [onWorkspaceChange, userSettings.loaded, userSettings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!userSettings.loaded || !(!userSettings.workspaceId && workspaceId)) return;
    queueMicrotask(() => {
      commitSettings({ workspaceId, workflowId: userSettings.workflowId, repositoryIds: userSettings.repositoryIds });
    });
  }, [commitSettings, userSettings.workflowId, userSettings.loaded, userSettings.repositoryIds, userSettings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (settingsLoadedOnMountRef.current) return;
    if (userSettings.workflowId && userSettings.workflowId !== workflowId) {
      onWorkflowChange?.(userSettings.workflowId);
    }
  }, [workflowId, onWorkflowChange, userSettings.workflowId, userSettings.loaded]);

  useEffect(() => {
    if (!userSettings.loaded || repositories.length === 0) return;
    const repoIds = repositories.map((repo: Repository) => repo.id);
    const validIds = userSettings.repositoryIds.filter((id: string) => repoIds.includes(id));
    const isSame = validIds.length === userSettings.repositoryIds.length &&
      validIds.every((id: string, index: number) => id === userSettings.repositoryIds[index]);
    if (!isSame) {
      queueMicrotask(() => {
        commitSettings({ workspaceId: userSettings.workspaceId, workflowId: userSettings.workflowId, repositoryIds: validIds });
      });
    }
  }, [commitSettings, repositories, userSettings.workflowId, userSettings.loaded, userSettings.repositoryIds, userSettings.workspaceId]);

  const allRepositoriesSelected = userSettings.repositoryIds.length === 0;
  const selectedRepositoryIds = useMemo(
    () => mapSelectedRepositoryIds(repositories, userSettings.repositoryIds),
    [repositories, userSettings.repositoryIds]
  );

  return { settings: userSettings, commitSettings, repositories, repositoriesLoading, allRepositoriesSelected, selectedRepositoryIds };
}
