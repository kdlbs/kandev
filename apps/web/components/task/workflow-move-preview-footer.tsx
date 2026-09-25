"use client";

import type { WorkflowMoveEntryOptions } from "@/lib/api";
import { useWorkflowMovePreview } from "@/hooks/domains/kanban/use-workflow-move-preview";
import { useWorkflowMovePreviewRevision } from "@/hooks/domains/kanban/use-workflow-move-preview-revision";
import { WorkflowMovePreviewDisclosure } from "./workflow-move-preview";

export type WorkflowMovePreviewTarget = {
  taskId: string;
  workflowId: string;
  workflowStepId: string;
};

export function WorkflowMovePreviewFooter({
  target,
  entryOptions,
  isTouchSurface,
}: {
  target: WorkflowMovePreviewTarget;
  entryOptions?: WorkflowMoveEntryOptions;
  isTouchSurface: boolean;
}) {
  const invalidationKey = useWorkflowMovePreviewRevision(
    target.taskId,
    target.workflowId,
    target.workflowStepId,
  );
  const state = useWorkflowMovePreview({ ...target, entryOptions, invalidationKey });
  return <WorkflowMovePreviewDisclosure state={state} isTouchSurface={isTouchSurface} />;
}
