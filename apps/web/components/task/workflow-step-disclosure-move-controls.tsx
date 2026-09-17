"use client";

import { useState, type ReactNode } from "react";
import { StepDisclosureRowActions } from "./workflow-step-disclosure-actions";
import {
  WorkflowMoveOptionsFields,
  useWorkflowMoveOptionsForm,
  workflowMoveOptionsPayload,
} from "./workflow-move-options";
import { CompactWorkflowMovePreview } from "./workflow-move-preview";
import { useWorkflowMovePreview } from "@/hooks/domains/kanban/use-workflow-move-preview";
import { useWorkflowMovePreviewRevision } from "@/hooks/domains/kanban/use-workflow-move-preview-revision";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

type StepDisclosureMoveControlsProps = {
  heading: ReactNode;
  stepId: string;
  taskId: string;
  workflowId: string;
  isMoving: boolean;
  movePending: boolean;
  isTouchSurface: boolean;
  previewEnabled: boolean;
  onMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
};

export function StepDisclosureMoveControls({
  heading,
  stepId,
  taskId,
  workflowId,
  isMoving,
  movePending,
  isTouchSurface,
  previewEnabled,
  onMove,
}: StepDisclosureMoveControlsProps) {
  const [showOptions, setShowOptions] = useState(false);
  const { draft, patchDraft } = useWorkflowMoveOptionsForm();
  const entryOptions = workflowMoveOptionsPayload(draft);
  const invalidationKey = useWorkflowMovePreviewRevision(taskId, workflowId, stepId);
  const previewState = useWorkflowMovePreview({
    taskId,
    workflowId,
    workflowStepId: stepId,
    entryOptions,
    enabled: previewEnabled,
    invalidationKey,
  });
  const buttonSizeClass = isTouchSurface ? "h-11" : "h-7 [@media(pointer:coarse)]:h-11";

  return (
    <>
      <div className="flex min-w-0 items-center gap-2">
        {heading}
        <StepDisclosureRowActions
          stepId={stepId}
          isMoving={isMoving}
          movePending={movePending}
          showOptions={showOptions}
          buttonSizeClass={buttonSizeClass}
          draft={draft}
          entryOptions={entryOptions}
          onToggleOptions={() => setShowOptions((value) => !value)}
          onMove={onMove}
        />
      </div>
      <CompactWorkflowMovePreview
        state={previewState}
        isTouchSurface={isTouchSurface}
        expanded={showOptions}
      />
      {showOptions && (
        <div
          className="pb-1 pl-4 pr-1"
          onKeyDown={(event) => event.stopPropagation()}
          data-testid={`workflow-step-disclosure-options-panel-${stepId}`}
        >
          <WorkflowMoveOptionsFields
            draft={draft}
            onDraftChange={patchDraft}
            isTouchSurface={isTouchSurface}
            instructionsRows={3}
          />
        </div>
      )}
    </>
  );
}
