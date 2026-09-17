"use client";

import { useWorkflowMoveSubmit, workflowMoveShortcutLabel } from "./use-workflow-move-submit";

import { memo, useMemo, useRef, useState } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { IconAdjustments, IconArrowRight } from "@tabler/icons-react";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import {
  WorkflowMoveOptionsFields,
  useWorkflowMoveOptionsForm,
  workflowMoveOptionsPayload,
} from "./workflow-move-options";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import {
  usePresentationToken,
  useWorkflowStepMove,
} from "@/hooks/domains/kanban/use-workflow-step-move";
import {
  useWorkflowStepProgress,
  type WorkflowStepProgress,
} from "@/hooks/domains/kanban/use-workflow-step-progress";
import { sortWorkflowStepsByPosition } from "@/lib/kanban/workflow-step-order";
import { useTranslation } from "react-i18next";
import {
  MinimalWorkflowStepper,
  canMoveToStep,
  getStepLabelClass,
  type WorkflowStepperStep,
} from "./workflow-step-disclosure";
import {
  StepProgressDetails,
  workflowStepProgressTranslationKey,
} from "./workflow-step-progress-details";
import { StepCircleIndicator } from "./workflow-step-marker";
import { useHoverPopover } from "@/components/integrations/use-hover-popover";

type Step = WorkflowStepperStep;

type WorkflowStepperProps = {
  steps: Step[];
  currentStepId: string | null;
  taskId?: string | null;
  workflowId?: string | null;
  taskState?: string | null;
  isArchived?: boolean;
  onMoveStart?: () => void;
  onMoveError?: (error: unknown) => void;
};

const WorkflowStepper = memo(function WorkflowStepper({
  steps,
  currentStepId,
  taskId,
  workflowId,
  taskState,
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
  const { movingToStepId, progressingToStepId, handleMove } = useWorkflowStepMove({
    taskId,
    workflowId,
    currentStepId,
    taskState,
    presentationToken,
    onMoveStart,
    onMoveError,
  });
  const { progressByStepId, agentLabelsByProfileId } = useWorkflowStepProgress({
    taskId,
    currentStepId,
    movingToStepId: progressingToStepId ?? movingToStepId,
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
          progressByStepId={progressByStepId}
          agentLabelsByProfileId={agentLabelsByProfileId}
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
                progress={progressByStepId[step.id]}
                pendingLabel={
                  progressByStepId[step.id]?.isPending
                    ? t(workflowStepProgressTranslationKey(progressByStepId[step.id].status))
                    : undefined
                }
                agentLabelsByProfileId={agentLabelsByProfileId}
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
  progress,
  pendingLabel,
  agentLabelsByProfileId,
  onMove,
}: {
  step: Step;
  index: number;
  currentIndex: number;
  isArchived?: boolean;
  taskId?: string | null;
  workflowId?: string | null;
  movingToStepId: string | null;
  progress?: WorkflowStepProgress;
  pendingLabel?: string;
  agentLabelsByProfileId: Readonly<Record<string, string>>;
  onMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
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
  const hover = useHoverPopover({ openDelayMs: 200, closeDelayMs: 100 });

  return (
    <div className="flex items-center">
      {index > 0 && (
        <StepConnector
          isActive={isCompleted || isCurrent}
          testId={`workflow-step-connector-${step.id}`}
        />
      )}
      <Popover open={hover.open} onOpenChange={hover.onOpenChange}>
        <PopoverTrigger asChild>
          <button
            type="button"
            data-testid={`workflow-step-${step.name}`}
            aria-current={isCurrent ? "step" : undefined}
            className={cn(
              "m-0 flex items-center gap-1.5 rounded-md border-0 bg-transparent p-0 px-2 py-0.5 text-left text-xs whitespace-nowrap transition-colors cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
              isCurrent ? "bg-muted/40" : "hover:bg-muted/30",
            )}
            onMouseEnter={hover.onTriggerEnter}
            onMouseMove={hover.onTriggerEnter}
            onPointerEnter={hover.onTriggerEnter}
            onPointerMove={hover.onTriggerEnter}
            onFocus={hover.onTriggerEnter}
            onMouseLeave={hover.onTriggerLeave}
            onPointerLeave={hover.onTriggerLeave}
            onBlur={hover.onTriggerLeave}
          >
            <StepCircleIndicator
              isCurrent={isCurrent}
              isCompleted={isCompleted}
              isPending={progress?.isPending}
              pendingLabel={pendingLabel}
            />
            <span className={cn("text-xs leading-none", getStepLabelClass(isCurrent, isCompleted))}>
              {step.name}
            </span>
          </button>
        </PopoverTrigger>
        <StepHoverContent
          step={step}
          isCurrent={isCurrent}
          canMove={canMove}
          isMoving={movingToStepId === step.id}
          progress={progress}
          agentLabelsByProfileId={agentLabelsByProfileId}
          onMove={onMove}
          hover={hover}
        />
      </Popover>
    </div>
  );
}

/** Connector line between steps */
function StepConnector({ isActive, testId }: { isActive: boolean; testId?: string }) {
  return (
    <div
      data-testid={testId}
      className={cn("h-px w-6 shrink-0", isActive ? "bg-muted-foreground/40" : "bg-border")}
    />
  );
}

/** Hover content for a workflow step */
function StepHoverContent({
  step,
  isCurrent,
  canMove,
  isMoving,
  progress,
  agentLabelsByProfileId,
  onMove,
  hover,
}: {
  step: Step;
  isCurrent: boolean;
  canMove: boolean;
  isMoving: boolean;
  progress?: WorkflowStepProgress;
  agentLabelsByProfileId: Readonly<Record<string, string>>;
  onMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
  hover: ReturnType<typeof useHoverPopover>;
}) {
  const { t } = useTranslation();
  return (
    <PopoverContent
      side="bottom"
      align="center"
      data-testid="workflow-step-popover"
      className="p-1.5 flex flex-col gap-1.5 items-center w-auto min-w-28 max-w-[calc(100vw-1rem)]"
      onMouseEnter={hover.onContentEnter}
      onMouseMove={hover.onContentEnter}
      onPointerEnter={hover.onContentEnter}
      onPointerMove={hover.onContentEnter}
      onMouseLeave={hover.onContentLeave}
      onPointerLeave={hover.onContentLeave}
      onFocusCapture={hover.onContentEnter}
      onBlurCapture={hover.onContentLeave}
      onOpenAutoFocus={(event) => event.preventDefault()}
    >
      {canMove && <StepMoveControls step={step} isMoving={isMoving} onMove={onMove} />}
      {isCurrent && (
        <div className="text-[11px] text-muted-foreground">{t("task:currentStep")}</div>
      )}
      {progress && (
        <StepProgressDetails
          progress={progress}
          agentProfileId={step.agent_profile_id}
          agentLabelsByProfileId={agentLabelsByProfileId}
          testId={`workflow-step-progress-${step.id}`}
        />
      )}
      <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
    </PopoverContent>
  );
}

/**
 * Move button plus an opt-in inline options draft. The options stay hidden
 * until the user reveals them, so a quick hover-and-click keeps the original
 * zero-config move; a revealed, filled draft rides along as one-shot
 * entry_options. Rendered only while the hover card is open, so the draft
 * hook subscribes lazily and resets whenever the pointer leaves the step.
 */
function StepMoveControls({
  step,
  isMoving,
  onMove,
}: {
  step: Step;
  isMoving: boolean;
  onMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [showOptions, setShowOptions] = useState(false);
  const { draft, patchDraft } = useWorkflowMoveOptionsForm();
  const submission = useWorkflowMoveSubmit(isMoving, () =>
    onMove(step.id, workflowMoveOptionsPayload(draft)),
  );

  return (
    <div className="flex w-full flex-col items-stretch gap-1.5">
      <Button
        size="sm"
        variant="default"
        className="cursor-pointer text-xs h-6 px-2.5 rounded-sm"
        disabled={submission.busy}
        onClick={() => void submission.submit()}
        data-testid="workflow-step-move-here"
      >
        <IconArrowRight className="h-3 w-3" />
        {isMoving ? t("task:moving") : t("task:moveHere")}
        {showOptions && (
          <kbd className="self-center font-sans text-[10px] leading-none opacity-60">
            {workflowMoveShortcutLabel()}
          </kbd>
        )}
      </Button>
      {showOptions ? (
        <div
          className="w-64 max-w-[calc(100vw-2rem)]"
          onKeyDown={(event) => {
            submission.onKeyDown(event);
            event.stopPropagation();
          }}
        >
          <WorkflowMoveOptionsFields
            draft={draft}
            onDraftChange={patchDraft}
            isTouchSurface={false}
            instructionsRows={3}
          />
        </div>
      ) : (
        <Button
          size="sm"
          variant="ghost"
          className="cursor-pointer text-xs h-6 px-2.5 rounded-sm text-muted-foreground"
          onClick={() => setShowOptions(true)}
          data-testid="workflow-step-move-options-trigger"
        >
          <IconAdjustments className="h-3 w-3" />
          {t("task:workflowMoveOptions")}
        </Button>
      )}
    </div>
  );
}

export { WorkflowStepper };
export type { WorkflowStepperStep } from "./workflow-step-disclosure";
