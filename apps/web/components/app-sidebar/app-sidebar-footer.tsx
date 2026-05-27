"use client";

import Link from "next/link";
import { IconSettings } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { ThemeToggle } from "@/components/theme-toggle";
import { cn } from "@/lib/utils";

type AppSidebarFooterProps = {
  collapsed: boolean;
};

export function AppSidebarFooter({ collapsed }: AppSidebarFooterProps) {
  return (
    <div
      className={cn(
        "flex items-center h-10 border-t border-border shrink-0",
        collapsed ? "flex-col gap-1 justify-center px-1 h-auto py-1.5" : "px-2.5 justify-end gap-1",
      )}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Link href="/settings/general" className="cursor-pointer">
            <Button variant="ghost" size="icon" className="h-7 w-7 cursor-pointer">
              <IconSettings className="h-3.5 w-3.5" />
            </Button>
          </Link>
        </TooltipTrigger>
        <TooltipContent side={collapsed ? "right" : "top"}>Settings</TooltipContent>
      </Tooltip>
      <ThemeToggle />
    </div>
  );
}
