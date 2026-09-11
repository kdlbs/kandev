import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { isTaskDeleteDirtyWorktreeError } from "@/lib/api/task-delete-errors";
import {
  useArchiveAndSwitchTask,
  useDeleteAndSwitchTask,
  type TaskActionOptions,
} from "./use-task-actions";

/** Shared per-task single-flight removal. The caller owns confirmation and captures the task ID. */
export function useTaskMenuActions(options?: {
  stayOnListing?: boolean;
  useLayoutSwitch?: boolean;
}) {
  const { toast } = useToast();
  const { t } = useTranslation();
  const archive = useArchiveAndSwitchTask(options);
  const deleteAndSwitch = useDeleteAndSwitchTask(options);
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
          await deleteAndSwitch(taskId, opts);
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
    [archive, deleteAndSwitch, t, toast],
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
