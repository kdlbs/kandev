"use client";

import { useCallback, useRef, useState } from "react";
import type { WorkflowStepperStep } from "@/components/task/workflow-step-disclosure";
import { useWorkflowStepsById } from "./use-workflow-steps-by-id";
import { usePresentationToken, useWorkflowStepMove } from "./use-workflow-step-move";

export type PreviewStepMove = {
  workflowSteps: WorkflowStepperStep[];
  currentStepId: string | null;
  taskWorkflowId: string | null;
  isArchived: boolean;
  movingToStepId: string | null;
  handleMove: (stepId: string) => Promise<boolean>;
  moveError: unknown;
  handleDisclosureOpenChange: (open: boolean) => void;
  isDisclosureOpen: () => boolean;
};

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
  selectedTask: {
    id: string;
    workflowId?: string | null;
    workflowStepId: string | null;
    isArchived?: boolean;
  } | null,
): PreviewStepMove {
  const workflowSteps = useWorkflowStepsById(selectedTask?.workflowId ?? null);
  const [moveError, setMoveError] = useState<unknown>(null);
  const presentationToken = usePresentationToken(selectedTaskId ?? null);
  const disclosureOpenRef = useRef(false);

  // Clear a stale error the moment a new presentation begins, in the same
  // render pass that changes `presentationToken` — not in a passive effect,
  // whose flush can lag behind a `moveTask` rejection that resolves before it
  // runs. See the same "adjust state during render" pattern below.
  const moveErrorTokenRef = useRef(presentationToken);
  if (moveErrorTokenRef.current !== presentationToken) {
    moveErrorTokenRef.current = presentationToken;
    setMoveError(null);
  }

  const handleMoveStart = useCallback(() => setMoveError(null), []);
  const handleMoveError = useCallback((error: unknown) => setMoveError(error), []);

  const { movingToStepId, handleMove } = useWorkflowStepMove({
    taskId: selectedTask?.id ?? null,
    workflowId: selectedTask?.workflowId ?? null,
    presentationToken,
    onMoveStart: handleMoveStart,
    onMoveError: handleMoveError,
  });

  const handleDisclosureOpenChange = useCallback((open: boolean) => {
    disclosureOpenRef.current = open;
  }, []);
  const isDisclosureOpen = useCallback(() => disclosureOpenRef.current, []);

  return {
    workflowSteps,
    currentStepId: selectedTask?.workflowStepId ?? null,
    taskWorkflowId: selectedTask?.workflowId ?? null,
    isArchived: selectedTask?.isArchived ?? false,
    movingToStepId,
    handleMove,
    moveError,
    handleDisclosureOpenChange,
    isDisclosureOpen,
  };
}
