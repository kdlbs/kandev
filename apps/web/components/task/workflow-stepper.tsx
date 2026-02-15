'use client';

import { memo, useMemo } from 'react';
import { cn } from '@kandev/ui/lib/utils';

type Step = {
  id: string;
  name: string;
  color: string;
  position: number;
};

type WorkflowStepperProps = {
  steps: Step[];
  currentStepId: string | null;
};

const WorkflowStepper = memo(function WorkflowStepper({
  steps,
  currentStepId,
}: WorkflowStepperProps) {
  const sortedSteps = useMemo(
    () => [...steps].sort((a, b) => a.position - b.position),
    [steps]
  );

  const currentIndex = useMemo(
    () => sortedSteps.findIndex((s) => s.id === currentStepId),
    [sortedSteps, currentStepId]
  );

  if (sortedSteps.length === 0) return null;

  return (
    <div className="flex items-center gap-0 overflow-x-auto flex-shrink min-w-0">
      {sortedSteps.map((step, index) => {
        const isCompleted = currentIndex >= 0 && index < currentIndex;
        const isCurrent = index === currentIndex;

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

            {/* Step circle + label */}
            <div
              className={cn(
                'flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs whitespace-nowrap',
                isCurrent && 'bg-muted/40'
              )}
            >
              {/* Circle indicator */}
              <span className="relative flex items-center justify-center shrink-0">
                {isCurrent ? (
                  /* Current: filled circle with ring */
                  <>
                    <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
                    <span className="h-2 w-2 rounded-full bg-primary" />
                  </>
                ) : isCompleted ? (
                  /* Completed: filled circle */
                  <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
                ) : (
                  /* Future: hollow circle */
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
          </div>
        );
      })}
    </div>
  );
});

export { WorkflowStepper };
export type { Step as WorkflowStepperStep };
