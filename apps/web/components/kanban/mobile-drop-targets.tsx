"use client";

import { useDroppable } from "@dnd-kit/core";
import { cn } from "@/lib/utils";
import type { WorkflowStep } from "../kanban-column";

type MobileDropTargetProps = {
  step: WorkflowStep;
  isCurrentStep: boolean;
};

function MobileDropTarget({ step, isCurrentStep }: MobileDropTargetProps) {
  const { setNodeRef, isOver } = useDroppable({
    id: step.id,
  });

  return (
    <div
      ref={setNodeRef}
      className={cn(
        "flex min-h-11 w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed px-3 py-2 transition-all",
        (() => {
          if (isOver) return "border-primary bg-primary/10 scale-105";
          if (isCurrentStep) return "border-muted-foreground/30 bg-muted/50 opacity-50";
          return "border-muted-foreground/40 bg-background hover:border-muted-foreground/60";
        })(),
      )}
    >
      <div className={cn("w-3 h-3 rounded-full flex-shrink-0", step.color)} />
      <span className="text-sm font-medium truncate max-w-[80px]">{step.title}</span>
    </div>
  );
}

type MobileDropTargetsProps = {
  steps: WorkflowStep[];
  currentStepId: string | null;
  isDragging: boolean;
};

export function MobileDropTargets({ steps, currentStepId, isDragging }: MobileDropTargetsProps) {
  if (!isDragging) return null;

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 bg-gradient-to-t from-background via-background to-transparent p-4">
      <div className="mx-auto flex max-h-[50dvh] max-w-sm flex-col gap-2 overflow-y-auto overscroll-contain pb-safe">
        {steps.map((step) => (
          <MobileDropTarget key={step.id} step={step} isCurrentStep={step.id === currentStepId} />
        ))}
      </div>
      <p className="text-xs text-muted-foreground text-center mt-2">
        Drop on a column to move task
      </p>
    </div>
  );
}
