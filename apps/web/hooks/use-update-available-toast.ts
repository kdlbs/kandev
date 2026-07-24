"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { nativeNotifications } from "@/lib/desktop/native-notification-client";
import { fetchUpdateNotificationSettings } from "@/lib/api/domains/system-api";
import type { UpdateNotificationSettings } from "@/lib/types/system";

// Matches the backend default (DefaultNotifySettings in
// internal/system/updates/notify_settings.go): used only when the settings
// fetch itself fails, so a transient network hiccup doesn't silently
// swallow an update notification the user would otherwise want.
const FALLBACK_SETTINGS: UpdateNotificationSettings = { enabled: true, channel: "both" };

let settingsRequest: Promise<UpdateNotificationSettings> | null = null;

function loadSettings(): Promise<UpdateNotificationSettings> {
  if (!settingsRequest) {
    settingsRequest = fetchUpdateNotificationSettings({ cache: "no-store" }).catch(
      () => FALLBACK_SETTINGS,
    );
  }
  return settingsRequest;
}

/**
 * Watches for a newly detected Kandev release (pushed over WS as
 * `system.update_available`, see lib/ws/handlers/system-events.ts) and
 * notifies the user according to their saved channel preference
 * (desktop / in_view / both — see the Updates settings page). Mount once
 * inside ToastProvider, alongside the other notification bridges.
 */
export function useUpdateAvailableToast() {
  const notification = useAppStore((s) => s.updateAvailableNotification);
  const clearNotification = useAppStore((s) => s.setUpdateAvailableNotification);
  const settings = useAppStore((s) => s.system.updateNotificationSettings);
  const setSettings = useAppStore((s) => s.setSystemUpdateNotificationSettings);
  const { toast } = useToast();
  const shownRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (!notification) return;
    if (shownRef.current.has(notification.version)) {
      clearNotification(null);
      return;
    }
    shownRef.current.add(notification.version);

    void (async () => {
      let current: UpdateNotificationSettings;
      if (settings) {
        current = settings;
      } else {
        current = await loadSettings();
        setSettings(current);
      }
      if (!current.enabled) return;

      const title = "Update available";
      const body = `Kandev ${notification.version} is available. See Settings > System > Updates.`;

      if (current.channel === "in_view" || current.channel === "both") {
        toast({ title, description: body });
      }
      if (current.channel === "desktop" || current.channel === "both") {
        if (nativeNotifications.isAvailable()) {
          void nativeNotifications
            .show({ eventId: `system.update_available:${notification.version}`, title, body })
            .catch(() => undefined);
        } else if (typeof Notification !== "undefined" && Notification.permission === "granted") {
          new Notification(title, { body });
        }
      }
    })();

    clearNotification(null);
  }, [notification, settings, setSettings, toast, clearNotification]);
}
