import { useCallback } from 'react';
import { deleteTask, moveTask } from '@/lib/http';

type MovePayload = { board_id: string; column_id: string; position: number };

export function useTaskActions() {
  const moveTaskById = useCallback(async (taskId: string, payload: MovePayload) => {
    return moveTask(taskId, payload);
  }, []);

  const deleteTaskById = useCallback(async (taskId: string) => {
    return deleteTask(taskId);
  }, []);

  return { moveTaskById, deleteTaskById };
}
