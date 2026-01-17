import { useCallback, useEffect, useMemo } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { fetchUserSettings, updateUserSettings } from '@/lib/http';
import { mapSelectedRepositoryIds } from '@/lib/kanban/filters';
import { useAppStore } from '@/components/state-provider';
import { useRepositories } from '@/hooks/use-repositories';

type DisplaySettings = {
  workspaceId: string | null;
  boardId: string | null;
  repositoryIds: string[];
  preferredShell: string | null;
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

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const repositoryIds = Array.from(new Set(next.repositoryIds)).sort();
      const normalized: DisplaySettings = {
        workspaceId: next.workspaceId,
        boardId: next.boardId,
        repositoryIds,
        preferredShell: next.preferredShell ?? userSettings.preferredShell ?? null,
        loaded: true,
      };
      const sameWorkspace = normalized.workspaceId === userSettings.workspaceId;
      const sameBoard = normalized.boardId === userSettings.boardId;
      const sameRepos =
        normalized.repositoryIds.length === userSettings.repositoryIds.length &&
        normalized.repositoryIds.every((id, index) => id === userSettings.repositoryIds[index]);
      if (sameWorkspace && sameBoard && sameRepos && userSettings.loaded) {
        return;
      }
      setUserSettings(normalized);
      const payload = {
        workspace_id: normalized.workspaceId ?? '',
        board_id: normalized.boardId ?? '',
        repository_ids: normalized.repositoryIds,
      };
      const client = getWebSocketClient();
      if (!client) {
        updateUserSettings(payload, { cache: 'no-store' }).catch(() => {
          // Ignore update errors for now; local state stays responsive.
        });
        return;
      }
      client.request('user.settings.update', payload).catch(() => {
        // Ignore update errors for now; local state stays responsive.
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
      onWorkspaceChange?.(userSettings.workspaceId);
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
      onBoardChange?.(userSettings.boardId);
    }
  }, [boardId, onBoardChange, userSettings.boardId, userSettings.loaded]);

  useEffect(() => {
    if (!userSettings.loaded) return;
    if (repositories.length === 0) return;
    const repoIds = repositories.map((repo) => repo.id);
    const validIds = userSettings.repositoryIds.filter((id) => repoIds.includes(id));
    const nextIds = validIds;
    const isSame =
      nextIds.length === userSettings.repositoryIds.length &&
      nextIds.every((id, index) => id === userSettings.repositoryIds[index]);
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
