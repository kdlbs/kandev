"use client";

import { useEffect, useRef } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { Graph2Connector } from "./graph2-connector";
import { Graph2StepNode, Graph2UnassignedStepMarker } from "./graph2-step-node";
import { isOrphanMoveTarget } from "./swimlane-kanban-content";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

type ConnectorType = "past" | "transition" | "future";

function getStepPhase(index: number, currentStepIndex: number): "past" | "current" | "future" {
  if (index < currentStepIndex) return "past";
  if (index === currentStepIndex) return "current";
  return "future";
}

function getConnectorType(
  phase: "past" | "current" | "future",
  nextPhase: "past" | "current" | "future",
): ConnectorType {
  if (phase === "past" && nextPhase === "past") return "past";
  if (phase === "future" && nextPhase === "future") return "future";
  return "transition";
}

export type StepAdjacency = {
  hasPrev: boolean;
  prevStepId?: string;
  hasNext: boolean;
  nextStepId?: string;
};

export type StepMoveTargets = StepAdjacency & {
  prevStepTitle?: string;
  nextStepTitle?: string;
};

/**
 * Computes the prev/next move targets for the node at `index`. The synthetic
 * "Needs Reassignment" node is display-only: it marks where an orphaned task
 * currently sits, but is never itself a valid move destination (there is no
 * backing workflow step to move into), so it is excluded as an adjacency
 * target in either direction.
 */
export function getStepAdjacency(steps: WorkflowStep[], index: number): StepAdjacency {
  const hasPrev = index > 0 && !isOrphanMoveTarget(steps[index - 1].id);
  const hasNext = index < steps.length - 1 && !isOrphanMoveTarget(steps[index + 1].id);
  return {
    hasPrev,
    prevStepId: hasPrev ? steps[index - 1].id : undefined,
    hasNext,
    nextStepId: hasNext ? steps[index + 1].id : undefined,
  };
}

export function getStepAdjacencyForStep(
  moveTargetSteps: WorkflowStep[],
  stepId: string,
): StepAdjacency {
  const index = moveTargetSteps.findIndex((step) => step.id === stepId);
  if (index < 0) return { hasPrev: false, hasNext: false };
  return getStepAdjacency(moveTargetSteps, index);
}

/**
 * Excludes the synthetic orphan step from the steps handed to the shared task
 * menu. The step-pill chevrons need the orphan present so the current step's
 * own adjacency can be computed against it, but the shared menu's move-to-step
 * submenu turns every entry it receives into a selectable destination, and the
 * orphan is display-only: it has no backing workflow step to move into.
 */
export function excludeOrphanFromMoveMenu(moveTargetSteps: WorkflowStep[]): WorkflowStep[] {
  return moveTargetSteps.filter((step) => !isOrphanMoveTarget(step.id));
}

export function getStepMoveTargets(
  moveTargetSteps: WorkflowStep[],
  stepId: string,
): StepMoveTargets {
  const adjacency = getStepAdjacencyForStep(moveTargetSteps, stepId);
  const prevStep = moveTargetSteps.find((step) => step.id === adjacency.prevStepId);
  const nextStep = moveTargetSteps.find((step) => step.id === adjacency.nextStepId);
  return {
    ...adjacency,
    prevStepTitle: prevStep?.title,
    nextStepTitle: nextStep?.title,
  };
}

type HorizontalRect = { left: number; right: number };

/**
 * The scrollLeft adjustment that brings `nodeRect` fully within `containerRect`
 * horizontally, without moving it further than necessary. Zero when the node
 * is already within bounds.
 */
const MOVE_CONTROL_OVERHANG_PX = 12;

export function computeAnchorScrollDelta(
  nodeRect: HorizontalRect,
  containerRect: HorizontalRect,
): number {
  if (nodeRect.left < containerRect.left) return nodeRect.left - containerRect.left;
  if (nodeRect.right > containerRect.right) return nodeRect.right - containerRect.right;
  return 0;
}

export function PipelineStepNodes({
  steps,
  moveTargetSteps,
  currentStepIndex,
  task,
  onMoveTask,
  isMoving,
  atTerminus,
}: {
  steps: WorkflowStep[];
  moveTargetSteps: WorkflowStep[];
  currentStepIndex: number;
  task: Task;
  onMoveTask: (task: Task, targetStepId: string) => void;
  isMoving?: boolean;
  atTerminus: boolean;
}) {
  // A task whose `workflowStepId` matches no displayed step
  // (currentStepIndex === -1) gets one synthetic labelled marker before the
  // run, so the row keeps its single-label invariant. An empty `steps` list
  // is a distinct case and renders no run at all.
  const showUnassignedMarker = currentStepIndex === -1 && steps.length > 0;
  const currentNodeRef = useRef<HTMLDivElement>(null);

  // Every step keeps its own labelled pill (no dot-collapsing), so a
  // many-step run routinely overflows its lane. When it does, the current
  // step must stay the anchor: scroll it into the lane's visible bounds so
  // past steps are what falls off-screen, never the step the task is
  // actually on. This walks scrollLeft on the lane's own scroll container
  // directly rather than calling scrollIntoView, which also nudges every
  // vertically scrollable ancestor (the task list, the page) toward whichever
  // row's effect happens to run last.
  useEffect(() => {
    const node = currentNodeRef.current;
    const scrollRoot = node?.closest<HTMLElement>('[data-testid="pipeline-row-overflow-region"]');
    if (!node || !scrollRoot) return;
    let container: HTMLElement | null = node.parentElement;
    while (container && container !== scrollRoot.parentElement) {
      if (container.scrollWidth > container.clientWidth) break;
      container = container.parentElement;
    }
    if (!container) return;
    // MoveButton (graph2-step-node.tsx) overhangs its step pill by
    // `-left-3`/`-right-3` (12px); a bounding rect never includes an
    // absolutely-positioned descendant's overflow, so anchoring against the
    // pill's own rect can still leave the move control clipped past the edge.
    const rawRect = node.getBoundingClientRect();
    const nodeRect = {
      left: rawRect.left - MOVE_CONTROL_OVERHANG_PX,
      right: rawRect.right + MOVE_CONTROL_OVERHANG_PX,
    };
    const delta = computeAnchorScrollDelta(nodeRect, container.getBoundingClientRect());
    if (delta !== 0) container.scrollLeft += delta;
  }, [task.id, currentStepIndex, steps, atTerminus]);

  return (
    <div
      className={cn(
        "flex items-center gap-0",
        atTerminus ? "shrink-0" : "min-w-0 flex-1 overflow-x-auto scrollbar-hide",
      )}
      data-testid="pipeline-step-run-scroll"
    >
      {showUnassignedMarker && (
        <div className="flex items-center">
          <Graph2UnassignedStepMarker />
          <Graph2Connector type="future" />
        </div>
      )}
      {steps.map((step, index) => {
        const phase = getStepPhase(index, currentStepIndex);
        const hasConnector = index < steps.length - 1;
        const connectorType = hasConnector
          ? getConnectorType(phase, getStepPhase(index + 1, currentStepIndex))
          : null;

        const moveTargets = getStepMoveTargets(moveTargetSteps, step.id);

        return (
          <div
            key={step.id}
            ref={phase === "current" ? currentNodeRef : undefined}
            className="flex items-center"
          >
            <Graph2StepNode
              step={step}
              phase={phase}
              task={task}
              hasPrev={moveTargets.hasPrev}
              hasNext={moveTargets.hasNext}
              prevStepId={moveTargets.prevStepId}
              nextStepId={moveTargets.nextStepId}
              prevStepTitle={moveTargets.prevStepTitle}
              nextStepTitle={moveTargets.nextStepTitle}
              onMoveTask={onMoveTask}
              isMoving={isMoving}
            />

            {connectorType && <Graph2Connector type={connectorType} />}
          </div>
        );
      })}
    </div>
  );
}
