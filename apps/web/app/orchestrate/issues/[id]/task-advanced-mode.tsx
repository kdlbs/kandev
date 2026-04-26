"use client";

import { IconArrowLeft, IconLayoutList } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { StatusIcon } from "./status-icon";
import type { Issue } from "./types";

type TaskAdvancedModeProps = {
  task: Issue;
  onToggleSimple: () => void;
};

export function TaskAdvancedMode({ task, onToggleSimple }: TaskAdvancedModeProps) {
  return (
    <div className="flex flex-col h-full">
      {/* Topbar */}
      <div className="flex items-center gap-2 px-4 h-10 border-b border-border bg-background shrink-0">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer"
          onClick={onToggleSimple}
        >
          <IconArrowLeft className="h-4 w-4" />
        </Button>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 cursor-pointer"
              onClick={onToggleSimple}
            >
              <IconLayoutList className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Switch to simple mode</TooltipContent>
        </Tooltip>
        <Separator orientation="vertical" className="h-5" />
        <StatusIcon status={task.status} className="h-4 w-4" />
        <span className="text-xs font-mono text-muted-foreground">{task.identifier}</span>
        <span className="text-sm font-medium truncate">{task.title}</span>
        <div className="ml-auto flex items-center gap-2">
          {task.assigneeName && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <div className="h-6 w-6 rounded-full bg-muted flex items-center justify-center">
                <span className="text-[10px] font-medium">
                  {task.assigneeName.charAt(0).toUpperCase()}
                </span>
              </div>
              {task.assigneeName}
            </div>
          )}
        </div>
      </div>

      {/* Dockview placeholder */}
      <div className="flex-1 min-h-0 flex items-center justify-center bg-muted/30">
        <div className="text-center space-y-2">
          <p className="text-sm text-muted-foreground">
            Advanced mode -- Dockview layout coming soon
          </p>
          <p className="text-xs text-muted-foreground">
            This will include Chat, Terminal, Plan, Files, and Changes panels.
          </p>
        </div>
      </div>
    </div>
  );
}
