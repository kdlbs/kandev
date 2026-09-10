import { useCallback } from "react";
import { archiveTask, deleteTask, moveTask, updateTask } from "@/lib/api";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";
import { isTaskDeleteDirtyWorktreeError } from "@/lib/api/task-delete-errors";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useTaskRemoval, useTaskRemovalSuccessNotifier } from "@/hooks/use-task-removal";
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
  const notifySuccess = useTaskRemovalSuccessNotifier();
  const { runTaskRemoval } = useTaskRemoval({
    store,
    useLayoutSwitch: opts?.useLayoutSwitch,
    notifySuccess,
  });

  return useCallback(
    (taskId: string, opts?: TaskActionOptions) =>
      runTaskRemoval(
        "archive",
        {
          taskId,
          mutate: () => archiveTaskById(taskId, opts),
        },
        { cascade: opts?.cascade },
      ).then(() => undefined),
    [archiveTaskById, runTaskRemoval],
  );
}
