"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import type { PluginStatus } from "@/lib/types/plugins";

// The map keys are the backend's wire status; only the labels are copy, and
// they travel as catalog keys so they resolve at render.
const STATUS_LABEL_KEY: Record<PluginStatus, string> = {
  active: "plugins:statusActive",
  error: "plugins:statusError",
  disabled: "plugins:statusDisabled",
  registered: "plugins:statusRegistered",
  uninstalled: "plugins:statusUninstalled",
};

// green=active, red=error, gray=disabled, amber=registered, per task-20 acceptance.
const STATUS_CLASS: Record<PluginStatus, string> = {
  active: "border-green-500/40 bg-green-500/10 text-green-600 dark:text-green-400",
  error: "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400",
  disabled: "border-border bg-muted text-muted-foreground",
  registered: "border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400",
  uninstalled: "border-border bg-muted text-muted-foreground",
};

export function PluginStatusBadge({ status }: { status: PluginStatus }) {
  const { t } = useTranslation();
  return (
    <Badge variant="outline" className={STATUS_CLASS[status]}>
      {t(STATUS_LABEL_KEY[status])}
    </Badge>
  );
}
