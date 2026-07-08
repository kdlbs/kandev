"use client";

import { useCallback, useState, type ReactNode } from "react";
import { IconGitMerge, IconGitPullRequestClosed, IconX } from "@tabler/icons-react";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useArchiveAndSwitchTask } from "@/hooks/use-task-actions";
import { useToast } from "@/components/toast-provider";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import {
  markPRClosedBannerDismissed,
  markPRMergedBannerDismissed,
  wasPRClosedBannerDismissed,
  wasPRMergedBannerDismissed,
} from "@/lib/local-storage";

type ArchiveTarget = { title: string; executorType?: string | null };

// Archiving from the terminal-state banners goes through the same confirmation
// dialog as every other archive surface. Only failures toast; on success the
// archive-and-switch flow moves the user to the next task.
function useBannerArchiveConfirm(taskId: string) {
  const store = useAppStoreApi();
  const archiveAndSwitch = useArchiveAndSwitchTask();
  const { toast } = useToast();
  const [target, setTarget] = useState<ArchiveTarget | null>(null);

  const requestArchive = useCallback(() => {
    const state = store.getState();
    const task = findTaskInSnapshots(taskId, state.kanbanMulti.snapshots, state.kanban.tasks);
    setTarget({ title: task?.title ?? "this task", executorType: task?.primaryExecutorType });
  }, [store, taskId]);

  const closeConfirm = useCallback(() => setTarget(null), []);

  const confirmArchive = useCallback(
    async ({ cascade }: { cascade: boolean }) => {
      try {
        await archiveAndSwitch(taskId, { cascade });
      } catch {
        toast({ description: "Failed to archive task", variant: "error" });
      }
    },
    [archiveAndSwitch, taskId, toast],
  );

  return { target, requestArchive, closeConfirm, confirmArchive };
}

// Presentational banner shared by PRMergedBanner / PRClosedBanner — an icon, a
// message, and Archive + Dismiss controls. Colors/icon/testIds are supplied by
// the caller so the two variants stay visually distinct. The Archive control
// opens the shared archive confirmation dialog rather than archiving directly.
function ArchiveDismissBanner({
  testIdPrefix,
  icon,
  text,
  containerClass,
  archiveClass,
  dismissClass,
  taskId,
  onDismiss,
}: {
  testIdPrefix: string;
  icon: ReactNode;
  text: string;
  containerClass: string;
  archiveClass: string;
  dismissClass: string;
  taskId: string;
  onDismiss: () => void;
}) {
  const { target, requestArchive, closeConfirm, confirmArchive } = useBannerArchiveConfirm(taskId);
  return (
    <>
      <div data-testid={`${testIdPrefix}-banner`} className={containerClass}>
        {icon}
        <span className="flex-1">{text}</span>
        <button
          type="button"
          data-testid={`${testIdPrefix}-archive-button`}
          onClick={requestArchive}
          className={archiveClass}
        >
          Archive
        </button>
        <button
          type="button"
          aria-label="Dismiss"
          data-testid={`${testIdPrefix}-dismiss-button`}
          onClick={onDismiss}
          className={dismissClass}
        >
          <IconX className="h-3 w-3" />
        </button>
      </div>
      <TaskArchiveConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open) closeConfirm();
        }}
        taskTitle={target?.title ?? ""}
        taskId={taskId}
        executorType={target?.executorType}
        onConfirm={confirmArchive}
        confirmTestId={`${testIdPrefix}-archive-confirm`}
      />
    </>
  );
}

export function PRMergedBanner({ taskId }: { taskId: string }) {
  const taskPRs = useAppStore((state) => state.taskPRs.byTaskId[taskId]);
  const [dismissed, setDismissed] = useState(() => wasPRMergedBannerDismissed(taskId));

  const handleDismiss = useCallback(() => {
    markPRMergedBannerDismissed(taskId);
    setDismissed(true);
  }, [taskId]);

  // Multi-repo: only show "ready to archive" once every PR is merged. A
  // single merged repo with others still open means the task isn't done yet.
  const allMerged = !!taskPRs && taskPRs.length > 0 && taskPRs.every((pr) => pr.state === "merged");
  if (!allMerged || dismissed) return null;

  const bannerText =
    taskPRs.length === 1
      ? `PR #${taskPRs[0].pr_number} has been merged. You can archive this task.`
      : `All ${taskPRs.length} PRs have been merged. You can archive this task.`;

  return (
    <ArchiveDismissBanner
      testIdPrefix="pr-merged"
      icon={<IconGitMerge className="h-3.5 w-3.5 shrink-0" />}
      text={bannerText}
      containerClass="flex flex-1 items-center gap-2 rounded-md bg-purple-500/10 px-2 py-1 text-purple-600 dark:text-purple-400"
      archiveClass="underline underline-offset-2 hover:text-purple-700 dark:hover:text-purple-300 cursor-pointer"
      dismissClass="p-0.5 hover:bg-purple-500/10 rounded cursor-pointer"
      taskId={taskId}
      onDismiss={handleDismiss}
    />
  );
}

export function PRClosedBanner({ taskId }: { taskId: string }) {
  const taskPRs = useAppStore((state) => state.taskPRs.byTaskId[taskId]);
  const [dismissed, setDismissed] = useState(() => wasPRClosedBannerDismissed(taskId));

  const handleDismiss = useCallback(() => {
    markPRClosedBannerDismissed(taskId);
    setDismissed(true);
  }, [taskId]);

  // Mirror the merged banner's all-or-nothing rule: show only once every PR is
  // closed-without-merging. A mix of merged + closed shows neither banner.
  const allClosed = !!taskPRs && taskPRs.length > 0 && taskPRs.every((pr) => pr.state === "closed");
  if (!allClosed || dismissed) return null;

  const bannerText =
    taskPRs.length === 1
      ? `PR #${taskPRs[0].pr_number} was closed without merging. You can archive this task.`
      : `All ${taskPRs.length} PRs were closed without merging. You can archive this task.`;

  return (
    <ArchiveDismissBanner
      testIdPrefix="pr-closed"
      icon={<IconGitPullRequestClosed className="h-3.5 w-3.5 shrink-0" />}
      text={bannerText}
      containerClass="flex flex-1 items-center gap-2 rounded-md bg-red-500/10 px-2 py-1 text-red-600 dark:text-red-400"
      archiveClass="underline underline-offset-2 hover:text-red-700 dark:hover:text-red-300 cursor-pointer"
      dismissClass="p-0.5 hover:bg-red-500/10 rounded cursor-pointer"
      taskId={taskId}
      onDismiss={handleDismiss}
    />
  );
}
