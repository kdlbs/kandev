'use client';

import { memo, useCallback, useMemo, useState } from 'react';
import { cn } from '@kandev/ui/lib/utils';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@kandev/ui/hover-card';
import { Button } from '@kandev/ui/button';
import { IconArrowRight } from '@tabler/icons-react';
import { moveTask } from '@/lib/api';
import { StepCapabilityIcons } from '@/components/step-capability-icons';
import type { KanbanStepEvents } from '@/lib/state/slices/kanban/types';

type Step = {
  id: string;
  name: string;
  color: string;
  position: number;
  events?: KanbanStepEvents;
  allow_manual_move?: boolean;
  prompt?: string;
  is_start_step?: boolean;
};

type WorkflowStepperProps = {
  steps: Step[];
  currentStepId: string | null;
  taskId?: string | null;
  workflowId?: string | null;
  isArchived?: boolean;
};

const WorkflowStepper = memo(function WorkflowStepper({
  steps,
  currentStepId,
  taskId,
  workflowId,
  isArchived,
}: WorkflowStepperProps) {
  const [movingToStepId, setMovingToStepId] = useState<string | null>(null);

  const sortedSteps = useMemo(
    () => [...steps].sort((a, b) => a.position - b.position),
    [steps]
  );

  const currentIndex = useMemo(
    () => sortedSteps.findIndex((s) => s.id === currentStepId),
    [sortedSteps, currentStepId]
  );

  const handleMove = useCallback(
    async (stepId: string) => {
      if (!taskId || !workflowId) return;
      setMovingToStepId(stepId);
      try {
        await moveTask(taskId, {
          workflow_id: workflowId,
          workflow_step_id: stepId,
          position: 0,
        });
      } catch (err) {
        console.error('[WorkflowStepper] Failed to move task:', err);
      } finally {
        setMovingToStepId(null);
      }
    },
    [taskId, workflowId]
  );

  if (sortedSteps.length === 0) return null;

  return (
    <div className="flex items-center gap-0 overflow-x-auto flex-shrink min-w-0">
      {sortedSteps.map((step, index) => {
        const isCompleted = !isArchived && currentIndex >= 0 && index < currentIndex;
        const isCurrent = !isArchived && index === currentIndex;
        const isAdjacent =
          currentIndex >= 0 &&
          (index === currentIndex - 1 || index === currentIndex + 1);
        const canMove =
          !isArchived && !isCurrent && taskId && workflowId && (isAdjacent || step.allow_manual_move);

        return (
          <div key={step.id} className="flex items-center">
            {/* Connector line before (except first) */}
            {index > 0 && (
              <div
                className={cn(
                  'h-px w-6 shrink-0',
                  isCompleted || isCurrent
                    ? 'bg-muted-foreground/40'
                    : 'bg-border'
                )}
              />
            )}

            <HoverCard openDelay={200} closeDelay={100}>
              <HoverCardTrigger asChild>
                {/* Step circle + label */}
                <div
                  className={cn(
                    'flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs whitespace-nowrap transition-colors cursor-default',
                    isCurrent ? 'bg-muted/40' : 'hover:bg-muted/30'
                  )}
                >
                  {/* Circle indicator */}
                  <span className="relative flex items-center justify-center shrink-0">
                    {isCurrent ? (
                      <>
                        <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
                        <span className="h-2 w-2 rounded-full bg-primary" />
                      </>
                    ) : isCompleted ? (
                      <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
                    ) : (
                      <span className="h-2 w-2 rounded-full border border-muted-foreground/40" />
                    )}
                  </span>

                  {/* Label */}
                  <span
                    className={cn(
                      'text-xs leading-none',
                      isCurrent
                        ? 'text-foreground font-medium'
                        : isCompleted
                          ? 'text-muted-foreground'
                          : 'text-muted-foreground/60'
                    )}
                  >
                    {step.name}
                  </span>
                </div>
              </HoverCardTrigger>
              <HoverCardContent
                side="bottom"
                align="center"
                className="w-auto min-w-28 p-1.5 flex flex-col items-center gap-1.5"
              >
                {canMove && (
                  <Button
                    size="sm"
                    variant="default"
                    className="cursor-pointer text-xs h-6 px-2.5 rounded-sm"
                    disabled={movingToStepId === step.id}
                    onClick={() => handleMove(step.id)}
                  >
                    <IconArrowRight className="h-3 w-3" />
                    {movingToStepId === step.id ? 'Moving...' : 'Move here'}
                  </Button>
                )}
                {isCurrent && (
                  <div className="text-[11px] text-muted-foreground">
                    Current step
                  </div>
                )}
                <StepCapabilityIcons events={step.events} />
              </HoverCardContent>
            </HoverCard>
          </div>
        );
      })}
      {isArchived && (
        <>
          <div className="h-px w-6 shrink-0 bg-border" />
          <span className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap">
            Archived
          </span>
        </>
      )}
    </div>
  );
});

export { WorkflowStepper };
export type { Step as WorkflowStepperStep };
