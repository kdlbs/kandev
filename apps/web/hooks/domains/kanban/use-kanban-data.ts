'use client';

import { useMemo, useState, useSyncExternalStore } from 'react';
import { useAppStore } from '@/components/state-provider';
import { useBoards } from '@/hooks/use-boards';
import { useBoardSnapshot } from '@/hooks/use-board-snapshot';
import { useUserDisplaySettings } from '@/hooks/use-user-display-settings';
import { filterTasksByRepositories } from '@/lib/kanban/filters';
import type { Column } from '@/components/kanban-column';

type KanbanDataOptions = {
  onWorkspaceChange: (workspaceId: string | null) => void;
  onBoardChange: (boardId: string | null) => void;
};

export function useKanbanData({ onWorkspaceChange, onBoardChange }: KanbanDataOptions) {
  const [taskSessionAvailability, setTaskSessionAvailability] = useState<Record<string, boolean>>({});

  // Store selectors
  const kanban = useAppStore((state) => state.kanban);
  const workspaceState = useAppStore((state) => state.workspaces);
  const boardsState = useAppStore((state) => state.boards);
  const enablePreviewOnClick = useAppStore((state) => state.userSettings.enablePreviewOnClick);

  // Data fetching hooks
  useBoards(workspaceState.activeId, true);
  useBoardSnapshot(boardsState.activeId);

  // User settings hook
  const {
    settings: userSettings,
    commitSettings,
    selectedRepositoryIds,
  } = useUserDisplaySettings({
    workspaceId: workspaceState.activeId,
    boardId: boardsState.activeId,
    onWorkspaceChange,
    onBoardChange,
  });

  // SSR safety check
  const isMounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false
  );

  // Derived data
  const columns = useMemo<Column[]>(
    () =>
      [...kanban.columns]
        .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
        .map((column) => ({
          id: column.id,
          title: column.title,
          color: column.color || 'bg-neutral-400',
        })),
    [kanban.columns]
  );

  const tasks = kanban.tasks.map((task: typeof kanban.tasks[number]) => ({
    id: task.id,
    title: task.title,
    columnId: task.columnId,
    state: task.state,
    description: task.description,
    position: task.position,
    repositoryId: task.repositoryId,
  }));

  const activeColumns = kanban.boardId ? columns : [];

  const visibleTasks = useMemo(
    () => filterTasksByRepositories(tasks, selectedRepositoryIds),
    [tasks, selectedRepositoryIds]
  );

  const visibleTasksWithSessions = useMemo(
    () =>
      visibleTasks.map((task) => ({
        ...task,
        hasSession: taskSessionAvailability[task.id],
      })),
    [visibleTasks, taskSessionAvailability]
  );

  return {
    // State
    kanban,
    workspaceState,
    boardsState,
    enablePreviewOnClick,
    userSettings,
    commitSettings,
    selectedRepositoryIds,
    taskSessionAvailability,
    setTaskSessionAvailability,
    isMounted,

    // Derived data
    activeColumns,
    visibleTasksWithSessions,
  };
}
