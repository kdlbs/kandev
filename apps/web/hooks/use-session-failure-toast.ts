"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import { listNotificationProviders } from "@/lib/api";
import type { NotificationProvider } from "@/lib/types/http";
import { t } from "@/lib/i18n";

function localSessionNotificationsEnabled(
  providers: Array<{ type: string; enabled: boolean; events: string[] }>,
): boolean {
  return providers.some((provider) => provider.type === "local" && provider.enabled);
}

/** Watches for session failure notifications and shows an error toast. Mount once inside ToastProvider. */
export function useSessionFailureToast() {
  const notification = useAppStore((s) => s.sessionFailureNotification);
  const clearNotification = useAppStore((s) => s.setSessionFailureNotification);
  const notificationProviders = useAppStore((s) => s.notificationProviders.items);
  const notificationProvidersLoaded = useAppStore((s) => s.notificationProviders.loaded);
  const setNotificationProviders = useAppStore((s) => s.setNotificationProviders);
  const setNotificationProvidersLoading = useAppStore((s) => s.setNotificationProvidersLoading);
  const { toast } = useToast();
  const shownRef = useRef<Set<string>>(new Set());
  const providerLoadRef = useRef<Promise<NotificationProvider[]> | null>(null);

  useEffect(() => {
    if (!notification) return;
    if (shownRef.current.has(notification.sessionId)) {
      clearNotification(null);
      return;
    }
    shownRef.current.add(notification.sessionId);
    const description = notification.isLaunchFailure
      ? t("task:launchFailedSeeDetails")
      : notification.message;
    toast({
      title: t("task:taskFailedToStart"),
      description,
      variant: "error",
    });
    if (nativeNotifications.isAvailable()) {
      void (async () => {
        try {
          let providers = notificationProviders;
          if (!notificationProvidersLoaded) {
            if (!providerLoadRef.current) {
              setNotificationProvidersLoading(true);
              providerLoadRef.current = listNotificationProviders({ cache: "no-store" })
                .then((response) => {
                  const nextProviders = response.providers ?? [];
                  setNotificationProviders({
                    items: nextProviders,
                    events: response.events ?? [],
                    appriseAvailable: response.apprise_available ?? false,
                    loaded: true,
                    loading: false,
                  });
                  return nextProviders;
                })
                .catch(() => {
                  setNotificationProviders({
                    items: [],
                    events: [],
                    appriseAvailable: false,
                    loaded: true,
                    loading: false,
                  });
                  return [];
                });
            }
            providers = await providerLoadRef.current;
          }
          if (!localSessionNotificationsEnabled(providers)) return;
          await nativeNotifications.show({
            eventId: `session.failed:${notification.sessionId}`,
            title: t("task:taskFailedToStart"),
            body: description,
            taskId: notification.taskId,
            sessionId: notification.sessionId,
          });
        } catch {
          // The in-app failure toast remains authoritative when native delivery fails.
        }
      })();
    }
    clearNotification(null);
  }, [
    notification,
    notificationProviders,
    notificationProvidersLoaded,
    toast,
    clearNotification,
    setNotificationProviders,
    setNotificationProvidersLoading,
  ]);
}
