'use client';

import { IconAlertCircle, IconCheck, IconEye, IconLoader2, IconX } from '@tabler/icons-react';
import type { TaskState } from '@/lib/types/http';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';

type TaskStateActionsProps = {
  state?: TaskState;
  onMarkDone?: () => void;
};

function getStateIcon(state?: TaskState) {
  switch (state) {
    case 'CREATED':
    case 'SCHEDULING':
    case 'IN_PROGRESS':
      return <IconLoader2 className="h-4 w-4 animate-spin text-blue-500" />;
    case 'REVIEW':
      return <IconEye className="h-4 w-4 text-yellow-500" />;
    case 'WAITING_FOR_INPUT':
    case 'BLOCKED':
      return <IconAlertCircle className="h-4 w-4 text-yellow-500" />;
    case 'COMPLETED':
      return <IconCheck className="h-4 w-4 text-green-500" />;
    case 'FAILED':
    case 'CANCELLED':
      return <IconX className="h-4 w-4 text-red-500" />;
    default:
      return <IconAlertCircle className="h-4 w-4 text-muted-foreground" />;
  }
}

export function TaskStateActions({ state, onMarkDone }: TaskStateActionsProps) {
  return (
    <div className="group relative flex items-center justify-end">
      <div className="transition-opacity group-hover:opacity-0">
        {getStateIcon(state)}
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
