"use client";

import { type RefObject, useCallback, useLayoutEffect, useRef } from "react";
import type { DragStartEvent } from "@dnd-kit/core";
import type { Task } from "@/components/kanban-card";
import type { WorkflowStep } from "@/components/kanban-column";

type DragScrollAnchor = {
  stepId: string;
  viewportLeft: number;
  originalScrollLeft: number;
};

function findStepElement(board: HTMLDivElement, stepId: string): HTMLElement | undefined {
  return Array.from(board.querySelectorAll<HTMLElement>("[data-kanban-step-id]")).find(
    (element) => element.dataset.kanbanStepId === stepId,
  );
}

function findScrollWindow(board: HTMLDivElement): HTMLElement | null {
  return board.querySelector<HTMLElement>(
    '[data-testid="desktop-kanban-scroll-window"], [data-testid="tablet-kanban-layout"]',
  );
}

export function getPreservedScrollLeft(
  currentScrollLeft: number,
  anchorViewportLeft: number,
  currentAnchorLeft: number | undefined,
  originalScrollLeft: number,
): number {
  if (currentAnchorLeft == null) return originalScrollLeft;
  return currentScrollLeft + currentAnchorLeft - anchorViewportLeft;
}

export function getDragDisplaySteps(
  displaySteps: WorkflowStep[],
  moveTargetSteps: WorkflowStep[],
  isDragging: boolean,
  isMobile: boolean,
): WorkflowStep[] {
  if (isMobile || !isDragging) return displaySteps;
  const moveTargetIds = new Set(moveTargetSteps.map((step) => step.id));
  const displayOnlyStep = displaySteps.find((step) => !moveTargetIds.has(step.id));
  return displayOnlyStep ? [...moveTargetSteps, displayOnlyStep] : moveTargetSteps;
}

export function getTemporaryStepIds(
  displaySteps: WorkflowStep[],
  moveTargetSteps: WorkflowStep[],
): Set<string> {
  const visibleStepIds = new Set(displaySteps.map((step) => step.id));
  return new Set(
    moveTargetSteps.filter((step) => !visibleStepIds.has(step.id)).map((step) => step.id),
  );
}

export function useKanbanDragScrollAnchor({
  tasks,
  activeTask,
  renderedSteps,
  onDragStart,
}: {
  tasks: Task[];
  activeTask: Task | null;
  renderedSteps: WorkflowStep[];
  onDragStart: (event: DragStartEvent) => void;
}): {
  boardRef: RefObject<HTMLDivElement | null>;
  handleAnchoredDragStart: (event: DragStartEvent) => void;
} {
  const boardRef = useRef<HTMLDivElement | null>(null);
  const anchorRef = useRef<DragScrollAnchor | null>(null);
  const restoreFrameRef = useRef<number | null>(null);

  const handleAnchoredDragStart = useCallback(
    (event: DragStartEvent) => {
      const task = tasks.find((candidate) => candidate.id === event.active.id);
      const board = boardRef.current;
      const scrollWindow = board ? findScrollWindow(board) : null;
      const stepElement =
        board && task?.workflowStepId ? findStepElement(board, task.workflowStepId) : undefined;
      if (scrollWindow && stepElement && task?.workflowStepId) {
        anchorRef.current = {
          stepId: task.workflowStepId,
          viewportLeft: stepElement.getBoundingClientRect().left,
          originalScrollLeft: scrollWindow.scrollLeft,
        };
      }
      onDragStart(event);
    },
    [tasks, onDragStart],
  );

  useLayoutEffect(() => {
    const anchor = anchorRef.current;
    const board = boardRef.current;
    if (!anchor || !board) return;
    const scrollWindow = findScrollWindow(board);
    if (!scrollWindow) return;
    const stepElement = findStepElement(board, anchor.stepId);
    const desiredScrollLeft = getPreservedScrollLeft(
      scrollWindow.scrollLeft,
      anchor.viewportLeft,
      stepElement?.getBoundingClientRect().left,
      anchor.originalScrollLeft,
    );
    scrollWindow.scrollLeft = desiredScrollLeft;

    if (!activeTask) {
      if (restoreFrameRef.current != null) cancelAnimationFrame(restoreFrameRef.current);
      restoreFrameRef.current = requestAnimationFrame(() => {
        if (boardRef.current) {
          const liveScrollWindow = findScrollWindow(boardRef.current);
          if (liveScrollWindow) liveScrollWindow.scrollLeft = desiredScrollLeft;
        }
        anchorRef.current = null;
      });
    }

    return () => {
      if (restoreFrameRef.current != null) cancelAnimationFrame(restoreFrameRef.current);
    };
  }, [activeTask, renderedSteps]);

  return { boardRef, handleAnchoredDragStart };
}
