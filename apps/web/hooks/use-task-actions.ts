import { useCallback } from "react";
import { archiveTask, deleteTask, moveTask, updateTask } from "@/lib/api";
import { useAppStoreApi } from "@/components/state-provider";
import { useTaskRemoval } from "@/hooks/use-task-removal";

type MovePayload = { workflow_id: string; workflow_step_id: string; position: number };

export function useTaskActions() {
  const moveTaskById = useCallback(async (taskId: string, payload: MovePayload) => {
    return moveTask(taskId, payload);
  }, []);

  const deleteTaskById = useCallback(async (taskId: string) => {
    return deleteTask(taskId);
  }, []);

  const archiveTaskById = useCallback(async (taskId: string) => {
    return archiveTask(taskId);
  }, []);

  const renameTaskById = useCallback(async (taskId: string, title: string) => {
    return updateTask(taskId, { title });
  }, []);

  return { moveTaskById, deleteTaskById, archiveTaskById, renameTaskById };
}

/**
 * Archives a task and switches to the next available task.
 * Shared between the PR merged banner and the sidebar archive action.
 */
export function useArchiveAndSwitchTask(opts?: { useLayoutSwitch?: boolean }) {
  const store = useAppStoreApi();
  const { archiveTaskById } = useTaskActions();
  const { removeTaskFromBoard } = useTaskRemoval({
    store,
    useLayoutSwitch: opts?.useLayoutSwitch,
  });

  return useCallback(
    async (taskId: string) => {
      const { activeTaskId: wasActiveTaskId, activeSessionId: wasActiveSessionId } =
        store.getState().tasks;
      await archiveTaskById(taskId);
      await removeTaskFromBoard(taskId, { wasActiveTaskId, wasActiveSessionId });
    },
    [archiveTaskById, removeTaskFromBoard, store],
  );
}
