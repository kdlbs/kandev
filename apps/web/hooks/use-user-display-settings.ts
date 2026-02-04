import { useCallback, useEffect, useMemo, useRef } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { fetchUserSettings, updateUserSettings } from '@/lib/api';
import { mapSelectedRepositoryIds } from '@/lib/kanban/filters';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import type { Repository } from '@/lib/types/http';

type DisplaySettings = {
  workspaceId: string | null;
  boardId: string | null;
  repositoryIds: string[];
  preferredShell: string | null;
  shellOptions: Array<{ value: string; label: string }>;
  defaultEditorId: string | null;
  enablePreviewOnClick: boolean;
  chatSubmitKey: 'enter' | 'cmd_enter';
  loaded: boolean;
};

type UseUserDisplaySettingsInput = {
  workspaceId: string | null;
  boardId: string | null;
  onWorkspaceChange?: (workspaceId: string | null) => void;
  onBoardChange?: (boardId: string | null) => void;
};

type CommitPayload = {
  workspaceId: string | null;
  boardId: string | null;
  repositoryIds: string[];
  preferredShell?: string | null;
  enablePreviewOnClick?: boolean;
};

export function useUserDisplaySettings({
  workspaceId,
  boardId,
  onWorkspaceChange,
  onBoardChange,
}: UseUserDisplaySettingsInput) {
  const userSettings = useAppStore((state) => state.userSettings);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const { repositories, isLoading: repositoriesLoading } = useRepositories(workspaceId, true);
  const lastAppliedWorkspaceIdRef = useRef<string | null>(null);
  const lastAppliedBoardIdRef = useRef<string | null>(null);

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const repositoryIds = Array.from(new Set(next.repositoryIds)).sort();
      const enablePreviewOnClick = next.enablePreviewOnClick ?? userSettings.enablePreviewOnClick;
      const normalized: DisplaySettings = {
        workspaceId: next.workspaceId,
        boardId: next.boardId,
        repositoryIds,
        preferredShell: next.preferredShell ?? userSettings.preferredShell ?? null,
        shellOptions: userSettings.shellOptions ?? [],
        defaultEditorId: userSettings.defaultEditorId ?? null,
        enablePreviewOnClick,
        chatSubmitKey: userSettings.chatSubmitKey ?? 'cmd_enter',
        loaded: true,
      };
      const sameWorkspace = normalized.workspaceId === userSettings.workspaceId;
      const sameBoard = normalized.boardId === userSettings.boardId;
      const samePreview = normalized.enablePreviewOnClick === userSettings.enablePreviewOnClick;
      const sameRepos =
        normalized.repositoryIds.length === userSettings.repositoryIds.length &&
        normalized.repositoryIds.every((id, index) => id === userSettings.repositoryIds[index]);
      if (sameWorkspace && sameBoard && sameRepos && samePreview && userSettings.loaded) {
        return;
      }
      setUserSettings(normalized);
      const payload = {
        workspace_id: normalized.workspaceId ?? '',
        board_id: normalized.boardId ?? '',
        repository_ids: normalized.repositoryIds,
        enable_preview_on_click: normalized.enablePreviewOnClick,
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
    [setUserSettings, userSettings]
  );

  useEffect(() => {
    if (userSettings.loaded) return;
    fetchUserSettings({ cache: 'no-store' })
      .then((data) => {
        if (!data?.settings) return;
        const repositoryIds = Array.from(new Set<string>(data.settings.repository_ids ?? [])).sort();
        setUserSettings({
          workspaceId: data.settings.workspace_id || null,
          boardId: data.settings.board_id || null,
          repositoryIds,
          preferredShell: data.settings.preferred_shell || null,
          shellOptions: data.shell_options ?? [],
          defaultEditorId: data.settings.default_editor_id || null,
          enablePreviewOnClick: data.settings.enable_preview_on_click ?? false,
          chatSubmitKey: data.settings.chat_submit_key ?? 'cmd_enter',
          loaded: true,
        });
      })
      .catch(() => {
        // Ignore settings fetch errors for now.
      });
  }, [setUserSettings, userSettings.loaded]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (userSettings.workspaceId && userSettings.workspaceId !== workspaceId) {
      if (lastAppliedWorkspaceIdRef.current === userSettings.workspaceId) {
        return;
      }
      lastAppliedWorkspaceIdRef.current = userSettings.workspaceId;
      onWorkspaceChange?.(userSettings.workspaceId);
      return;
    }
    if (userSettings.workspaceId === workspaceId) {
      lastAppliedWorkspaceIdRef.current = null;
    }
  }, [onWorkspaceChange, userSettings.loaded, userSettings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (!userSettings.workspaceId && workspaceId) {
      queueMicrotask(() => {
        commitSettings({
          workspaceId,
          boardId: userSettings.boardId,
          repositoryIds: userSettings.repositoryIds,
        });
      });
    }
  }, [commitSettings, userSettings.boardId, userSettings.loaded, userSettings.repositoryIds, userSettings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (userSettings.boardId && userSettings.boardId !== boardId) {
      if (lastAppliedBoardIdRef.current === userSettings.boardId) {
        return;
      }
      lastAppliedBoardIdRef.current = userSettings.boardId;
      onBoardChange?.(userSettings.boardId);
      return;
    }
    if (userSettings.boardId === boardId) {
      lastAppliedBoardIdRef.current = null;
    }
  }, [boardId, onBoardChange, userSettings.boardId, userSettings.loaded]);

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
          boardId: userSettings.boardId,
          repositoryIds: nextIds,
        });
      });
    }
  }, [commitSettings, repositories, userSettings.boardId, userSettings.loaded, userSettings.repositoryIds, userSettings.workspaceId]);

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
