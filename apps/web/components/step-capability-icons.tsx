import type React from 'react';
import type { Icon } from '@tabler/icons-react';
import {
  IconArrowRight,
  IconClipboard,
  IconDoorExit,
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

const TRANSITION_TYPES = ['move_to_next', 'move_to_previous', 'move_to_step'];

type CapabilityDef = {
  key: string;
  icon: Icon;
  tooltip: string;
  check: (events: KanbanStepEvents) => boolean;
};

const CAPABILITIES: CapabilityDef[] = [
  {
    key: 'onTurnStart',
    icon: IconMessageForward,
    tooltip: 'On user message',
    check: (e) => e.on_turn_start?.some((a) => TRANSITION_TYPES.includes(a.type)) ?? false,
  },
  {
    key: 'autoStart',
    icon: IconRobot,
    tooltip: 'Auto-start agent',
    check: (e) => e.on_enter?.some((a) => a.type === 'auto_start_agent') ?? false,
  },
  {
    key: 'planMode',
    icon: IconClipboard,
    tooltip: 'Plan mode',
    check: (e) => e.on_enter?.some((a) => a.type === 'enable_plan_mode') ?? false,
  },
  {
    key: 'transition',
    icon: IconArrowRight,
    tooltip: 'Auto-transition',
    check: (e) => e.on_turn_complete?.some((a) => TRANSITION_TYPES.includes(a.type)) ?? false,
  },
  {
    key: 'onExit',
    icon: IconDoorExit,
    tooltip: 'On exit actions',
    check: (e) => (e.on_exit?.length ?? 0) > 0,
  },
];

export function StepCapabilityIcons({ events, className, fallback }: StepCapabilityIconsProps) {
  const defaultClassName = 'flex items-center gap-1.5 text-muted-foreground';
  const activeCapabilities = events
    ? CAPABILITIES.filter((cap) => cap.check(events))
    : [];

  if (activeCapabilities.length === 0) {
    return fallback ? <div className={className ?? defaultClassName}>{fallback}</div> : null;
  }

  return (
    <div className={className ?? defaultClassName}>
      {activeCapabilities.map((cap) => {
        const IconComponent = cap.icon;
        return (
          <TooltipProvider key={cap.key}>
            <Tooltip>
              <TooltipTrigger asChild>
                <IconComponent className="h-3.5 w-3.5" />
              </TooltipTrigger>
              <TooltipContent>{cap.tooltip}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        );
      })}
    </div>
  );
}
