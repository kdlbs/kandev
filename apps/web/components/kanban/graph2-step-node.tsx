'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  IconCheck,
  IconCircleDashed,
  IconChevronLeft,
  IconChevronRight,
} from '@tabler/icons-react';
import { cn } from '@kandev/ui/lib/utils';
import { getTaskStateIcon } from '@/lib/ui/state-icons';
import { linkToSession } from '@/lib/links';
import type { Task } from '@/components/kanban-card';
import type { WorkflowStep } from '@/components/kanban-column';

type StepPhase = 'past' | 'current' | 'future';

type StatusInfo = {
  label: string;
  color: string;
};

function getStatusInfo(task: Task): StatusInfo {
  if (task.reviewStatus === 'pending' && task.state !== 'IN_PROGRESS') {
    return { label: 'Needs Approval', color: 'text-amber-500' };
  }
  if (task.reviewStatus === 'changes_requested') {
    return { label: 'Changes Requested', color: 'text-amber-500' };
  }

  switch (task.state) {
    case 'IN_PROGRESS':
    case 'SCHEDULING':
      return { label: 'Running', color: 'text-blue-500' };
    case 'WAITING_FOR_INPUT':
      return { label: 'Waiting', color: 'text-amber-500' };
    case 'COMPLETED':
      return { label: 'Completed', color: 'text-green-500' };
    case 'FAILED':
      return { label: 'Failed', color: 'text-red-500' };
    case 'CANCELLED':
      return { label: 'Cancelled', color: 'text-red-500' };
    case 'TODO':
    case 'CREATED':
      return { label: 'Todo', color: 'text-muted-foreground' };
    case 'REVIEW':
      return { label: 'Review', color: 'text-yellow-500' };
    case 'BLOCKED':
      return { label: 'Blocked', color: 'text-yellow-500' };
    default:
      return { label: 'Pending', color: 'text-muted-foreground' };
  }
}

function isRunningState(state?: string): boolean {
  return state === 'IN_PROGRESS' || state === 'SCHEDULING';
}

export type Graph2StepNodeProps = {
  step: WorkflowStep;
  phase: StepPhase;
  task: Task;
  hasPrev: boolean;
  hasNext: boolean;
  onMoveTask: (task: Task, targetStepId: string) => void;
  onPreviewTask: (task: Task) => void;
  prevStepId?: string;
  nextStepId?: string;
  isMoving?: boolean;
};

const NODE_CLASS = 'w-[130px] h-[44px] rounded-lg shrink-0 px-2.5 flex flex-col items-start justify-center';

export function Graph2StepNode({
  step,
  phase,
  task,
  hasPrev,
  hasNext,
  onMoveTask,
  onPreviewTask,
  prevStepId,
  nextStepId,
  isMoving,
}: Graph2StepNodeProps) {
  const router = useRouter();
  const [isHovered, setIsHovered] = useState(false);

  const handleClick = () => {
    if (phase !== 'current') return;
    if (task.primarySessionId) {
      router.push(linkToSession(task.primarySessionId));
    } else {
      onPreviewTask(task);
    }
  };

  if (phase === 'past') {
    return (
      <div className={cn(NODE_CLASS, 'border border-muted-foreground/20 bg-muted/30')}>
        <div className="flex items-center gap-1.5 w-full">
          <IconCheck className="h-3 w-3 text-green-500 shrink-0" />
          <span className="text-[11px] text-muted-foreground truncate">
            {step.title}
          </span>
        </div>
      </div>
    );
  }

  if (phase === 'future') {
    return (
      <div className={cn(NODE_CLASS, 'border border-dashed border-muted-foreground/20 bg-muted/10')}>
        <div className="flex items-center gap-1.5 w-full">
          <IconCircleDashed className="h-3 w-3 text-muted-foreground/40 shrink-0" />
          <span className="text-[11px] text-muted-foreground/40 truncate">
            {step.title}
          </span>
        </div>
      </div>
    );
  }

  // Current phase
  const status = getStatusInfo(task);
  const running = isRunningState(task.state);

  return (
    <div
      className="relative shrink-0"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {/* Left move button */}
      {isHovered && hasPrev && prevStepId && (
        <button
          type="button"
          disabled={isMoving}
          onClick={(e) => {
            e.stopPropagation();
            onMoveTask(task, prevStepId);
          }}
          className={cn(
            'absolute -left-3 top-1/2 -translate-y-1/2 z-10',
            'h-5 w-5 rounded-full bg-background border border-border shadow-sm',
            'flex items-center justify-center',
            'hover:bg-accent transition-colors cursor-pointer',
            isMoving && 'opacity-50 cursor-not-allowed'
          )}
        >
          <IconChevronLeft className="h-3 w-3" />
        </button>
      )}

      {/* Current node */}
      <button
        type="button"
        onClick={handleClick}
        className={cn(
          NODE_CLASS,
          'bg-background cursor-pointer',
          'hover:bg-accent/30 transition-colors',
          running
            ? 'border border-muted-foreground/20 node-border-running'
            : 'border-2 ring-1 ring-offset-1 ring-offset-background'
        )}
        style={
          running
            ? undefined
            : {
                borderColor: step.color || 'hsl(var(--primary))',
                // @ts-expect-error -- CSS custom property for ring color
                '--tw-ring-color': step.color || 'hsl(var(--primary))',
              }
        }
      >
        <div className="flex items-center gap-1.5 w-full">
          <div className="shrink-0">{getTaskStateIcon(task.state, 'h-3 w-3')}</div>
          <span className={cn('text-[10px] font-medium', status.color)}>
            {status.label}
          </span>
        </div>
        <span className="text-[11px] text-foreground/80 truncate w-full text-left">
          {step.title}
        </span>
      </button>

      {/* Right move button */}
      {isHovered && hasNext && nextStepId && (
        <button
          type="button"
          disabled={isMoving}
          onClick={(e) => {
            e.stopPropagation();
            onMoveTask(task, nextStepId);
          }}
          className={cn(
            'absolute -right-3 top-1/2 -translate-y-1/2 z-10',
            'h-5 w-5 rounded-full bg-background border border-border shadow-sm',
            'flex items-center justify-center',
            'hover:bg-accent transition-colors cursor-pointer',
            isMoving && 'opacity-50 cursor-not-allowed'
          )}
        >
          <IconChevronRight className="h-3 w-3" />
        </button>
      )}
    </div>
  );
}
