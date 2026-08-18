"use client";

import { useDroppable } from "@dnd-kit/core";
import { cn } from "@/lib/utils";
import type { WorkflowStep } from "../kanban-column";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  if (!isDragging) return null;

  return (
    <div className="fixed inset-x-0 bottom-0 z-50 bg-gradient-to-t from-background via-background to-transparent p-4">
      <div className="mx-auto flex max-h-[50dvh] max-w-sm flex-col gap-2 overflow-y-auto overscroll-contain pb-safe">
        {steps.map((step) => (
          <MobileDropTarget key={step.id} step={step} isCurrentStep={step.id === currentStepId} />
        ))}
      </div>
      <p className="text-xs text-muted-foreground text-center mt-2">
        {t("kanban:dropOnAColumnToMove")}
      </p>
    </div>
  );
}

function DesktopAutoHiddenDropTarget({ step }: { step: WorkflowStep }) {
  const { setNodeRef, isOver } = useDroppable({ id: step.id });
  return (
    <div
      ref={setNodeRef}
      data-testid={`auto-hidden-drop-target-${step.id}`}
      className={cn(
        "flex min-h-12 min-w-40 shrink-0 items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-2 text-sm font-medium transition-colors",
        isOver
          ? "border-primary bg-primary/10 text-foreground opacity-100"
          : "border-muted-foreground/40 bg-background/70 text-muted-foreground opacity-70",
      )}
    >
      <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: step.color }} />
      <span className="max-w-40 truncate">{step.title}</span>
    </div>
  );
}

export function DesktopAutoHiddenDropTargets({
  steps,
  isDragging,
}: {
  steps: WorkflowStep[];
  isDragging: boolean;
}) {
  if (!isDragging || steps.length === 0) return null;
  return (
    <div
      data-testid="desktop-auto-hidden-drop-targets"
      className="pointer-events-none absolute inset-x-3 bottom-3 z-40 flex justify-center"
    >
      <div className="pointer-events-auto flex max-w-full gap-2 overflow-x-auto rounded-xl border bg-background/85 p-2 shadow-lg backdrop-blur-sm">
        {steps.map((step) => (
          <DesktopAutoHiddenDropTarget key={step.id} step={step} />
        ))}
      </div>
    </div>
  );
}
