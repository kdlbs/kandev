"use client";

import type { WorkflowChangePayload, WorkflowMoveEntryOptions } from "@/lib/api";
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
  workflowChange,
  isTouchSurface,
}: {
  target: WorkflowMovePreviewTarget;
  entryOptions?: WorkflowMoveEntryOptions;
  workflowChange?: WorkflowChangePayload;
  isTouchSurface: boolean;
}) {
  const invalidationKey = useWorkflowMovePreviewRevision(
    target.taskId,
    target.workflowId,
    target.workflowStepId,
  );
  const state = useWorkflowMovePreview({
    ...target,
    entryOptions,
    workflowChange,
    invalidationKey,
  });
  return <WorkflowMovePreviewDisclosure state={state} isTouchSurface={isTouchSurface} />;
}
