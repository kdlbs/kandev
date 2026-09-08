import { useCallback } from "react";
import { archiveTask, deleteTask, moveTask, updateTask } from "@/lib/api";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";
import { isTaskDeleteDirtyWorktreeError } from "@/lib/api/task-delete-errors";
import { replaceTaskUrl } from "@/lib/links";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useTaskRemoval } from "@/hooks/use-task-removal";
import { useTranslation } from "react-i18next";

type MovePayload = { workflow_id: string; workflow_step_id: string; position: number };

export type TaskActionOptions = {
  cascade?: boolean;
  discardWorktreeChanges?: boolean;
};

export function useTaskActions() {
  const { toast } = useToast();
  const { t } = useTranslation();

  const moveTaskById = useCallback(async (taskId: string, payload: MovePayload) => {
    return moveTask(taskId, payload);
  }, []);

  const deleteTaskById = useCallback(
    async (taskId: string, opts?: DeleteTaskParams) => {
      try {
        return await deleteTask(taskId, opts);
      } catch (error) {
        if (isTaskDeleteDirtyWorktreeError(error)) {
          toast({
            title: t("task:deleteDirtyWorktreeTitle"),
            description: t("task:deleteDirtyWorktreeDescription"),
            variant: "error",
          });
        }
        throw error;
      }
    },
    [t, toast],
  );

  const archiveTaskById = useCallback(async (taskId: string, opts?: TaskActionOptions) => {
    return archiveTask(taskId, opts);
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
    async (taskId: string, opts?: TaskActionOptions) => {
      const { activeTaskId: wasActiveTaskId, activeSessionId: wasActiveSessionId } =
        store.getState().tasks;
      const removalOptions = opts?.cascade ? { excludeTaskTree: true } : {};

      const initialSwitch = await removeTaskFromBoard(taskId, {
        wasActiveTaskId,
        wasActiveSessionId,
        switchOnly: true,
        ...removalOptions,
      });

      try {
        await archiveTaskById(taskId, opts);
        await removeTaskFromBoard(taskId, {
          wasActiveTaskId,
          wasActiveSessionId,
          ...removalOptions,
          ...(initialSwitch.excludedTaskIds
            ? { excludedTaskIds: initialSwitch.excludedTaskIds }
            : {}),
        });
      } catch (error) {
        if (
          wasActiveTaskId &&
          initialSwitch.switchedTaskId !== null &&
          store.getState().tasks.activeTaskId === initialSwitch.switchedTaskId
        ) {
          if (wasActiveSessionId) {
            store.getState().setActiveSession(wasActiveTaskId, wasActiveSessionId);
          } else {
            store.getState().setActiveTask(wasActiveTaskId);
          }
          replaceTaskUrl(wasActiveTaskId);
        }
        throw error;
      }
    },
    [archiveTaskById, removeTaskFromBoard, store],
  );
}
