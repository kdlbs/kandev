"use client";

import { useMemo } from "react";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import type { AppStatusBarSlotProps } from "@/lib/plugins/types";

/** Renders plugins registered for the app status bar with host-owned context. */
export function AppStatusBarPluginSlots({
  placement,
  presentation,
  density,
  pathname,
  activeWorkspaceId,
  activeTaskId,
  activeSessionId,
}: AppStatusBarSlotProps) {
  const slotProps = useMemo<AppStatusBarSlotProps>(
    () => ({
      placement,
      presentation,
      density,
      pathname,
      activeWorkspaceId,
      activeTaskId,
      activeSessionId,
    }),
    [placement, presentation, density, pathname, activeWorkspaceId, activeTaskId, activeSessionId],
  );

  const name = placement === "left" ? "app-status-bar-left" : "app-status-bar-right";
  return <PluginSlot name={name} slotProps={slotProps} />;
}
