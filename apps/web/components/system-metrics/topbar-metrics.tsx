"use client";

import { useAppStore } from "@/components/state-provider";
import { StatusSurfaceMetrics } from "./status-surface-metrics";

type TopbarMetricsProps = {
  activeSessionId?: string | null;
  size?: "sm" | "lg";
  mobile?: boolean;
};

/** Preserves topbar metrics while the user-owned status surface is hidden. */
export function TopbarMetrics({ size = "lg", mobile = false }: TopbarMetricsProps) {
  const statusBarEnabled = useAppStore((state) => state.userSettings.appStatusBarEnabled);
  const metricsEnabled = useAppStore(
    (state) => state.userSettings.systemMetricsDisplay.showInTopbar,
  );

  if (statusBarEnabled || !metricsEnabled) return null;

  return (
    <div
      className={`flex shrink-0 items-center overflow-hidden ${size === "sm" ? "h-7" : "h-8"}`}
      data-testid="topbar-metrics"
    >
      <StatusSurfaceMetrics
        presentation="bar"
        density="compact"
        drawerOpen
        iconSize={mobile ? "size-4" : undefined}
      />
    </div>
  );
}
