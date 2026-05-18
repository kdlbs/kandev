"use client";

import { IconChevronDown, IconPlus, IconRocket } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

/**
 * Split-button: primary "Implement" runs the plan inline in the current
 * session; the dropdown caret offers "Implement in fresh agent" which starts
 * a new session with a clean context window. `onClick(fresh)` is invoked
 * with `false` for the primary path and `true` for the fresh-agent path.
 */
export function ImplementPlanButton({ onClick }: { onClick: (fresh: boolean) => void }) {
  const primary = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          data-testid="implement-plan-button"
          className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-violet-400 rounded-r-none pr-1.5"
          onClick={() => onClick(false)}
        >
          <IconRocket className="h-4 w-4" />
          <span className="text-xs">Implement</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>Implement the plan in this session</TooltipContent>
    </Tooltip>
  );
  return (
    <div className="flex items-center">
      {primary}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            data-testid="implement-plan-menu-trigger"
            aria-label="More implement options"
            className="h-7 px-1 cursor-pointer hover:bg-muted/40 text-violet-400 rounded-l-none border-l border-violet-400/20"
          >
            <IconChevronDown className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-64">
          <DropdownMenuItem
            data-testid="implement-fresh-menu-item"
            onClick={() => onClick(true)}
            className="cursor-pointer"
          >
            <IconPlus className="h-4 w-4 mr-2 shrink-0 self-start mt-0.5" />
            <div>
              <div>Implement in fresh agent</div>
              <div className="text-[11px] text-muted-foreground font-normal">
                Starts a new session with a clean context window
              </div>
            </div>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
