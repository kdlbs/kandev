import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { isTaskDeleteDirtyWorktreeError } from "@/lib/api/task-delete-errors";
import {
  useArchiveAndSwitchTask,
  useTaskActions,
  type TaskActionOptions,
} from "./use-task-actions";
import { collectTaskTreeIdsFromStore, useTaskRemoval } from "./use-task-removal";

/** Shared per-task single-flight removal. The caller owns confirmation and captures the task ID. */
export function useTaskMenuActions(options?: {
  stayOnListing?: boolean;
  useLayoutSwitch?: boolean;
}) {
  const store = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation();
  const archive = useArchiveAndSwitchTask(options);
  const { deleteTaskById } = useTaskActions();
  const { removeTaskFromBoard } = useTaskRemoval({ store, ...options });
  const pending = useRef(new Set<string>());
  const [pendingTaskId, setPendingTaskId] = useState<string | null>(null);
  const run = useCallback(
    async (kind: "archive" | "delete", taskId: string, opts?: TaskActionOptions) => {
      if (pending.current.has(taskId)) return false;
      pending.current.add(taskId);
      setPendingTaskId(pending.current.values().next().value ?? null);
      try {
        if (kind === "archive") {
          await archive(taskId, opts);
        } else {
          const { activeTaskId: wasActiveTaskId, activeSessionId: wasActiveSessionId } =
            store.getState().tasks;
          const excludedTaskIds = opts?.cascade
            ? collectTaskTreeIdsFromStore(store, taskId)
            : undefined;
          await deleteTaskById(taskId, opts);
          await removeTaskFromBoard(taskId, {
            wasActiveTaskId,
            wasActiveSessionId,
            excludeTaskTree: opts?.cascade,
            excludedTaskIds,
          });
        }
        return true;
      } catch (error) {
        if (kind !== "delete" || !isTaskDeleteDirtyWorktreeError(error)) {
          toast({
            title: t(kind === "archive" ? "tasks:failedToArchiveTask" : "tasks:failedToDeleteTask"),
            variant: "error",
          });
        }
        return false;
      } finally {
        pending.current.delete(taskId);
        setPendingTaskId(pending.current.values().next().value ?? null);
      }
    },
    [archive, deleteTaskById, removeTaskFromBoard, store, t, toast],
  );
  return {
    pendingTaskId,
    runArchive: useCallback(
      (taskId: string, opts?: TaskActionOptions) => run("archive", taskId, opts),
      [run],
    ),
    runDelete: useCallback(
      (taskId: string, opts?: TaskActionOptions) => run("delete", taskId, opts),
      [run],
    ),
  };
}
