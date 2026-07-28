"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";

export function useUpdateAvailableToast() {
  const notification = useAppStore((s) => s.updateAvailableNotification);
  const clearNotification = useAppStore((s) => s.setUpdateAvailableNotification);
  const { toast } = useToast();
  const shownRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!notification) return;
    if (shownRef.current.has(notification.version)) {
      clearNotification(null);
      return;
    }
    shownRef.current.add(notification.version);

    const title = notification.title || "Kandev update available";
    const body =
      notification.body ||
      `Kandev ${notification.version} is available. Open Settings > System > Updates to review it.`;
    toast({ title, description: body });

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
