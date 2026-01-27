'use client';

import type { WorkflowStep } from '@/lib/types/http';
import { cn } from '@kandev/ui/lib/utils';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@kandev/ui/tooltip';

type SessionWorkflowProgressProps = {
  steps: WorkflowStep[];
  currentStepId: string | null;
  className?: string;
};

export function SessionWorkflowProgress({
  steps,
  currentStepId,
  className,
}: SessionWorkflowProgressProps) {
  const currentStepIndex = currentStepId
    ? steps.findIndex((step) => step.id === currentStepId)
    : -1;

  return (
    <div className={cn('flex items-center gap-1', className)}>
      {steps.map((step, i) => (
        <Tooltip key={step.id}>
          <TooltipTrigger asChild>
            <div
              className={cn(
                'w-2 h-2 rounded-full',
                i < currentStepIndex && 'bg-green-500',
                i === currentStepIndex && 'bg-blue-500 ring-2 ring-blue-200',
                i > currentStepIndex && 'bg-gray-300'
              )}
            />
          </TooltipTrigger>
          <TooltipContent>{step.name}</TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}

