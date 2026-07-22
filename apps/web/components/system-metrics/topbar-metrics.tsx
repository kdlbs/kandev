"use client";

import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { StatusSurfaceMetrics } from "./status-surface-metrics";

type TopbarMetricsProps = {
  activeSessionId?: string | null;
  size?: "sm" | "lg";
};

/** Preserves pre-status-bar metrics until the new surface is enabled. */
export function TopbarMetrics({ size = "lg" }: TopbarMetricsProps) {
  const statusBarEnabled = useFeature("appStatusBar");
  const metricsEnabled = useAppStore(
    (state) => state.userSettings.systemMetricsDisplay.showInTopbar,
  );

  if (statusBarEnabled || !metricsEnabled) return null;

  return (
    <div
      className={`flex items-center overflow-hidden ${size === "sm" ? "h-7" : "h-8"}`}
      data-testid="topbar-metrics"
    >
      <StatusSurfaceMetrics presentation="bar" density="compact" drawerOpen />
    </div>
  );
}
