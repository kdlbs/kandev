"use client";

import { memo, useCallback, useMemo, useRef, useState } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Button } from "@kandev/ui/button";
import { IconArrowRight } from "@tabler/icons-react";
import { moveTask } from "@/lib/api";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useAppStore } from "@/components/state-provider";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";

type Step = {
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

const PLAN_CONTEXT_PATH = "plan:context";

/** Returns a callback that disables plan mode for the active session of a task. */
function useDisablePlanMode() {
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const planModeEnabled = useAppStore((s) =>
    activeSessionId ? (s.chatInput.planModeBySessionId[activeSessionId] ?? false) : false,
  );
  const setPlanMode = useAppStore((s) => s.setPlanMode);
  const setActiveDocument = useAppStore((s) => s.setActiveDocument);
  const closeDocument = useLayoutStore((s) => s.closeDocument);
  const removeContextFile = useContextFilesStore((s) => s.removeFile);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);

  return useCallback(() => {
    if (!activeSessionId || !planModeEnabled) return;
    applyBuiltInPreset("default");
    closeDocument(activeSessionId);
    setActiveDocument(activeSessionId, null);
    setPlanMode(activeSessionId, false);
    removeContextFile(activeSessionId, PLAN_CONTEXT_PATH);
  }, [
    activeSessionId,
    planModeEnabled,
    setPlanMode,
    setActiveDocument,
    closeDocument,
    removeContextFile,
    applyBuiltInPreset,
  ]);
}

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
  const disablePlanMode = useDisablePlanMode();

  const sortedSteps = useMemo(() => [...steps].sort((a, b) => a.position - b.position), [steps]);

  const currentIndex = useMemo(
    () => sortedSteps.findIndex((s) => s.id === currentStepId),
    [sortedSteps, currentStepId],
  );

  const handleMove = useCallback(
    async (stepId: string) => {
      if (!taskId || !workflowId) return;
      disablePlanMode();
      setMovingToStepId(stepId);
      try {
        await moveTask(taskId, {
          workflow_id: workflowId,
          workflow_step_id: stepId,
          position: 0,
        });
      } catch (err) {
        console.error("[WorkflowStepper] Failed to move task:", err);
      } finally {
        setMovingToStepId(null);
      }
    },
    [taskId, workflowId, disablePlanMode],
  );

  // When the full stepper outgrows the available horizontal space (small
  // windows), collapse it to a minimal "current step only" view so it stops
  // overlapping the top-bar action buttons. The container fills the grid track
  // (`w-full`), so its width is content-independent and the collapse decision
  // stays stable across the full/minimal swap.
  const containerRef = useRef<HTMLDivElement>(null);
  const isCollapsed = useToolbarCollapsed(containerRef);

  if (sortedSteps.length === 0) return null;

  return (
    <div
      ref={containerRef}
      className="flex w-full min-w-0 items-center justify-center gap-0 overflow-hidden mx-2"
    >
      {isCollapsed ? (
        <MinimalWorkflowStepper
          sortedSteps={sortedSteps}
          currentIndex={currentIndex}
          isArchived={isArchived}
        />
      ) : (
        <>
          <div data-testid="workflow-stepper" className="flex items-center gap-0">
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
                Archived
              </span>
            </>
          )}
        </>
      )}
    </div>
  );
});

/** Minimal stepper shown when there isn't room for the full version: renders
 *  only the current step (or the archived badge). Keeps the same
 *  `workflow-step-<name>` test id + `aria-current` on the active step so it
 *  stays selectable regardless of which variant is on screen. */
function MinimalWorkflowStepper({
  sortedSteps,
  currentIndex,
  isArchived,
}: {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
}) {
  if (isArchived) {
    return (
      <span className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap">
        Archived
      </span>
    );
  }

  const current = currentIndex >= 0 ? sortedSteps[currentIndex] : sortedSteps[0];
  if (!current) return null;

  return (
    <div
      data-testid="workflow-stepper-minimal"
      className="flex min-w-0 items-center gap-1.5 rounded-md px-2 py-0.5"
    >
      <div
        data-testid={`workflow-step-${current.name}`}
        aria-current="step"
        className="flex min-w-0 items-center gap-1.5 text-xs"
      >
        <StepCircleIndicator isCurrent isCompleted={false} />
        <span className="truncate text-xs font-medium leading-none text-foreground">
          {current.name}
        </span>
      </div>
      {sortedSteps.length > 1 && (
        <span className="shrink-0 text-[11px] tabular-nums leading-none text-muted-foreground">
          {(currentIndex >= 0 ? currentIndex : 0) + 1}/{sortedSteps.length}
        </span>
      )}
    </div>
  );
}

/** Check if a step can be moved to */
function canMoveToStep(params: {
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
          {isMoving ? "Moving..." : "Move here"}
        </Button>
      )}
      {isCurrent && <div className="text-[11px] text-muted-foreground">Current step</div>}
      <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
    </HoverCardContent>
  );
}

/** Circle indicator for step state */
function StepCircleIndicator({
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

/** Get CSS class for step label based on state */
function getStepLabelClass(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return "text-foreground font-medium";
  if (isCompleted) return "text-muted-foreground";
  return "text-muted-foreground/60";
}

export { WorkflowStepper };
export type { Step as WorkflowStepperStep };
