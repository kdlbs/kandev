"use client";

import { IconActivity } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { StatusSurfaceMetrics } from "@/components/system-metrics/status-surface-metrics";
import { TaskChromeDisclosure } from "./task-chrome-disclosure";

export function TaskTopBarMetrics() {
  const { t } = useTranslation();
  const statusBarEnabled = useAppStore((state) => state.userSettings.appStatusBarEnabled);
  const metricsEnabled = useAppStore(
    (state) => state.userSettings.systemMetricsDisplay.showInTopbar,
  );
  if (statusBarEnabled || !metricsEnabled) return null;

  return (
    <TaskChromeDisclosure
      label={t("system:systemMetrics")}
      icon={<IconActivity className="size-4" aria-hidden />}
      testId="task-metrics-trigger"
    >
      <StatusSurfaceMetrics presentation="mobile-drawer" density="full" drawerOpen />
    </TaskChromeDisclosure>
  );
}
