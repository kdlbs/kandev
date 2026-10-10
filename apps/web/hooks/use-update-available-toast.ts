"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import type { UpdateAvailableNotification } from "@/lib/state/slices/ui/types";
import { t } from "@/lib/i18n";

function summaryNotificationCopy(notification: UpdateAvailableNotification) {
  return {
    runtime: true,
    title: t("agents:runtimeSummaryTitle", { count: notification.runtime_updates?.length ?? 0 }),
    body: t("agents:runtimeSummaryBody"),
    action: {
      href: notification.url || "/settings/agents#runtime-updates",
      label: t("agents:runtimeReviewUpdates"),
    },
  };
}

function runtimeCopyKey(status: UpdateAvailableNotification["runtime_update_status"]) {
  switch (status) {
    case "succeeded":
      return "runtimeSucceeded";
    case "failed":
      return "runtimeFailed";
    case "interrupted":
      return "runtimeInterrupted";
    default:
      return "runtimeAvailable";
  }
}

function runtimeNotificationCopy(notification: UpdateAvailableNotification) {
  const copyKey = runtimeCopyKey(notification.runtime_update_status);
  const values = {
    agent: notification.display_name || notification.agent_name,
    runtime: notification.runtime_id,
    previous: notification.previous_version || t("agents:runtimeUnknown"),
    version: notification.version,
  };
  return {
    runtime: true,
    title: t(`agents:${copyKey}Title`, values),
    body: t(`agents:${copyKey}Body`, values),
    action: {
      href:
        notification.url ||
        `/settings/agents#runtime-update-${encodeURIComponent(notification.agent_name!)}`,
      label: t("agents:runtimeReview"),
    },
  };
}

function kandevNotificationCopy(notification: UpdateAvailableNotification) {
  return {
    runtime: false,
    title: notification.title || t("task:kandevUpdateAvailable"),
    body: notification.body || t("task:updateAvailableToast", { version: notification.version }),
    action: undefined,
  };
}

function notificationCopy(notification: UpdateAvailableNotification) {
  if (notification.notification_kind === "agent_runtime_summary") {
    return summaryNotificationCopy(notification);
  }
  if (!notification.agent_name) return kandevNotificationCopy(notification);
  return runtimeNotificationCopy(notification);
}

export function useUpdateAvailableToast() {
  const notification = useAppStore((s) => s.updateAvailableNotification);
  const clearNotification = useAppStore((s) => s.setUpdateAvailableNotification);
  const { toast } = useToast();
  const shownRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!notification) return;
    if (shownRef.current.has(notification.occurrence_id)) {
      clearNotification(null);
      return;
    }
    shownRef.current.add(notification.occurrence_id);

    const { runtime, title, body, action } = notificationCopy(notification);
    toast({
      title,
      description: body,
      placement: "top",
      ...(runtime && action && { action }),
    });

    if (nativeNotifications.isAvailable()) {
      void nativeNotifications
        .show({ eventId: `system.update_available:${notification.occurrence_id}`, title, body })
        .catch(() => undefined);
    } else if (typeof Notification !== "undefined" && Notification.permission === "granted") {
      new Notification(title, { body });
    }

    clearNotification(null);
  }, [notification, toast, clearNotification]);
}
