"use client";

import { useCallback, useState, type ReactNode } from "react";
import { IconGitMerge, IconGitPullRequestClosed, IconX } from "@tabler/icons-react";
import { TaskArchiveConfirmFlow } from "@/components/task/task-archive-confirm-flow";
import { useAppStore } from "@/components/state-provider";
import { useTaskArchiveConfirm } from "@/hooks/use-task-archive-confirm";
import {
  markPRClosedBannerDismissed,
  markPRMergedBannerDismissed,
  wasPRClosedBannerDismissed,
  wasPRMergedBannerDismissed,
} from "@/lib/local-storage";
import { useTranslation } from "react-i18next";

// Presentational banner shared by PRMergedBanner / PRClosedBanner — an icon, a
// message, and Archive + Dismiss controls. Colors/icon/testIds are supplied by
// the caller so the two variants stay visually distinct. The Archive control
// routes through the shared preference-aware archive flow.
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
  const { t } = useTranslation();
  const archive = useTaskArchiveConfirm(taskId);
  return (
    <>
      <div data-testid={`${testIdPrefix}-banner`} className={containerClass}>
        {icon}
        <span className="flex-1">{text}</span>
        <button
          type="button"
          data-testid={`${testIdPrefix}-archive-button`}
          onClick={archive.requestArchive}
          className={archiveClass}
        >
          {t("task:archive")}
        </button>
        <button
          type="button"
          aria-label={t("task:dismiss")}
          data-testid={`${testIdPrefix}-dismiss-button`}
          onClick={onDismiss}
          className={dismissClass}
        >
          <IconX className="h-3 w-3" />
        </button>
      </div>
      <TaskArchiveConfirmFlow
        taskId={taskId}
        archive={archive}
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
