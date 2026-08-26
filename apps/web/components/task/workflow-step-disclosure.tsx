"use client";

import { useState } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { IconArrowRight } from "@tabler/icons-react";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";
import { useTranslation } from "react-i18next";

export type WorkflowStepperStep = {
  id: string;
  name: string;
  color: string;
  position: number;
  events?: KanbanStepEvents;
  allow_manual_move?: boolean;
  prompt?: string;
  is_start_step?: boolean;
  agent_profile_id?: string;
};

type Step = WorkflowStepperStep;

type MinimalWorkflowStepperProps = {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
  taskId?: string | null;
  workflowId?: string | null;
  movingToStepId: string | null;
  onMove: (stepId: string) => Promise<boolean>;
};

export function MinimalWorkflowStepper({
  sortedSteps,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
}: MinimalWorkflowStepperProps) {
  const { t } = useTranslation();

  if (isArchived) {
    return (
      <span
        data-testid="workflow-stepper-minimal"
        className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap"
      >
        {t("task:filterDimensionArchived")}
      </span>
    );
  }

  const current = currentIndex >= 0 ? sortedSteps[currentIndex] : sortedSteps[0];
  if (!current) return null;

  if (!taskId || !workflowId) {
    return (
      <MinimalStepIndicator
        current={current}
        currentIndex={currentIndex}
        total={sortedSteps.length}
      />
    );
  }

  return (
    <CompactWorkflowStepDisclosure
      sortedSteps={sortedSteps}
      current={current}
      currentIndex={currentIndex}
      isArchived={isArchived}
      taskId={taskId}
      workflowId={workflowId}
      movingToStepId={movingToStepId}
      onMove={onMove}
    />
  );
}

function CompactWorkflowStepDisclosure({
  sortedSteps,
  current,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
}: {
  sortedSteps: Step[];
  current: Step;
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  onMove: (stepId: string) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const currentDisplayIndex = currentIndex >= 0 ? currentIndex : 0;
  const trigger = (
    <button
      type="button"
      data-testid="workflow-stepper-minimal"
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={t("task:stepOf", {
        stepNumber: currentDisplayIndex + 1,
        totalSteps: sortedSteps.length,
        stepLabel: current.name,
      })}
      className="flex min-w-0 cursor-pointer items-center gap-1.5 rounded-md px-2 py-0.5 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
    >
      <MinimalStepContents
        current={current}
        currentIndex={currentIndex}
        total={sortedSteps.length}
      />
    </button>
  );
  const handleDisclosureMove = async (stepId: string) => {
    const moved = await onMove(stepId);
    if (moved) setOpen(false);
    return moved;
  };
  const content = (
    <StepDisclosureBody
      sortedSteps={sortedSteps}
      currentIndex={currentIndex}
      isArchived={isArchived}
      taskId={taskId}
      workflowId={workflowId}
      movingToStepId={movingToStepId}
      onMove={handleDisclosureMove}
    />
  );

  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent className="max-h-[80dvh]">
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{t("task:moveTo")}</DrawerTitle>
            <DrawerDescription>
              {t("task:stepCount", { count: sortedSteps.length })}
            </DrawerDescription>
          </DrawerHeader>
          {content}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <HoverCard open={open} onOpenChange={setOpen} openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>{trigger}</HoverCardTrigger>
      <HoverCardContent side="bottom" align="center" className="w-72 max-w-[calc(100vw-1rem)] p-2">
        {content}
      </HoverCardContent>
    </HoverCard>
  );
}

function MinimalStepIndicator({
  current,
  currentIndex,
  total,
}: {
  current: Step;
  currentIndex: number;
  total: number;
}) {
  return (
    <div
      data-testid="workflow-stepper-minimal"
      className="flex min-w-0 items-center gap-1.5 rounded-md px-2 py-0.5"
    >
      <MinimalStepContents current={current} currentIndex={currentIndex} total={total} />
    </div>
  );
}

function MinimalStepContents({
  current,
  currentIndex,
  total,
}: {
  current: Step;
  currentIndex: number;
  total: number;
}) {
  const displayIndex = currentIndex >= 0 ? currentIndex : 0;
  return (
    <>
      <div
        data-testid={`workflow-step-${current.name}`}
        aria-current={currentIndex >= 0 ? "step" : undefined}
        className="flex min-w-0 items-center gap-1.5 text-xs"
      >
        <StepCircleIndicator isCurrent isCompleted={false} />
        <span className="truncate text-xs font-medium leading-none text-foreground">
          {current.name}
        </span>
      </div>
      {total > 1 && (
        <span className="shrink-0 text-[11px] tabular-nums leading-none text-muted-foreground">
          {displayIndex + 1}/{total}
        </span>
      )}
    </>
  );
}

function StepDisclosureBody({
  sortedSteps,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
}: {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  onMove: (stepId: string) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="workflow-step-disclosure"
      className="min-h-0 max-h-[70dvh] overflow-y-auto px-2 pb-[calc(1rem+env(safe-area-inset-bottom))]"
    >
      {sortedSteps.map((step, index) => {
        const isCurrent = index === currentIndex;
        const isCompleted = !isArchived && currentIndex >= 0 && index < currentIndex;
        const isAdjacent =
          currentIndex >= 0 && (index === currentIndex - 1 || index === currentIndex + 1);
        const canMove = canMoveToStep({
          isArchived,
          isCurrent,
          taskId,
          workflowId,
          isAdjacent,
          allowManualMove: step.allow_manual_move,
        });
        return (
          <div
            key={step.id}
            data-testid={`workflow-step-disclosure-row-${step.id}`}
            aria-current={isCurrent ? "step" : undefined}
            className="flex min-h-11 items-center gap-2 rounded-md px-2 py-1.5"
          >
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <StepCircleIndicator isCurrent={isCurrent} isCompleted={isCompleted} />
              <span
                className={cn(
                  "min-w-0 truncate text-xs",
                  getStepLabelClass(isCurrent, isCompleted),
                )}
              >
                {step.name}
              </span>
              <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
            </div>
            {isCurrent ? (
              <span className="shrink-0 text-[11px] text-muted-foreground">
                {t("task:currentStep")}
              </span>
            ) : (
              canMove && (
                <Button
                  type="button"
                  data-testid={`workflow-step-disclosure-move-${step.id}`}
                  size="sm"
                  variant="default"
                  className="h-11 shrink-0 cursor-pointer rounded-sm px-2.5 text-xs"
                  disabled={movingToStepId !== null}
                  onClick={() => void onMove(step.id)}
                >
                  <IconArrowRight className="h-3 w-3" />
                  {movingToStepId === step.id ? t("task:moving") : t("task:moveHere")}
                </Button>
              )
            )}
          </div>
        );
      })}
    </div>
  );
}

export function canMoveToStep(params: {
  isArchived: boolean | undefined;
  isCurrent: boolean;
  taskId: string | null | undefined;
  workflowId: string | null | undefined;
  isAdjacent: boolean;
  allowManualMove: boolean | undefined;
}): boolean {
  if (params.isArchived || params.isCurrent || !params.taskId || !params.workflowId) return false;
  return params.isAdjacent || !!params.allowManualMove;
}

export function StepCircleIndicator({
  isCurrent,
  isCompleted,
}: {
  isCurrent: boolean;
  isCompleted: boolean;
}) {
  if (isCurrent) {
    return (
      <span className="relative flex items-center justify-center shrink-0">
        <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
        <span className="h-2 w-2 rounded-full bg-primary" />
      </span>
    );
  }
  if (isCompleted) {
    return (
      <span className="relative flex items-center justify-center shrink-0">
        <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
      </span>
    );
  }
  return (
    <span className="relative flex items-center justify-center shrink-0">
      <span className="h-2 w-2 rounded-full border border-muted-foreground/40" />
    </span>
  );
}

export function getStepLabelClass(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return "text-foreground font-medium";
  if (isCompleted) return "text-muted-foreground";
  return "text-muted-foreground/60";
}
