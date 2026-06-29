"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

/** Maps a backend deletion reason to a human-readable explanation. */
function describeReason(reason: string | undefined): string {
  switch (reason) {
    case "pr_approved_by_user":
      return "Its pull request was approved, so this review task was closed automatically.";
    case "pr_merged_or_closed":
      return "Its pull request was merged or closed, so this review task was closed automatically.";
    case "issue_closed":
      return "Its issue was closed, so this task was closed automatically.";
    default:
      return "This task was closed automatically.";
  }
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
      title: notification.title ? `"${notification.title}" was closed` : "Task closed",
      description: describeReason(notification.reason),
    });
    clearNotification(null);
  }, [notification, toast, clearNotification]);
}
