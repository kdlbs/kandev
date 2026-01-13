import { useCallback, useEffect, useMemo, useState } from 'react';
import { getWebSocketClient } from '@/lib/ws/connection';
import { getBackendConfig } from '@/lib/config';
import { listRepositories } from '@/lib/http/client';
import type { Repository } from '@/lib/types/http';
import { mapSelectedRepositoryPaths } from '@/lib/kanban/filters';

const DEFAULT_SETTINGS = {
  workspaceId: null as string | null,
  boardId: null as string | null,
  repositoryIds: [] as string[],
  loaded: false,
};

type DisplaySettings = typeof DEFAULT_SETTINGS;

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
};

export function useUserDisplaySettings({
  workspaceId,
  boardId,
  onWorkspaceChange,
  onBoardChange,
}: UseUserDisplaySettingsInput) {
  const [settings, setSettings] = useState<DisplaySettings>(DEFAULT_SETTINGS);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repositoriesLoading, setRepositoriesLoading] = useState(false);

  const commitSettings = useCallback(
    (next: CommitPayload) => {
      const repositoryIds = Array.from(new Set(next.repositoryIds)).sort();
      const normalized = { ...next, repositoryIds, loaded: true };
      const sameWorkspace = normalized.workspaceId === settings.workspaceId;
      const sameBoard = normalized.boardId === settings.boardId;
      const sameRepos =
        normalized.repositoryIds.length === settings.repositoryIds.length &&
        normalized.repositoryIds.every((id, index) => id === settings.repositoryIds[index]);
      if (sameWorkspace && sameBoard && sameRepos && settings.loaded) {
        return;
      }
      setSettings({ ...normalized, loaded: true });
      const payload = {
        workspace_id: normalized.workspaceId ?? '',
        board_id: normalized.boardId ?? '',
        repository_ids: normalized.repositoryIds,
      };
      const client = getWebSocketClient();
      if (!client) {
        fetch(`${getBackendConfig().apiBaseUrl}/api/v1/user/settings`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        }).catch(() => {
          // Ignore update errors for now; local state stays responsive.
        });
        return;
      }
      client.request('user.settings.update', payload).catch(() => {
        // Ignore update errors for now; local state stays responsive.
      });
    },
    [settings.boardId, settings.loaded, settings.repositoryIds, settings.workspaceId]
  );

  useEffect(() => {
    if (settings.loaded) return;
    fetch(`${getBackendConfig().apiBaseUrl}/api/v1/user/settings`, { cache: 'no-store' })
      .then((response) => (response.ok ? response.json() : null))
      .then((data) => {
        if (!data?.settings) return;
        const repositoryIds = Array.from(new Set(data.settings.repository_ids ?? [])).sort();
        setSettings({
          workspaceId: data.settings.workspace_id || null,
          boardId: data.settings.board_id || null,
          repositoryIds,
          loaded: true,
        });
      })
      .catch(() => {
        // Ignore settings fetch errors for now.
      });
  }, [settings.loaded]);

  useEffect(() => {
    if (!settings.loaded) return;
    if (settings.workspaceId && settings.workspaceId !== workspaceId) {
      onWorkspaceChange?.(settings.workspaceId);
    }
  }, [onWorkspaceChange, settings.loaded, settings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!settings.loaded) return;
    if (!settings.workspaceId && workspaceId) {
      queueMicrotask(() => {
        commitSettings({
          workspaceId,
          boardId: settings.boardId,
          repositoryIds: settings.repositoryIds,
        });
      });
    }
  }, [commitSettings, settings.boardId, settings.loaded, settings.repositoryIds, settings.workspaceId, workspaceId]);

  useEffect(() => {
    if (!settings.loaded) return;
    if (settings.boardId && settings.boardId !== boardId) {
      onBoardChange?.(settings.boardId);
    }
  }, [boardId, onBoardChange, settings.boardId, settings.loaded]);

  useEffect(() => {
    if (!workspaceId) {
      queueMicrotask(() => {
        setRepositories([]);
      });
      return;
    }
    queueMicrotask(() => {
      setRepositoriesLoading(true);
    });
    listRepositories(getBackendConfig().apiBaseUrl, workspaceId)
      .then((response) => {
        setRepositories(response.repositories);
      })
      .catch(() => {
        setRepositories([]);
      })
      .finally(() => {
        setRepositoriesLoading(false);
      });
  }, [workspaceId]);

  useEffect(() => {
    if (!settings.loaded) return;
    if (repositories.length === 0) return;
    const repoIds = repositories.map((repo) => repo.id);
    const validIds = settings.repositoryIds.filter((id) => repoIds.includes(id));
    const nextIds = validIds;
    const isSame =
      nextIds.length === settings.repositoryIds.length &&
      nextIds.every((id, index) => id === settings.repositoryIds[index]);
    if (!isSame) {
      queueMicrotask(() => {
        commitSettings({
          workspaceId: settings.workspaceId,
          boardId: settings.boardId,
          repositoryIds: nextIds,
        });
      });
    }
  }, [commitSettings, repositories, settings.boardId, settings.loaded, settings.repositoryIds, settings.workspaceId]);

  const allRepositoriesSelected = settings.repositoryIds.length === 0;
  const selectedRepositoryPaths = useMemo(
    () => mapSelectedRepositoryPaths(repositories, settings.repositoryIds),
    [repositories, settings.repositoryIds]
  );

  return {
    settings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryPaths,
  };
}
