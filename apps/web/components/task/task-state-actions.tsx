'use client';

import { IconCheck } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { getTaskStateIcon } from '@/lib/ui/state-icons';
import type { TaskState } from '@/lib/types/http';

type TaskStateActionsProps = {
  state?: TaskState;
  onMarkDone?: () => void;
};

export function TaskStateActions({ state, onMarkDone }: TaskStateActionsProps) {
  return (
    <div className="group relative flex items-center justify-end">
      <div className="transition-opacity group-hover:opacity-0">
        {getTaskStateIcon(state)}
      </div>
      <div className="absolute right-0 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label="Mark task done"
              className="text-muted-foreground hover:text-foreground cursor-pointer rounded-md border border-border/60 p-1 hover:bg-muted/50"
              onClick={(event) => {
                event.stopPropagation();
                onMarkDone?.();
              }}
            >
              <IconCheck className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent side="left">Mark as done</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
