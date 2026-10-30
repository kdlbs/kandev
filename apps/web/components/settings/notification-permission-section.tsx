import { useCallback, useEffect, useState } from "react";
import { IconBell, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  nativeNotifications,
  type NativeNotificationPermission,
} from "@/lib/desktop/native-notification-client";
import type { PermissionRefresh } from "./notifications-settings-actions";
import { SettingsGroup } from "./settings-group";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";

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

/**
 * Returns a catalog key rather than copy: `permission` is a sentinel compared
 * with `===` and this helper runs outside a component, so resolving here would
 * either freeze the copy at the boot locale or need a `t` threaded through.
 */
function permissionActionLabelKey(permission: NotificationPermissionState) {
  if (permission === "granted") return "settings:notificationPermissionEnabled";
  if (permission === "error") return "settings:notificationPermissionRetry";
  return "settings:notificationPermissionEnable";
}

/**
 * Warning copy is keyed the same way as the action label so the same value
 * decides both the warning paragraph and the group frame.
 */
function permissionWarningKey(
  permission: NotificationPermissionState,
  nativeAvailable: boolean,
): string | null {
  if (permission === "denied") {
    return nativeAvailable
      ? "settings:notificationsBlockedInOs"
      : "settings:notificationsBlockedInBrowser";
  }
  if (permission === "unsupported") return "settings:notificationsUnsupported";
  if (permission === "error") return "settings:notificationPermissionCheckFailed";
  return null;
}

export function DesktopNotificationsSection({
  notificationPermission,
  onRequestPermission,
  onRefreshPermission,
  onTestNotification,
}: DesktopNotificationsSectionProps) {
  const { t } = useTranslation();
  const warningKey = permissionWarningKey(
    notificationPermission,
    nativeNotifications.isAvailable(),
  );
  const warning = warningKey ? t(warningKey) : null;
  return (
    <SettingsGroup
      title={t("settings:desktopNotifications")}
      description={t("settings:desktopNotificationsDescription")}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.desktopNotifications}
      frame={warning ? "card" : "none"}
      contentClassName={warning ? undefined : "hidden"}
      data-testid="notification-permission-group"
      action={
        <div className="flex items-center gap-2">
          <Button
            title={t("settings:enableDesktopNotifications")}
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
            {t(permissionActionLabelKey(notificationPermission))}
          </Button>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t("settings:refreshNotificationPermission")}
                  className="cursor-pointer"
                  onClick={() => void onRefreshPermission()}
                >
                  <IconRefresh className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("settings:refreshPermissionStatus")}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <HoverCard>
            <HoverCardTrigger asChild>
              <Button
                title={t("settings:sendTestNotification")}
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
            <HoverCardContent side="top">
              {t("settings:notificationsNotShowingHint")}
            </HoverCardContent>
          </HoverCard>
        </div>
      }
    >
      {warning && (
        <p data-testid="notification-permission-warning" className="text-xs text-amber-600">
          {warning}
        </p>
      )}
    </SettingsGroup>
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
