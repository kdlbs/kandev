"use client";

import Link from "next/link";
import { IconLayoutSidebarLeftCollapse, IconLayoutSidebarLeftExpand } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { workspaceHomeHref } from "./app-sidebar-workspace-navigation";
import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

type AppSidebarHeaderProps = {
  collapsed: boolean;
  onToggleCollapse: () => void;
};

const COLLAPSE_BUTTON_CLASS = "h-7 w-7 shrink-0 cursor-pointer";

export function AppSidebarHeader({ collapsed, onToggleCollapse }: AppSidebarHeaderProps) {
  const workspaces = useAppStore((s) => s.workspaces);
  const activeWorkspace = workspaces.items.find(
    (workspace) => workspace.id === workspaces.activeId,
  );
  const homeHref = workspaceHomeHref(activeWorkspace);

  if (collapsed) {
    // Minimal rail: brand home + expand. The workspace switcher lives only in
    // the expanded header — a lone workspace glyph here read as noise.
    return (
      <div className="flex flex-col items-center gap-1 px-1 py-1.5 border-b border-border shrink-0">
        <Tooltip>
          <TooltipTrigger asChild>
            <Link
              href={homeHref}
              aria-label="Kandev home"
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
              aria-label="Expand sidebar"
            >
              <IconLayoutSidebarLeftExpand className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Expand sidebar</TooltipContent>
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
      className="flex items-center gap-1.5 h-10 px-3 shrink-0 border-b border-border"
    >
      <Link
        href={homeHref}
        aria-label="Kandev home"
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
      <AppSidebarWorkspacePicker />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className={COLLAPSE_BUTTON_CLASS}
            onClick={onToggleCollapse}
            aria-label="Collapse sidebar"
          >
            <IconLayoutSidebarLeftCollapse className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">Collapse sidebar</TooltipContent>
      </Tooltip>
    </div>
  );
}
