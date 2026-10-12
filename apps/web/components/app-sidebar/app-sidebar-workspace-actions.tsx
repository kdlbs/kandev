"use client";

import { useId, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconLayoutGrid, IconListDetails } from "@tabler/icons-react";
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
import { useHasSavedSidebarLayout } from "@/hooks/domains/sidebar/use-sidebar-layout-navigation";

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
        "flex items-center",
        presentation === "mobile"
          ? "min-w-0 max-w-full flex-wrap gap-2 [&_a:not([data-slot=surface-action])]:min-h-11 [&_a:not([data-slot=surface-action])]:min-w-11 [&_button:not([data-slot=surface-action])]:min-h-11 [&_button:not([data-slot=surface-action])]:min-w-11"
          : "min-w-0 max-w-full flex-wrap gap-1",
      )}
      data-sidebar-drag-exclude
      data-plugin-slot="sidebar-workspace-actions"
      data-presentation={presentation}
    >
      <PluginSlot
        name="sidebar-workspace-actions"
        slotProps={slotProps}
        actionSurface={{ surface: "sidebar", presentation }}
      />
    </div>
  );
}

/**
 * Workspace canvases and optional plugin actions for navigation menus.
 * Phone app navigation renders plugin actions in its dedicated Plugins section.
 */
export function MobileWorkspaceActionsSection({
  workspaceId: providedWorkspaceId,
  includePluginActions = true,
  collapseCanvases = false,
}: {
  workspaceId?: string;
  includePluginActions?: boolean;
  collapseCanvases?: boolean;
}) {
  const { t } = useTranslation();
  const [canvasesExpanded, setCanvasesExpanded] = useState(false);
  const canvasesContentId = useId();
  const activeWorkspaceId = useAppStore((state) => state.workspaces?.activeId ?? null);
  const canvasesEnabled = useFeature("canvases");
  const hasSavedSidebarLayout = useHasSavedSidebarLayout();
  const showCanvases = canvasesEnabled && !hasSavedSidebarLayout;
  const registry = usePluginRegistry();
  const pathname = usePathname();
  const workspaceId = providedWorkspaceId ?? activeWorkspaceId;
  const canvases = useWorkspaceCanvases(showCanvases ? workspaceId : null);
  const activeCanvases = canvases.filter(isActiveWorkspaceCanvas);
  const hasPluginActions =
    includePluginActions && registry.getSlotRegistrations("sidebar-workspace-actions").length > 0;

  if (!workspaceId || (!hasPluginActions && !showCanvases)) {
    return null;
  }

  return (
    <div
      className="flex flex-col gap-3"
      data-testid="mobile-workspace-actions"
      role="group"
      aria-label={t("common:workspace")}
    >
      {showCanvases && (
        <div className="flex flex-col gap-1" data-testid="mobile-workspace-canvases">
          <WorkspaceCanvasHeading
            controlsId={canvasesContentId}
            collapsible={collapseCanvases}
            expanded={canvasesExpanded}
            onToggle={() => setCanvasesExpanded(!canvasesExpanded)}
          />
          <div id={canvasesContentId} hidden={collapseCanvases && !canvasesExpanded}>
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
        </div>
      )}
      {hasPluginActions && (
        <AppSidebarWorkspaceActions workspaceId={workspaceId} presentation="mobile" />
      )}
    </div>
  );
}

function WorkspaceCanvasHeading({
  controlsId,
  collapsible,
  expanded,
  onToggle,
}: {
  controlsId: string;
  collapsible: boolean;
  expanded: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const label = (
    <>
      <IconLayoutGrid className="size-4" aria-hidden="true" />
      <span className="flex-1">{t("canvases:canvases")}</span>
    </>
  );
  if (!collapsible)
    return (
      <div className="flex items-center gap-2 px-1 text-xs font-semibold text-muted-foreground">
        {label}
      </div>
    );
  return (
    <button
      type="button"
      aria-expanded={expanded}
      aria-controls={controlsId}
      onClick={onToggle}
      className="flex min-h-11 w-full cursor-pointer items-center gap-2 text-left text-sm font-medium"
    >
      {label}
      <IconChevronRight className={cn("size-3.5", expanded && "rotate-90")} aria-hidden="true" />
    </button>
  );
}
