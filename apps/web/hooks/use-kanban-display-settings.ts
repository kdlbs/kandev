'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useAppStore } from '@/components/state-provider';
import { useUserDisplaySettings } from '@/hooks/use-user-display-settings';

/**
 * Custom hook that consolidates all kanban display settings and eliminates prop drilling.
 * This hook provides access to workspaces, boards, repositories, and preview settings,
 * along with handlers for changing these settings.
 */
export function useKanbanDisplaySettings() {
  const router = useRouter();

  // Access store directly
  const workspaces = useAppStore((state) => state.workspaces.items);
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const boards = useAppStore((state) => state.boards.items);
  const activeBoardId = useAppStore((state) => state.boards.activeId);
  const setActiveWorkspace = useAppStore((state) => state.setActiveWorkspace);
  const setActiveBoard = useAppStore((state) => state.setActiveBoard);

  // Use existing compound hook for user settings
  const {
    settings: userSettings,
    commitSettings,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
  } = useUserDisplaySettings({
    workspaceId: activeWorkspaceId,
    boardId: activeBoardId,
  });

  // Get preview setting from store
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);

  // Define handlers
  const handleWorkspaceChange = useCallback(
    (nextWorkspaceId: string | null) => {
      setActiveWorkspace(nextWorkspaceId);
      if (nextWorkspaceId) {
        router.push(`/?workspaceId=${nextWorkspaceId}`);
      } else {
        router.push('/');
      }
      commitSettings({
        workspaceId: nextWorkspaceId,
        boardId: null,
        repositoryIds: [],
      });
    },
    [setActiveWorkspace, router, commitSettings]
  );

  const handleBoardChange = useCallback(
    (nextBoardId: string | null) => {
      setActiveBoard(nextBoardId);
      if (nextBoardId) {
        const workspaceId = boards.find((board) => board.id === nextBoardId)?.workspaceId;
        const workspaceParam = workspaceId ? `&workspaceId=${workspaceId}` : '';
        router.push(`/?boardId=${nextBoardId}${workspaceParam}`);
      }
      commitSettings({
        workspaceId: userSettings.workspaceId,
        boardId: nextBoardId,
        repositoryIds: userSettings.repositoryIds,
      });
    },
    [setActiveBoard, boards, router, commitSettings, userSettings.workspaceId, userSettings.repositoryIds]
  );

  const handleRepositoryChange = useCallback(
    (value: string | 'all') => {
      if (value === 'all') {
        commitSettings({
          workspaceId: userSettings.workspaceId,
          boardId: userSettings.boardId,
          repositoryIds: [],
        });
        return;
      }
      commitSettings({
        workspaceId: userSettings.workspaceId,
        boardId: userSettings.boardId,
        repositoryIds: [value],
      });
    },
    [commitSettings, userSettings.workspaceId, userSettings.boardId]
  );

  const handleTogglePreviewOnClick = useCallback(
    (enabled: boolean) => {
      commitSettings({
        workspaceId: userSettings.workspaceId,
        boardId: userSettings.boardId,
        repositoryIds: userSettings.repositoryIds,
        enablePreviewOnClick: enabled,
      });
    },
    [commitSettings, userSettings.boardId, userSettings.repositoryIds, userSettings.workspaceId]
  );

  return {
    // Data
    workspaces,
    boards,
    activeWorkspaceId,
    activeBoardId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId: userSettings.repositoryIds[0] ?? null,
    enablePreviewOnClick,

    // Handlers
    onWorkspaceChange: handleWorkspaceChange,
    onBoardChange: handleBoardChange,
    onRepositoryChange: handleRepositoryChange,
    onTogglePreviewOnClick: handleTogglePreviewOnClick,
  };
}
