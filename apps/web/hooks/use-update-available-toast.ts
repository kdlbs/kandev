"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import type { UpdateAvailableNotification } from "@/lib/state/slices/ui/types";
import { t } from "@/lib/i18n";

function notificationCopy(notification: UpdateAvailableNotification) {
  const runtime = Boolean(notification.agent_name);
  const status = notification.runtime_update_status;
  let copyKey = "runtimeAvailable";
  if (status === "succeeded") copyKey = "runtimeSucceeded";
  if (status === "failed") copyKey = "runtimeFailed";
  if (status === "interrupted") copyKey = "runtimeInterrupted";
  const values = {
    agent: notification.display_name || notification.agent_name,
    runtime: notification.runtime_id,
    previous: notification.previous_version || t("agents:runtimeUnknown"),
    version: notification.version,
  };
  const title = runtime
    ? t(`agents:${copyKey}Title`, values)
    : notification.title || t("task:kandevUpdateAvailable");
  const body = runtime
    ? t(`agents:${copyKey}Body`, values)
    : notification.body || t("task:updateAvailableToast", { version: notification.version });
  return { runtime, title, body };
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

    const { runtime, title, body } = notificationCopy(notification);
    toast({
      title,
      description: body,
      placement: "top",
      ...(runtime && {
        action: {
          href: `/settings/agents#runtime-update-${encodeURIComponent(notification.agent_name!)}`,
          label: t("agents:runtimeReview"),
        },
      }),
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
