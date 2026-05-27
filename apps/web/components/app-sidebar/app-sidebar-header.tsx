"use client";

import { IconChevronsLeft, IconChevronsRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

type AppSidebarHeaderProps = {
  collapsed: boolean;
  onToggleCollapse: () => void;
};

export function AppSidebarHeader({ collapsed, onToggleCollapse }: AppSidebarHeaderProps) {
  return (
    <div
      className={cn(
        "flex items-center h-12 border-b border-border shrink-0",
        collapsed ? "flex-col gap-1 justify-center px-1 h-auto py-1.5" : "px-2.5 gap-2",
      )}
    >
      <AppSidebarWorkspacePicker collapsed={collapsed} />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 shrink-0 cursor-pointer"
            onClick={onToggleCollapse}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? (
              <IconChevronsRight className="h-4 w-4" />
            ) : (
              <IconChevronsLeft className="h-4 w-4" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent side={collapsed ? "right" : "top"}>
          {collapsed ? "Expand sidebar" : "Collapse sidebar"}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}
