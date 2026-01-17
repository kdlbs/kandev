import { useMemo } from 'react';
import { useBoardSnapshot } from '@/hooks/use-board-snapshot';
import { useAppStore } from '@/components/state-provider';

export function useTasks(boardId: string | null) {
  useBoardSnapshot(boardId);

  const kanbanBoardId = useAppStore((state) => state.kanban.boardId);
  const tasks = useAppStore((state) => state.kanban.tasks);

  const boardTasks = useMemo(() => {
    if (!boardId || kanbanBoardId !== boardId) {
      return [];
    }
    return tasks;
  }, [boardId, kanbanBoardId, tasks]);

  return { tasks: boardTasks };
}
