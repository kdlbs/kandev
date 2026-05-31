"use client";

import Link from "next/link";
import { IconChevronsLeft, IconChevronsRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

type AppSidebarHeaderProps = {
  collapsed: boolean;
  onToggleCollapse: () => void;
};

const COLLAPSE_BUTTON_CLASS = "h-7 w-7 shrink-0 cursor-pointer";

export function AppSidebarHeader({ collapsed, onToggleCollapse }: AppSidebarHeaderProps) {
  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-1 px-1 py-1.5 border-b border-border shrink-0">
        <Tooltip>
          <TooltipTrigger asChild>
            <Link
              href="/"
              aria-label="Kandev home"
              className="flex h-7 w-7 items-center justify-center rounded-md text-foreground/80 hover:bg-muted/60 cursor-pointer"
            >
              <span className="text-base font-bold tracking-tight">K</span>
            </Link>
          </TooltipTrigger>
          <TooltipContent side="right">Kandev</TooltipContent>
        </Tooltip>
        <AppSidebarWorkspacePicker collapsed />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className={COLLAPSE_BUTTON_CLASS}
              onClick={onToggleCollapse}
              aria-label="Expand sidebar"
            >
              <IconChevronsRight className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">Expand sidebar</TooltipContent>
        </Tooltip>
      </div>
    );
  }

  // Single h-10 row — brand · workspace picker · collapse — so the sidebar's
  // top section lines up with the page/dockview top bar (also h-10).
  return (
    <div className="flex items-center gap-1.5 h-10 px-2 shrink-0 border-b border-border">
      <Link
        href="/"
        aria-label="Kandev home"
        className={cn(
          "flex items-center shrink-0 cursor-pointer",
          "text-foreground hover:text-foreground/80 transition-colors",
        )}
      >
        <span className="text-base font-bold tracking-tight leading-none">Kandev</span>
      </Link>
      <AppSidebarWorkspacePicker collapsed={false} />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className={COLLAPSE_BUTTON_CLASS}
            onClick={onToggleCollapse}
            aria-label="Collapse sidebar"
          >
            <IconChevronsLeft className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">Collapse sidebar</TooltipContent>
      </Tooltip>
    </div>
  );
}
