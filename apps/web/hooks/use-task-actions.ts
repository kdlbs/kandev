import { useCallback } from 'react';
import { deleteTask, moveTask } from '@/lib/api';

type MovePayload = { workflow_id: string; workflow_step_id: string; position: number };

export function useTaskActions() {
  const moveTaskById = useCallback(async (taskId: string, payload: MovePayload) => {
    return moveTask(taskId, payload);
  }, []);

  const deleteTaskById = useCallback(async (taskId: string) => {
    return deleteTask(taskId);
  }, []);

  return { moveTaskById, deleteTaskById };
}
