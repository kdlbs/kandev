import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import type { useAppStoreApi } from "@/components/state-provider";
import { usePendingTaskRemovalIds } from "@/hooks/use-task-removal";
import { useTaskMenuActions } from "@/hooks/use-task-menu-actions";
import type { TaskActionOptions } from "@/hooks/use-task-actions";
import type { AggregatedSidebarTasks } from "./task-session-sidebar-aggregate";
import { findSidebarTask } from "./task-session-sidebar-task-lookup";

type StoreApi = ReturnType<typeof useAppStoreApi>;

/**
 * Desktop sidebar archive adapter. The confirmation target is local; busy
 * state comes from the accepted removal membership of that captured target,
 * so one archive settling never clears another target's controls.
 */
export function useArchiveActions(
  store: StoreApi,
  pageTasks: AggregatedSidebarTasks["allTasks"],
  workspaceId: string | null,
) {
  const { t } = useTranslation();
  const { runArchive: archiveAndSwitch } = useTaskMenuActions({ useLayoutSwitch: true });
  const [archivingTask, setArchivingTask] = useState<{
    id: string;
    title: string;
    executorType?: string | null;
  } | null>(null);
  const pendingRemovalTaskIds = usePendingTaskRemovalIds(store, workspaceId);
  const isArchiving = archivingTask !== null && pendingRemovalTaskIds.has(archivingTask.id);

  const runArchive = useCallback(
    async (taskId: string, opts: TaskActionOptions) => {
      try {
        await archiveAndSwitch(taskId, opts);
      } catch (error) {
        console.error("Failed to archive task:", error);
      } finally {
        setArchivingTask((current) => (current?.id === taskId ? null : current));
      }
    },
    [archiveAndSwitch],
  );

  const handleArchiveTask = useCallback(
    (taskId: string, opts?: TaskActionOptions) => {
      if (opts) {
        void runArchive(taskId, opts);
        return;
      }
      const task = findSidebarTask(store.getState(), taskId, pageTasks);
      setArchivingTask({
        id: taskId,
        title: task?.title ?? t("task:thisTask"),
        executorType: task?.primaryExecutorType,
      });
    },
    [pageTasks, runArchive, store, t],
  );

  const handleArchiveConfirm = useCallback(
    async (opts: { cascade: boolean }) => {
      if (!archivingTask) return;
      await runArchive(archivingTask.id, opts);
    },
    [archivingTask, runArchive],
  );

  return {
    archivingTask,
    setArchivingTask,
    isArchiving,
    handleArchiveTask,
    handleArchiveConfirm,
  };
}
