"use client";

import { useMemo } from "react";
import { PluginSlot } from "@/components/plugins/plugin-slot";

/**
 * Props forwarded to every plugin component registered for the
 * `sidebar-workspace-actions` slot (`registry.registerComponent("sidebar-workspace-actions",
 * Component)`) — the sidebar's New Task row action cluster, alongside the
 * built-in Quick Terminal and Quick Chat icons. Forwarded from the same
 * `useAppStore` reads that row already performs, so a plugin never needs its
 * own subscription just to know the active workspace.
 */
export type SidebarWorkspaceActionsSlotProps = {
  /** Active workspace, or null when none is active (the row itself hides in that case). */
  workspaceId: string | null;
  /** Human-readable label of that workspace, when known. */
  workspaceLabel?: string;
};

/**
 * Plugin extension point in the sidebar's New Task row action cluster,
 * rendered after the first-party Quick Terminal and Quick Chat icons. Each
 * registered component is isolated behind its own error boundary via
 * `PluginSlot`, so a throwing plugin component never takes the built-in
 * actions down with it.
 */
export function AppSidebarWorkspaceActions(props: {
  workspaceId: string | null;
  workspaceLabel?: string;
}) {
  const { workspaceId, workspaceLabel } = props;

  const slotProps = useMemo<SidebarWorkspaceActionsSlotProps>(
    () => ({ workspaceId, workspaceLabel }),
    [workspaceId, workspaceLabel],
  );

  return <PluginSlot name="sidebar-workspace-actions" slotProps={slotProps} />;
}
