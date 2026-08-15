"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import type { SidebarWorkspaceActionsSlotProps } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { cn } from "@/lib/utils";

/**
 * Props forwarded to every plugin component registered for the
 * `sidebar-workspace-actions` slot (`registry.registerComponent("sidebar-workspace-actions",
 * Component)`) — the sidebar's New Task row action cluster or its mobile
 * navigation counterpart, alongside the built-in Quick Terminal and Quick
 * Chat actions. The host keeps the workspace context and presentation shape
 * stable across both surfaces, so a plugin never needs its own subscription
 * just to know the active workspace.
 */
export function AppSidebarWorkspaceActions(props: {
  workspaceId: string;
  workspaceLabel?: string;
  presentation: SidebarWorkspaceActionsSlotProps["presentation"];
}) {
  const { workspaceId, workspaceLabel, presentation } = props;
  const registry = usePluginRegistry();
  const hasRegistrations = registry.getSlotRegistrations("sidebar-workspace-actions").length > 0;

  const slotProps = useMemo<SidebarWorkspaceActionsSlotProps>(
    () => ({ workspaceId, workspaceLabel, presentation }),
    [presentation, workspaceId, workspaceLabel],
  );

  if (!hasRegistrations) return null;

  return (
    <div
      className={cn(
        "flex shrink-0 items-center",
        presentation === "mobile"
          ? "gap-2 [&_a]:min-h-11 [&_a]:min-w-11 [&_button]:min-h-11 [&_button]:min-w-11"
          : "gap-1",
      )}
      data-plugin-slot="sidebar-workspace-actions"
      data-presentation={presentation}
    >
      <PluginSlot name="sidebar-workspace-actions" slotProps={slotProps} />
    </div>
  );
}

/**
 * Shared phone navigation entry point for workspace-scoped plugin actions.
 * The same slot is mounted in the kanban drawer and at the top of the shared
 * app navigation sheet on other phone surfaces.
 */
export function MobileWorkspaceActionsSection({
  workspaceId: providedWorkspaceId,
}: {
  workspaceId?: string;
}) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((state) => state.workspaces?.activeId ?? null);
  const registry = usePluginRegistry();
  const workspaceId = providedWorkspaceId ?? activeWorkspaceId;

  if (!workspaceId || registry.getSlotRegistrations("sidebar-workspace-actions").length === 0) {
    return null;
  }

  return (
    <div
      className="flex flex-col gap-3"
      data-testid="mobile-workspace-actions"
      role="group"
      aria-label={t("common:workspace")}
    >
      <AppSidebarWorkspaceActions workspaceId={workspaceId} presentation="mobile" />
    </div>
  );
}
