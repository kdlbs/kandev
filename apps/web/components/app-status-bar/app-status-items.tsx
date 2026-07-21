"use client";

import { useMemo, type ReactNode } from "react";
import { StatusSurfaceMetrics } from "@/components/system-metrics/status-surface-metrics";
import { useAppStore } from "@/components/state-provider";
import { usePluginRegistry, type PluginSlotRegistration } from "@/lib/plugins/registry";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";
import { ConnectionStatusItem } from "./connection-status-item";
import { AppStatusBarPluginContribution } from "./app-status-bar-plugin-slots";
import {
  APP_STATUS_CONNECTION_ID,
  APP_STATUS_METRICS_ID,
  type AppStatusItemDescriptor,
} from "./app-status-bar-order";

export type AppStatusItemPresentation = {
  presentation: "bar" | "mobile-drawer";
  density: "full" | "compact";
  drawerOpen: boolean;
};

export type AppStatusItem = AppStatusItemDescriptor & {
  render: (presentation: AppStatusItemPresentation) => ReactNode;
};

type AppStatusContext = Pick<
  AppStatusBarSlotProps,
  "pathname" | "activeWorkspaceId" | "activeTaskId" | "activeSessionId"
>;

export function useAppStatusItems(context: AppStatusContext): AppStatusItem[] {
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const metricsEnabled = useAppStore(
    (state) => state.userSettings.systemMetricsDisplay.showInTopbar,
  );

  return useMemo(() => {
    const left = registry.getSlotRegistrations("app-status-bar-left");
    const right = registry.getSlotRegistrations("app-status-bar-right");
    return [
      connectionItem(),
      ...left.map((registration) => pluginItem(registration, "left", context)),
      ...(metricsEnabled ? [metricsItem(context.activeSessionId)] : []),
      ...right.map((registration) => pluginItem(registration, "right", context)),
    ];
  }, [context, metricsEnabled, registry, registryVersion]);
}

function connectionItem(): AppStatusItem {
  return {
    id: APP_STATUS_CONNECTION_ID,
    defaultSide: "left",
    render: () => <ConnectionStatusItem className="h-full" />,
  };
}

function metricsItem(activeSessionId: string | null): AppStatusItem {
  return {
    id: APP_STATUS_METRICS_ID,
    defaultSide: "right",
    render: ({ presentation, density, drawerOpen }) => (
      <StatusSurfaceMetrics
        activeSessionId={activeSessionId}
        presentation={presentation}
        density={density}
        drawerOpen={drawerOpen}
      />
    ),
  };
}

function pluginItem(
  registration: PluginSlotRegistration,
  placement: "left" | "right",
  context: AppStatusContext,
): AppStatusItem {
  return {
    id: registration.orderingId,
    defaultSide: placement,
    render: ({ presentation, density }) => (
      <AppStatusBarPluginContribution
        registration={registration}
        {...context}
        placement={placement}
        presentation={presentation}
        density={density}
      />
    ),
  };
}
