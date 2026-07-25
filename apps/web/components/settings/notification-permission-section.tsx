import { useCallback, useEffect, useState } from "react";
import { IconBell, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  nativeNotifications,
  type NativeNotificationPermission,
} from "@/lib/desktop/native-notification-client";
import type { PermissionRefresh } from "./notifications-settings-actions";

type NotificationPermissionState =
  | NativeNotificationPermission
  | NotificationPermission
  | "unsupported"
  | "error";

type DesktopNotificationsSectionProps = {
  notificationPermission: NotificationPermissionState;
  onRequestPermission: () => Promise<void>;
  onRefreshPermission: () => Promise<void>;
  onTestNotification: () => void;
};

function permissionActionLabel(permission: NotificationPermissionState) {
  if (permission === "granted") return "Enabled";
  if (permission === "error") return "Retry";
  return "Enable";
}

export function DesktopNotificationsSection({
  notificationPermission,
  onRequestPermission,
  onRefreshPermission,
  onTestNotification,
}: DesktopNotificationsSectionProps) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-base font-medium">Desktop Notifications</div>
          <p className="text-sm text-muted-foreground">
            Notify this device when a selected agent turn, question, or Office event occurs.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            title="Enable desktop notifications"
            variant="default"
            size="sm"
            onClick={() => void onRequestPermission()}
            disabled={
              notificationPermission === "granted" || notificationPermission === "unsupported"
            }
            className={
              notificationPermission === "granted"
                ? "bg-emerald-500 text-white hover:bg-emerald-500"
                : "cursor-pointer"
            }
          >
            {permissionActionLabel(notificationPermission)}
          </Button>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Refresh notification permission"
                  className="cursor-pointer"
                  onClick={() => void onRefreshPermission()}
                >
                  <IconRefresh className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Refresh permission status</TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <HoverCard>
            <HoverCardTrigger asChild>
              <Button
                title="Send test notification"
                variant="outline"
                className="cursor-pointer"
                size="icon"
                onClick={() => {
                  void onTestNotification();
                }}
              >
                <IconBell className="h-4 w-4" />
              </Button>
            </HoverCardTrigger>
            <HoverCardContent side="top" className="text-sm">
              If you do not see notifications, check your OS settings and allow this browser.
            </HoverCardContent>
          </HoverCard>
        </div>
      </div>

      {notificationPermission === "denied" && (
        <p className="text-sm text-amber-600">
          {nativeNotifications.isAvailable()
            ? "Notifications are blocked in your OS app notification settings. Enable them there, then click Refresh."
            : "Notifications are blocked in your browser. Enable them in site settings, then click Refresh."}
        </p>
      )}
      {notificationPermission === "unsupported" && (
        <p className="text-sm text-amber-600">
          This browser does not support desktop notifications.
        </p>
      )}
      {notificationPermission === "error" && (
        <p className="text-sm text-amber-600">
          Kandev could not check notification permission. Try again, then check your browser or OS
          notification settings.
        </p>
      )}
    </div>
  );
}

export function useNotificationPermission() {
  const [notificationPermission, setNotificationPermission] =
    useState<NotificationPermissionState>("default");
  const refreshPermission = useCallback<PermissionRefresh>(async (error) => {
    if (error) {
      setNotificationPermission("error");
      return;
    }
    try {
      if (nativeNotifications.isAvailable()) {
        setNotificationPermission(await nativeNotifications.permission.get());
        return;
      }
      setNotificationPermission(
        typeof Notification === "undefined" ? "unsupported" : Notification.permission,
      );
    } catch {
      setNotificationPermission("error");
    }
  }, []);
  useEffect(() => {
    void refreshPermission();
  }, [refreshPermission]);
  return { notificationPermission, refreshPermission };
}
