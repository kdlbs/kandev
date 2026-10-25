"use client";

import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { ShortcutActivity } from "@/hooks/domains/sidebar/use-shortcut-activity";

type ShortcutActivityIndicatorProps = {
  activity: ShortcutActivity;
  className?: string;
};

const ACTIVITY_LABEL_KEYS = {
  running: "automations:stateRunning",
  idle: "automations:stateIdle",
  paused: "automations:statePaused",
  unknown: "common:unknown",
} as const;

export function shortcutActivityLabel(activity: ShortcutActivity, t: (key: string) => string) {
  return activity.error
    ? t("automations:failedToLoadAutomationActivity")
    : t(ACTIVITY_LABEL_KEYS[activity.state]);
}

export function ShortcutActivityIndicator({ activity, className }: ShortcutActivityIndicatorProps) {
  const { t } = useTranslation();
  return (
    <span
      aria-hidden="true"
      className={cn(
        "absolute -right-1 -top-1 h-2 w-2 rounded-full ring-2 ring-background",
        activity.state === "running" && "bg-blue-500",
        activity.state === "idle" && "bg-emerald-500",
        activity.state === "paused" && "bg-muted-foreground/60",
        activity.state === "unknown" && "bg-muted-foreground/40",
        className,
      )}
      data-state={activity.state}
      data-testid="sidebar-shortcut-activity-indicator"
      title={shortcutActivityLabel(activity, t)}
    />
  );
}
