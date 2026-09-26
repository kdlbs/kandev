"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import type { TaskDeletionReason } from "@/lib/types/http";
import { t } from "@/lib/i18n";

// Exhaustive over TaskDeletionReason: adding or renaming a reason is a compile
// error here until a matching message is provided.
// Catalog keys, resolved in `describeReason`. Module scope, so a `t()` here
// would pin the explanations to the boot locale.
const REASON_MESSAGE_KEYS: Record<TaskDeletionReason, string> = {
  pr_approved_by_user: "task:taskClosedPrApproved",
  pr_merged_or_closed: "task:taskClosedPrMergedOrClosed",
  issue_closed: "task:taskClosedIssueClosed",
};

/** Maps a backend deletion reason (an untyped wire string) to an explanation. */
function describeReason(reason: string | undefined): string {
  const key = reason && REASON_MESSAGE_KEYS[reason as TaskDeletionReason];
  return t(key || "task:taskClosedAutomatically");
}

/**
 * Watches for task-deleted notifications and shows an explanatory toast when the
 * focused task is removed out from under the user. Mount once inside ToastProvider.
 */
export function useTaskDeletedToast() {
  const notification = useAppStore((s) => s.taskDeletedNotification);
  const clearNotification = useAppStore((s) => s.setTaskDeletedNotification);
  const { toast } = useToast();
  const shownRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!notification) return;
    if (shownRef.current.has(notification.taskId)) {
      clearNotification(null);
      return;
    }
    shownRef.current.add(notification.taskId);
    toast({
      title: notification.title
        ? t("task:taskTitledWasClosed", { title: notification.title })
        : t("task:taskClosed"),
      description: describeReason(notification.reason),
    });
    clearNotification(null);
  }, [notification, toast, clearNotification]);
}
