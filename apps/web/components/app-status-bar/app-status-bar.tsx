"use client";

import { ConnectionStatusItem } from "./connection-status-item";
import { AppStatusBarPluginSlots } from "./app-status-bar-plugin-slots";
import { StatusSurfaceMetrics } from "@/components/system-metrics/status-surface-metrics";

type AppStatusBarProps = {
  pathname: string;
  activeWorkspaceId: string | null;
  activeTaskId: string | null;
  activeSessionId: string | null;
  density: "full" | "compact";
};

/**
 * Interaction reference: Orca StatusBar at d9d939a33b5858495ffb33489a952f1ac9293610.
 * Kandev implementation is independent; third-party notice ships in Settings > Licenses.
 */
export function AppStatusBar({
  pathname,
  activeWorkspaceId,
  activeTaskId,
  activeSessionId,
  density,
}: AppStatusBarProps) {
  return (
    <footer
      className="flex h-6 shrink-0 items-center gap-2 border-t border-border bg-background px-2 text-xs leading-none"
      data-testid="app-status-bar"
      aria-label="Application status"
    >
      <div className="flex h-full min-w-0 items-center overflow-hidden">
        <ConnectionStatusItem className="h-full" />
        <div
          className="flex h-full min-w-0 items-center gap-1 overflow-hidden"
          data-testid="app-status-bar-left-plugins"
        >
          <AppStatusBarPluginSlots
            placement="left"
            presentation="bar"
            density={density}
            pathname={pathname}
            activeWorkspaceId={activeWorkspaceId}
            activeTaskId={activeTaskId}
            activeSessionId={activeSessionId}
          />
        </div>
      </div>
      <div className="min-w-0 flex-1" />
      <div className="flex h-full min-w-0 items-center overflow-hidden">
        <StatusSurfaceMetrics
          activeSessionId={activeSessionId}
          presentation="bar"
          density={density}
          drawerOpen={false}
        />
        <div
          className="flex h-full min-w-0 items-center gap-1 overflow-hidden"
          data-testid="app-status-bar-right-plugins"
        >
          <AppStatusBarPluginSlots
            placement="right"
            presentation="bar"
            density={density}
            pathname={pathname}
            activeWorkspaceId={activeWorkspaceId}
            activeTaskId={activeTaskId}
            activeSessionId={activeSessionId}
          />
        </div>
      </div>
    </footer>
  );
}
