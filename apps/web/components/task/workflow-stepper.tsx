"use client";

import { memo, useMemo, useRef } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Button } from "@kandev/ui/button";
import { IconArrowRight } from "@tabler/icons-react";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import {
  usePresentationToken,
  useWorkflowStepMove,
} from "@/hooks/domains/kanban/use-workflow-step-move";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import { useTranslation } from "react-i18next";
import {
  MinimalWorkflowStepper,
  StepCircleIndicator,
  canMoveToStep,
  getStepLabelClass,
  type WorkflowStepperStep,
} from "./workflow-step-disclosure";

type Step = WorkflowStepperStep;

type WorkflowStepperProps = {
  steps: Step[];
  currentStepId: string | null;
  taskId?: string | null;
  workflowId?: string | null;
  isArchived?: boolean;
  onMoveStart?: () => void;
  onMoveError?: (error: unknown) => void;
};

const WorkflowStepper = memo(function WorkflowStepper({
  steps,
  currentStepId,
  taskId,
  workflowId,
  isArchived,
  onMoveStart,
  onMoveError,
}: WorkflowStepperProps) {
  const { t } = useTranslation();
  // The task route's continuous presentation of this task: a navigation away
  // and back changes taskId and back, which must invalidate a request left
  // over from the earlier presentation the same way a preview close-and-reopen
  // does for the kanban preview header.
  const presentationToken = usePresentationToken(taskId ?? null);
  const { movingToStepId, handleMove } = useWorkflowStepMove({
    taskId,
    workflowId,
    presentationToken,
    onMoveStart,
    onMoveError,
  });

  const sortedSteps = useMemo(() => sortWorkflowStepsByPosition(steps), [steps]);

  const currentIndex = useMemo(
    () => sortedSteps.findIndex((s) => s.id === currentStepId),
    [sortedSteps, currentStepId],
  );

  // Collapse to a minimal view when the full stepper can't fit (w-full keeps the measurement track-driven).
  const containerRef = useRef<HTMLDivElement>(null);
  const isCollapsed = useToolbarCollapsed(containerRef);

  if (sortedSteps.length === 0) return null;

  return (
    <div
      ref={containerRef}
      data-testid="workflow-stepper"
      className="flex w-full min-w-0 items-center justify-center gap-0 overflow-hidden"
    >
      {isCollapsed ? (
        <MinimalWorkflowStepper
          sortedSteps={sortedSteps}
          currentIndex={currentIndex}
          isArchived={isArchived}
          taskId={taskId}
          workflowId={workflowId}
          movingToStepId={movingToStepId}
          onMove={handleMove}
        />
      ) : (
        <>
          <div className="flex items-center gap-0">
            {sortedSteps.map((step, index) => (
              <WorkflowStepItem
                key={step.id}
                step={step}
                index={index}
                currentIndex={currentIndex}
                isArchived={isArchived}
                taskId={taskId}
                workflowId={workflowId}
                movingToStepId={movingToStepId}
                onMove={handleMove}
              />
            ))}
          </div>
          {isArchived && (
            <>
              <div className="h-px w-6 shrink-0 bg-border" />
              <span className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap">
                {t("task:filterDimensionArchived")}
              </span>
            </>
          )}
        </>
      )}
    </div>
  );
});

/** Individual step in the workflow stepper */
function WorkflowStepItem({
  step,
  index,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
}: {
  step: Step;
  index: number;
  currentIndex: number;
  isArchived?: boolean;
  taskId?: string | null;
  workflowId?: string | null;
  movingToStepId: string | null;
  onMove: (stepId: string) => void;
}) {
  const isCompleted = !isArchived && currentIndex >= 0 && index < currentIndex;
  const isCurrent = !isArchived && index === currentIndex;
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
    <div className="flex items-center">
      {index > 0 && <StepConnector isActive={isCompleted || isCurrent} />}
      <HoverCard openDelay={200} closeDelay={100}>
        <HoverCardTrigger asChild>
          <div
            data-testid={`workflow-step-${step.name}`}
            aria-current={isCurrent ? "step" : undefined}
            className={cn(
              "flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs whitespace-nowrap transition-colors cursor-default",
              isCurrent ? "bg-muted/40" : "hover:bg-muted/30",
            )}
          >
            <StepCircleIndicator isCurrent={isCurrent} isCompleted={isCompleted} />
            <span className={cn("text-xs leading-none", getStepLabelClass(isCurrent, isCompleted))}>
              {step.name}
            </span>
          </div>
        </HoverCardTrigger>
        <StepHoverContent
          step={step}
          isCurrent={isCurrent}
          canMove={canMove}
          isMoving={movingToStepId === step.id}
          onMove={onMove}
        />
      </HoverCard>
    </div>
  );
}

/** Connector line between steps */
function StepConnector({ isActive }: { isActive: boolean }) {
  return (
    <div className={cn("h-px w-6 shrink-0", isActive ? "bg-muted-foreground/40" : "bg-border")} />
  );
}

/** Hover content for a workflow step */
function StepHoverContent({
  step,
  isCurrent,
  canMove,
  isMoving,
  onMove,
}: {
  step: Step;
  isCurrent: boolean;
  canMove: boolean;
  isMoving: boolean;
  onMove: (stepId: string) => void;
}) {
  const { t } = useTranslation();
  return (
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
          disabled={isMoving}
          onClick={() => onMove(step.id)}
        >
          <IconArrowRight className="h-3 w-3" />
          {isMoving ? t("task:moving") : t("task:moveHere")}
        </Button>
      )}
      {isCurrent && (
        <div className="text-[11px] text-muted-foreground">{t("task:currentStep")}</div>
      )}
      <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
    </HoverCardContent>
  );
}

export { WorkflowStepper };
export type { WorkflowStepperStep } from "./workflow-step-disclosure";
