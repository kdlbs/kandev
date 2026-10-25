"use client";

import { useCallback, useRef, useState } from "react";
import type {
  DisclosureMove,
  WorkflowStepperStep,
} from "@/components/task/workflow-step-disclosure";
import { useWorkflowStepsById } from "./use-workflow-steps-by-id";
import { usePresentationToken, useWorkflowStepMove } from "./use-workflow-step-move";
import { useWorkflowStepProgress, type WorkflowStepProgress } from "./use-workflow-step-progress";

export type PreviewStepMove = {
  workflowSteps: WorkflowStepperStep[];
  currentStepId: string | null;
  taskWorkflowId: string | null;
  isArchived: boolean;
  movingToStepId: string | null;
  progressByStepId?: Readonly<Record<string, WorkflowStepProgress>>;
  agentLabelsByProfileId?: Readonly<Record<string, string>>;
  handleMove: DisclosureMove;
  moveError: unknown;
  handleDisclosureOpenChange: (open: boolean) => void;
  isDisclosureOpen: () => boolean;
};

type PreviewSelectedTask = {
  id: string;
  workflowId?: string | null;
  workflowStepId: string | null;
  state?: string | null;
  isArchived?: boolean;
};

type NormalizedPreviewTask = {
  id: string | null;
  workflowId: string | null;
  workflowStepId: string | null;
  state: string | null;
  isArchived: boolean;
};

function normalizePreviewTask(task: PreviewSelectedTask | null): NormalizedPreviewTask {
  if (!task) {
    return { id: null, workflowId: null, workflowStepId: null, state: null, isArchived: false };
  }
  return {
    id: task.id,
    workflowId: task.workflowId ?? null,
    workflowStepId: task.workflowStepId,
    state: task.state ?? null,
    isArchived: task.isArchived ?? false,
  };
}

function usePreviewMoveError(presentationToken: number) {
  const [moveError, setMoveError] = useState<unknown>(null);
  const moveErrorTokenRef = useRef(presentationToken);

  // Clear a stale error in the render that changes the presentation identity.
  if (moveErrorTokenRef.current !== presentationToken) {
    moveErrorTokenRef.current = presentationToken;
    setMoveError(null);
  }

  const handleMoveStart = useCallback(() => setMoveError(null), []);
  const handleMoveError = useCallback((error: unknown) => setMoveError(error), []);

  return { moveError, handleMoveStart, handleMoveError };
}

/**
 * Owns the preview header's step indicator: step resolution for the
 * previewed task's own workflow, the move request, and the move-failure
 * banner. Lives here rather than in `TaskPreviewPanel` because the panel
 * unmounts on preview close in both layouts, while a stale response must
 * still be discarded and never rendered against the presentation that
 * replaced it.
 */
export function usePreviewWorkflowStepMove(
  selectedTaskId: string | null | undefined,
  selectedTask: PreviewSelectedTask | null,
): PreviewStepMove {
  const task = normalizePreviewTask(selectedTask);
  const workflowSteps = useWorkflowStepsById(task.workflowId);
  const presentationToken = usePresentationToken(selectedTaskId);
  const disclosureOpenRef = useRef(false);
  const { moveError, handleMoveStart, handleMoveError } = usePreviewMoveError(presentationToken);

  const { movingToStepId, progressingToStepId, handleMove } = useWorkflowStepMove({
    taskId: task.id,
    workflowId: task.workflowId,
    currentStepId: task.workflowStepId,
    taskState: task.state,
    presentationToken,
    onMoveStart: handleMoveStart,
    onMoveError: handleMoveError,
  });
  const { progressByStepId, agentLabelsByProfileId } = useWorkflowStepProgress({
    taskId: task.id,
    currentStepId: task.workflowStepId,
    movingToStepId: progressingToStepId ?? movingToStepId,
  });

  const handleDisclosureOpenChange = useCallback((open: boolean) => {
    disclosureOpenRef.current = open;
  }, []);
  const isDisclosureOpen = useCallback(() => disclosureOpenRef.current, []);

  return {
    workflowSteps,
    currentStepId: task.workflowStepId,
    taskWorkflowId: task.workflowId,
    isArchived: task.isArchived,
    movingToStepId,
    progressByStepId,
    agentLabelsByProfileId,
    handleMove,
    moveError,
    handleDisclosureOpenChange,
    isDisclosureOpen,
  };
}
