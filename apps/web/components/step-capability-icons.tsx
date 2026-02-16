import type React from 'react';
import {
  IconArrowRight,
  IconClipboard,
  IconMessageForward,
  IconRobot,
} from '@tabler/icons-react';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@kandev/ui/tooltip';
import type { KanbanStepEvents } from '@/lib/state/slices/kanban/types';

type StepCapabilityIconsProps = {
  events?: KanbanStepEvents;
  className?: string;
  fallback?: React.ReactNode;
};

export function StepCapabilityIcons({ events, className, fallback }: StepCapabilityIconsProps) {
  const hasOnTurnStart = events?.on_turn_start?.some((a) =>
    ['move_to_next', 'move_to_previous', 'move_to_step'].includes(a.type)
  ) ?? false;
  const hasAuto = events?.on_enter?.some((a) => a.type === 'auto_start_agent') ?? false;
  const hasPlan = events?.on_enter?.some((a) => a.type === 'enable_plan_mode') ?? false;
  const hasTransition = events?.on_turn_complete?.some((a) =>
    ['move_to_next', 'move_to_previous', 'move_to_step'].includes(a.type)
  ) ?? false;

  if (!hasOnTurnStart && !hasAuto && !hasPlan && !hasTransition) {
    return fallback ? <div className={className ?? 'flex items-center gap-1.5 text-muted-foreground'}>{fallback}</div> : null;
  }

  return (
    <div className={className ?? 'flex items-center gap-1.5 text-muted-foreground'}>
      {hasOnTurnStart && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconMessageForward className="h-3.5 w-3.5" />
            </TooltipTrigger>
            <TooltipContent>On user message</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
      {hasAuto && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconRobot className="h-3.5 w-3.5" />
            </TooltipTrigger>
            <TooltipContent>Auto-start agent</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
      {hasPlan && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconClipboard className="h-3.5 w-3.5" />
            </TooltipTrigger>
            <TooltipContent>Plan mode</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
      {hasTransition && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconArrowRight className="h-3.5 w-3.5" />
            </TooltipTrigger>
            <TooltipContent>Auto-transition</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
    </div>
  );
}
