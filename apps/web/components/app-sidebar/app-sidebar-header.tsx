"use client";

import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import { IconLayoutSidebarLeftCollapse, IconLayoutSidebarLeftExpand } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useOfficeModeState } from "@/hooks/use-in-office";
import { cn } from "@/lib/utils";
import { workspaceHomeHref } from "./app-sidebar-workspace-navigation";
import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

type AppSidebarHeaderProps = {
  collapsed: boolean;
  onToggleCollapse: () => void;
};

const COLLAPSE_BUTTON_CLASS = "h-7 w-7 shrink-0 cursor-pointer";

export function AppSidebarHeader({ collapsed, onToggleCollapse }: AppSidebarHeaderProps) {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces);
  // The global WORKSPACE_PICKER shortcut opens this instance (and only this
  // one) through the store; the mobile sheet keeps its own local open state.
  const pickerOpen = useAppStore((s) => s.appSidebar.workspacePickerOpen);
  const setPickerOpen = useAppStore((s) => s.setWorkspacePickerOpen);
  const mode = useOfficeModeState();
  const activeWorkspace = workspaces.items.find(
    (workspace) => workspace.id === workspaces.activeId,
  );
  const homeDisabled = mode === "unknown";
  const homeHref = homeDisabled ? "#" : workspaceHomeHref(activeWorkspace);

  if (collapsed) {
    // Minimal rail: brand home + expand. The workspace switcher lives only in
    // the expanded header — a lone workspace glyph here read as noise.
    return (
      <div
        data-testid="app-sidebar-header"
        data-window-controls-overlay-region="sidebar"
        className="flex flex-col items-center gap-1 px-1 py-1.5 border-b border-border shrink-0"
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <Link
              href={homeHref}
              aria-disabled={homeDisabled || undefined}
              onClick={homeDisabled ? (event) => event.preventDefault() : undefined}
              aria-label={t("sidebar:kandevHome")}
              className="flex h-7 w-7 items-center justify-center rounded-md text-foreground/80 hover:bg-muted/60 cursor-pointer"
            >
              <span className="text-base font-bold tracking-tight">K</span>
            </Link>
          </TooltipTrigger>
          <TooltipContent side="right">Kandev</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className={COLLAPSE_BUTTON_CLASS}
              onClick={onToggleCollapse}
              aria-label={t("sidebar:expandSidebar")}
            >
              <IconLayoutSidebarLeftExpand className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">{t("sidebar:expandSidebar")}</TooltipContent>
        </Tooltip>
      </div>
    );
  }

  // Single h-10 row — brand · workspace picker · collapse — so the sidebar's
  // top section lines up with the page/dockview top bar (also h-10). Brand and
  // workspace share the same text size so they sit on a common baseline; the
  // brand carries weight/colour, the workspace stays muted and secondary.
  return (
    <div
      data-testid="app-sidebar-header"
      data-window-controls-overlay-region="sidebar"
      className="flex items-center gap-1.5 h-10 px-3 shrink-0 border-b border-border"
    >
      <Link
        href={homeHref}
        aria-disabled={homeDisabled || undefined}
        onClick={homeDisabled ? (event) => event.preventDefault() : undefined}
        aria-label={t("sidebar:kandevHome")}
        className={cn(
          "shrink-0 cursor-pointer text-sm font-semibold tracking-tight",
          "text-foreground hover:text-foreground/80 transition-colors",
        )}
      >
        Kandev
      </Link>
      <span aria-hidden className="shrink-0 select-none text-muted-foreground/30">
        /
      </span>
      <AppSidebarWorkspacePicker open={pickerOpen} onOpenChange={setPickerOpen} />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className={COLLAPSE_BUTTON_CLASS}
            onClick={onToggleCollapse}
            aria-label={t("sidebar:collapseSidebar")}
          >
            <IconLayoutSidebarLeftCollapse className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{t("sidebar:collapseSidebar")}</TooltipContent>
      </Tooltip>
    </div>
  );
}
