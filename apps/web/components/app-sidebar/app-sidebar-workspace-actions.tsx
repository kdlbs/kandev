"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconLayoutGrid, IconListDetails } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { usePathname } from "@/lib/routing/client-router";
import type { SidebarWorkspaceActionsSlotProps } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { cn } from "@/lib/utils";
import { canvasHref, workspaceCanvasSettingsHref } from "@/lib/api/domains/canvas-api";
import { isActiveWorkspaceCanvas, useWorkspaceCanvases } from "./sections/canvases-section";

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
  const canvasesEnabled = useFeature("canvases");
  const registry = usePluginRegistry();
  const pathname = usePathname();
  const workspaceId = providedWorkspaceId ?? activeWorkspaceId;
  const canvases = useWorkspaceCanvases(canvasesEnabled ? workspaceId : null);
  const activeCanvases = canvases.filter(isActiveWorkspaceCanvas);
  const hasPluginActions = registry.getSlotRegistrations("sidebar-workspace-actions").length > 0;

  if (!workspaceId || (!hasPluginActions && !canvasesEnabled)) {
    return null;
  }

  return (
    <div
      className="flex flex-col gap-3"
      data-testid="mobile-workspace-actions"
      role="group"
      aria-label={t("common:workspace")}
    >
      {canvasesEnabled && (
        <div className="flex flex-col gap-1" data-testid="mobile-workspace-canvases">
          <div className="flex items-center gap-2 px-1 text-xs font-semibold text-muted-foreground">
            <IconLayoutGrid className="h-4 w-4" aria-hidden="true" />
            <span>{t("canvases:canvases")}</span>
          </div>
          {activeCanvases.length > 0 ? (
            activeCanvases.map((canvas) => (
              <Link
                key={canvas.id}
                href={canvasHref(canvas.id)}
                aria-current={pathname === canvasHref(canvas.id) ? "page" : undefined}
                className="flex min-h-11 items-center gap-3 rounded-md px-3 py-2 text-sm cursor-pointer hover:bg-muted/60"
                data-testid={`mobile-workspace-canvas-${canvas.id}`}
              >
                <IconLayoutGrid
                  className="h-4 w-4 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
                <span className="min-w-0 flex-1 truncate">{canvas.title}</span>
              </Link>
            ))
          ) : (
            <Link
              href={workspaceCanvasSettingsHref(workspaceId)}
              className="flex min-h-11 items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground cursor-pointer hover:bg-muted/60"
              data-testid="mobile-workspace-canvases-settings"
            >
              <IconListDetails className="h-4 w-4 shrink-0" aria-hidden="true" />
              <span>{t("canvases:openWorkspaceSettings")}</span>
            </Link>
          )}
        </div>
      )}
      {hasPluginActions && (
        <AppSidebarWorkspaceActions workspaceId={workspaceId} presentation="mobile" />
      )}
    </div>
  );
}
