import { useCallback, useEffect, useMemo, useRef } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { fetchUserSettings, updateUserSettings } from '@/lib/api';
import { mapSelectedRepositoryIds } from '@/lib/kanban/filters';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import type { Repository } from '@/lib/types/http';

type DisplaySettings = {
  workspaceId: string | null;
  workflowId: string | null;
  kanbanViewMode: string | null;
  repositoryIds: string[];
  preferredShell: string | null;
  shellOptions: Array<{ value: string; label: string }>;
  defaultEditorId: string | null;
  enablePreviewOnClick: boolean;
  chatSubmitKey: 'enter' | 'cmd_enter';
  reviewAutoMarkOnScroll: boolean;
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  lspServerConfigs: Record<string, Record<string, unknown>>;
  loaded: boolean;
};

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

export function useUserDisplaySettings({
  workspaceId,
  workflowId,
  onWorkspaceChange,
  onWorkflowChange,
}: UseUserDisplaySettingsInput) {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, true);
  // Use a ref for userSettings inside commitSettings to keep the callback stable.
  // This prevents cascading effect re-runs when userSettings changes.
  const userSettingsRef = useRef(userSettings);
  useEffect(() => {
    userSettingsRef.current = userSettings;
  });

  // If settings were already loaded when the component mounted (zustand store
  // persists across client-side navigation), skip redirect effects entirely.
  // Only redirect when settings transition from not-loaded → loaded on a fresh
  // page load. The WS handler intentionally preserves workspaceId/workflowId,
  // so redirect effects never fire from external (cross-tab) changes.
  const settingsLoadedOnMountRef = useRef(userSettings.loaded);

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const current = userSettingsRef.current;
      const repositoryIds = Array.from(new Set(next.repositoryIds)).sort();
      const enablePreviewOnClick = next.enablePreviewOnClick ?? current.enablePreviewOnClick;
      const kanbanViewMode = next.kanbanViewMode !== undefined ? next.kanbanViewMode : (current.kanbanViewMode ?? null);
      const normalized: DisplaySettings = {
        workspaceId: next.workspaceId,
        workflowId: next.workflowId,
        kanbanViewMode,
        repositoryIds,
        preferredShell: next.preferredShell ?? current.preferredShell ?? null,
        shellOptions: current.shellOptions ?? [],
        defaultEditorId: current.defaultEditorId ?? null,
        enablePreviewOnClick,
        chatSubmitKey: current.chatSubmitKey ?? 'cmd_enter',
        reviewAutoMarkOnScroll: current.reviewAutoMarkOnScroll ?? true,
        lspAutoStartLanguages: current.lspAutoStartLanguages ?? [],
        lspAutoInstallLanguages: current.lspAutoInstallLanguages ?? [],
        lspServerConfigs: current.lspServerConfigs ?? {},
        loaded: true,
      };
      const sameWorkspace = normalized.workspaceId === current.workspaceId;
      const sameWorkflow = normalized.workflowId === current.workflowId;
      const samePreview = normalized.enablePreviewOnClick === current.enablePreviewOnClick;
      const sameViewMode = normalized.kanbanViewMode === current.kanbanViewMode;
      const sameRepos =
        normalized.repositoryIds.length === current.repositoryIds.length &&
        normalized.repositoryIds.every((id, index) => id === current.repositoryIds[index]);
      if (sameWorkspace && sameWorkflow && sameRepos && samePreview && sameViewMode && current.loaded) {
        return;
      }
      setUserSettings(normalized);
      const payload = {
        workspace_id: normalized.workspaceId ?? '',
        workflow_filter_id: normalized.workflowId ?? '',
        repository_ids: normalized.repositoryIds,
        enable_preview_on_click: normalized.enablePreviewOnClick,
        kanban_view_mode: normalized.kanbanViewMode ?? '',
      };
      const client = getWebSocketClient();
      if (!client) {
        updateUserSettings(payload, { cache: 'no-store' }).catch(() => {
          // Ignore update errors for now; local state stays responsive.
        });
        return;
      }
      client.request('user.settings.update', payload).catch(() => {
        // Fall back to HTTP if WS update fails (e.g. navigation races).
        updateUserSettings(payload, { cache: 'no-store' }).catch(() => {
          // Ignore update errors for now; local state stays responsive.
        });
      });
    },
    [setUserSettings]
  );

  useEffect(() => {
    if (userSettings.loaded) return;
    fetchUserSettings({ cache: 'no-store' })
      .then((data) => {
        if (!data?.settings) return;
        const repositoryIds = Array.from(new Set<string>(data.settings.repository_ids ?? [])).sort();
        setUserSettings({
          workspaceId: data.settings.workspace_id || null,
          workflowId: data.settings.workflow_filter_id || null,
          kanbanViewMode: data.settings.kanban_view_mode || null,
          repositoryIds,
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
  }, [setUserSettings, userSettings.loaded]);

  // Workspace redirect effect — only fires on fresh page loads when settings
  // transition from not-loaded to loaded. Skipped entirely on re-mounts
  // (e.g. navigating to a task and back) since the store already has settings.
  useEffect(() => {
    if (!userSettings.loaded) return;
    if (settingsLoadedOnMountRef.current) return;
    settingsLoadedOnMountRef.current = true;
    if (userSettings.workspaceId && userSettings.workspaceId !== workspaceId) {
      onWorkspaceChange?.(userSettings.workspaceId);
    }
  }, [onWorkspaceChange, userSettings.loaded, userSettings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (!userSettings.workspaceId && workspaceId) {
      queueMicrotask(() => {
        commitSettings({
          workspaceId,
          workflowId: userSettings.workflowId,
          repositoryIds: userSettings.repositoryIds,
        });
      });
    }
  }, [commitSettings, userSettings.workflowId, userSettings.loaded, userSettings.repositoryIds, userSettings.workspaceId, workspaceId]);

  // Workflow redirect effect — only fires on fresh page loads when settings
  // transition from not-loaded to loaded. Skipped on re-mounts since the
  // store already has settings and URL/active state is the source of truth.
  useEffect(() => {
    if (!userSettings.loaded) return;
    if (settingsLoadedOnMountRef.current) return;
    // settingsLoadedOnMountRef is set by the workspace effect above
    if (userSettings.workflowId && userSettings.workflowId !== workflowId) {
      onWorkflowChange?.(userSettings.workflowId);
    }
  }, [workflowId, onWorkflowChange, userSettings.workflowId, userSettings.loaded]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (repositories.length === 0) return;
    const repoIds = repositories.map((repo: Repository) => repo.id);
    const validIds = userSettings.repositoryIds.filter((id: string) => repoIds.includes(id));
    const nextIds = validIds;
    const isSame =
      nextIds.length === userSettings.repositoryIds.length &&
      nextIds.every((id: string, index: number) => id === userSettings.repositoryIds[index]);
    if (!isSame) {
      queueMicrotask(() => {
        commitSettings({
          workspaceId: userSettings.workspaceId,
          workflowId: userSettings.workflowId,
          repositoryIds: nextIds,
        });
      });
    }
  }, [commitSettings, repositories, userSettings.workflowId, userSettings.loaded, userSettings.repositoryIds, userSettings.workspaceId]);

  const allRepositoriesSelected = userSettings.repositoryIds.length === 0;
  const selectedRepositoryIds = useMemo(
    () => mapSelectedRepositoryIds(repositories, userSettings.repositoryIds),
    [repositories, userSettings.repositoryIds]
  );

  return {
    settings: userSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryIds,
  };
}
