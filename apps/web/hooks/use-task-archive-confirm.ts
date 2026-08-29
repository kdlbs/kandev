import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useToast } from "@/components/toast-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

export type ArchiveConfirmTarget = {
  id: string;
  title: string;
  executorType?: string | null;
};

/**
 * Archive flow shared by every task-scoped archive entry point (the terminal-state
 * PR banners, the command palette). Archiving always goes through the same
 * preference-aware confirmation dialog: `requestArchive` only resolves the target
 * task, and `TaskArchiveConfirmDialog` decides whether to prompt or bypass.
 * Only failures toast; on success the archive-and-switch flow moves the user to
 * the next task.
 */
export function useTaskArchiveConfirm(taskId: string | null) {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { toast } = useToast();
  const [target, setTarget] = useState<ArchiveConfirmTarget | null>(null);
  const [isPending, setIsPending] = useState(false);

  const requestArchive = useCallback(() => {
    if (!taskId) return;
    const state = store.getState();
    const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
    setTarget({
      id: taskId,
      title: task?.title ?? t("task:thisTask"),
      executorType: task?.primaryExecutorType,
    });
  }, [store, t, taskId]);

  const closeConfirm = useCallback(() => setTarget(null), []);

  const confirmArchive = useCallback(
    async ({ cascade }: { cascade: boolean }) => {
      const requestedTaskId = target?.id ?? taskId;
      if (!requestedTaskId) return;
      setIsPending(true);
      try {
        await archiveAndSwitch(requestedTaskId, { cascade });
      } catch (error) {
        // The toast is deliberately generic; keep the rejection reason in the
        // console so a failed archive is diagnosable from a user's devtools.
        console.error("Failed to archive task:", error);
        toast({ description: t("task:failedToArchiveTask"), variant: "error" });
      } finally {
        setIsPending(false);
      }
    },
    [archiveAndSwitch, t, target?.id, taskId, toast],
  );

  return { target, requestArchive, closeConfirm, confirmArchive, isPending };
}
