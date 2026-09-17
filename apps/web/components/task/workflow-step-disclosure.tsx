"use client";

import { forwardRef, useEffect, type ComponentPropsWithoutRef, type ReactNode } from "react";
import { cn } from "@kandev/ui/lib/utils";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { IconChevronDown } from "@tabler/icons-react";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import {
  useCompactWorkflowDisclosure,
  type CompactWorkflowDisclosureControls,
} from "./workflow-step-disclosure-controls";
import type { WorkflowStepProgress } from "@/hooks/domains/kanban/use-workflow-step-progress";
import {
  StepProgressDetails,
  workflowStepProgressTranslationKey,
} from "./workflow-step-progress-details";
import { StepCircleIndicator } from "./workflow-step-marker";
import { useTranslation } from "react-i18next";
import { StepDisclosureMoveControls } from "./workflow-step-disclosure-move-controls";

/** Move callback shared by every compact-disclosure surface. A revealed,
 * filled options draft rides along as one-shot `entry_options`. */
export type DisclosureMove = (
  stepId: string,
  entryOptions?: WorkflowMoveEntryOptions,
) => Promise<boolean>;

export type WorkflowStepperStep = {
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

type Step = WorkflowStepperStep;

type MinimalWorkflowStepperProps = {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
  taskId?: string | null;
  workflowId?: string | null;
  movingToStepId: string | null;
  onMove: DisclosureMove;
  progressByStepId?: Readonly<Record<string, WorkflowStepProgress>>;
  agentLabelsByProfileId?: Readonly<Record<string, string>>;
  /** Notified whenever the disclosure surface opens or closes. */
  onDisclosureOpenChange?: (open: boolean) => void;
};

const EMPTY_PROGRESS_BY_STEP_ID: Readonly<Record<string, WorkflowStepProgress>> = {};
const EMPTY_AGENT_LABELS_BY_PROFILE_ID: Readonly<Record<string, string>> = {};

export function MinimalWorkflowStepper({
  sortedSteps,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
  progressByStepId = EMPTY_PROGRESS_BY_STEP_ID,
  agentLabelsByProfileId = EMPTY_AGENT_LABELS_BY_PROFILE_ID,
  onDisclosureOpenChange,
}: MinimalWorkflowStepperProps) {
  const { t } = useTranslation();

  if (isArchived) {
    return (
      <span
        data-testid="workflow-stepper-minimal"
        className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap"
      >
        {t("task:filterDimensionArchived")}
      </span>
    );
  }

  const current = currentIndex >= 0 ? sortedSteps[currentIndex] : sortedSteps[0];
  if (!current) return null;

  if (!taskId || !workflowId) {
    return (
      <MinimalStepIndicator
        current={current}
        currentIndex={currentIndex}
        total={sortedSteps.length}
        progress={progressByStepId[current.id]}
      />
    );
  }

  return (
    <CompactWorkflowStepDisclosure
      sortedSteps={sortedSteps}
      current={current}
      currentIndex={currentIndex}
      isArchived={isArchived}
      taskId={taskId}
      workflowId={workflowId}
      movingToStepId={movingToStepId}
      onMove={onMove}
      progressByStepId={progressByStepId}
      agentLabelsByProfileId={agentLabelsByProfileId}
      onDisclosureOpenChange={onDisclosureOpenChange}
    />
  );
}

type CompactWorkflowTriggerProps = ComponentPropsWithoutRef<"button"> & {
  current: Step;
  currentIndex: number;
  total: number;
  progress?: WorkflowStepProgress;
  usesTouchDrawer: boolean;
  controls: CompactWorkflowDisclosureControls;
};

const CompactWorkflowTrigger = forwardRef<HTMLButtonElement, CompactWorkflowTriggerProps>(
  function CompactWorkflowTrigger(
    {
      current,
      currentIndex,
      total,
      progress,
      usesTouchDrawer,
      controls,
      className,
      ...buttonProps
    },
    ref,
  ) {
    const { t } = useTranslation();
    const currentDisplayIndex = currentIndex >= 0 ? currentIndex : 0;
    const progressLabel = progress
      ? t(workflowStepProgressTranslationKey(progress.status))
      : undefined;
    return (
      <button
        {...buttonProps}
        type="button"
        ref={(node) => {
          controls.setTriggerRef(node);
          if (typeof ref === "function") ref(node);
          else if (ref) ref.current = node;
        }}
        data-testid="workflow-stepper-minimal"
        aria-haspopup="dialog"
        aria-expanded={controls.open}
        aria-label={t(progressLabel ? "task:stepOfWithStatus" : "task:stepOf", {
          stepNumber: currentDisplayIndex + 1,
          totalSteps: total,
          stepLabel: current.name,
          status: progressLabel,
        })}
        onMouseEnter={controls.openDisclosure}
        onMouseLeave={controls.scheduleClose}
        onFocus={controls.handleTriggerFocus}
        onBlur={controls.handleTriggerBlur}
        className={cn(
          "flex min-w-0 cursor-pointer items-center gap-1.5 rounded-md px-2 py-0.5 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
          usesTouchDrawer && "min-h-11",
          className,
        )}
      >
        <MinimalStepContents
          current={current}
          currentIndex={currentIndex}
          total={total}
          progress={progress}
        />
        {usesTouchDrawer && (
          <IconChevronDown
            data-testid="workflow-stepper-touch-disclosure-cue"
            aria-hidden="true"
            className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
          />
        )}
      </button>
    );
  },
);

function CompactWorkflowStepDisclosure({
  sortedSteps,
  current,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
  progressByStepId,
  agentLabelsByProfileId,
  onDisclosureOpenChange,
}: {
  sortedSteps: Step[];
  current: Step;
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  onMove: DisclosureMove;
  progressByStepId: Readonly<Record<string, WorkflowStepProgress>>;
  agentLabelsByProfileId: Readonly<Record<string, string>>;
  onDisclosureOpenChange?: (open: boolean) => void;
}) {
  const usesTouchDrawer = useTouchDrawer();
  const controls = useCompactWorkflowDisclosure();
  useEffect(() => {
    onDisclosureOpenChange?.(controls.open);
  }, [controls.open, onDisclosureOpenChange]);
  useEffect(
    () => () => {
      onDisclosureOpenChange?.(false);
    },
    [onDisclosureOpenChange],
  );
  const trigger = (
    <CompactWorkflowTrigger
      current={current}
      currentIndex={currentIndex}
      total={sortedSteps.length}
      progress={progressByStepId[current.id]}
      usesTouchDrawer={usesTouchDrawer}
      controls={controls}
    />
  );
  const handleDisclosureMove: DisclosureMove = async (stepId, entryOptions) => {
    const moved = await onMove(stepId, entryOptions);
    if (moved) controls.setOpen(false);
    return moved;
  };
  const content = (
    <StepDisclosureBody
      sortedSteps={sortedSteps}
      currentIndex={currentIndex}
      isArchived={isArchived}
      taskId={taskId}
      workflowId={workflowId}
      movingToStepId={movingToStepId}
      isTouchSurface={usesTouchDrawer}
      progressByStepId={progressByStepId}
      agentLabelsByProfileId={agentLabelsByProfileId}
      onMove={handleDisclosureMove}
    />
  );

  return (
    <CompactWorkflowDisclosureSurface
      trigger={trigger}
      content={content}
      controls={controls}
      usesTouchDrawer={usesTouchDrawer}
      stepCount={sortedSteps.length}
    />
  );
}

function CompactWorkflowDisclosureSurface({
  trigger,
  content,
  controls,
  usesTouchDrawer,
  stepCount,
}: {
  trigger: ReactNode;
  content: ReactNode;
  controls: CompactWorkflowDisclosureControls;
  usesTouchDrawer: boolean;
  stepCount: number;
}) {
  const { t } = useTranslation();

  if (usesTouchDrawer) {
    return (
      <Drawer open={controls.open} onOpenChange={controls.setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent className="max-h-[80dvh]">
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{t("task:moveTo")}</DrawerTitle>
            <DrawerDescription>{t("task:stepCount", { count: stepCount })}</DrawerDescription>
          </DrawerHeader>
          {content}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Popover open={controls.open} onOpenChange={controls.setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        ref={controls.contentRef}
        role="dialog"
        aria-label={t("task:moveTo")}
        side="bottom"
        align="center"
        className="w-[25rem] max-w-[calc(100vw-1rem)] p-2"
        onOpenAutoFocus={controls.handleOpenAutoFocus}
        onCloseAutoFocus={controls.handleCloseAutoFocus}
        onEscapeKeyDown={(event) => event.stopPropagation()}
        onMouseEnter={controls.openDisclosure}
        onMouseLeave={controls.scheduleClose}
        onFocusCapture={controls.handleContentFocus}
        onBlurCapture={controls.handleContentBlur}
      >
        {content}
      </PopoverContent>
    </Popover>
  );
}

function MinimalStepIndicator({
  current,
  currentIndex,
  total,
  progress,
}: {
  current: Step;
  currentIndex: number;
  total: number;
  progress?: WorkflowStepProgress;
}) {
  return (
    <div
      data-testid="workflow-stepper-minimal"
      className="flex min-w-0 items-center gap-1.5 rounded-md px-2 py-0.5"
    >
      <MinimalStepContents
        current={current}
        currentIndex={currentIndex}
        total={total}
        progress={progress}
      />
    </div>
  );
}

function MinimalStepContents({
  current,
  currentIndex,
  total,
  progress,
}: {
  current: Step;
  currentIndex: number;
  total: number;
  progress?: WorkflowStepProgress;
}) {
  const { t } = useTranslation();
  const displayIndex = currentIndex >= 0 ? currentIndex : 0;
  const pendingLabel = progress?.isPending
    ? t(workflowStepProgressTranslationKey(progress.status))
    : undefined;
  return (
    <>
      <div
        data-testid={`workflow-step-${current.name}`}
        aria-current={currentIndex >= 0 ? "step" : undefined}
        className="flex min-w-0 items-center gap-1.5 text-xs"
      >
        <StepCircleIndicator
          isCurrent={currentIndex >= 0}
          isCompleted={false}
          isPending={progress?.isPending}
          pendingLabel={pendingLabel}
        />
        <span className="min-w-0 truncate text-xs font-medium leading-none text-foreground">
          {current.name}
        </span>
      </div>
      {total > 1 && (
        <span className="shrink-0 text-[11px] tabular-nums leading-none text-muted-foreground">
          {displayIndex + 1}/{total}
        </span>
      )}
    </>
  );
}

function StepDisclosureBody({
  sortedSteps,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  isTouchSurface,
  progressByStepId,
  agentLabelsByProfileId,
  onMove,
}: {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  isTouchSurface: boolean;
  progressByStepId: Readonly<Record<string, WorkflowStepProgress>>;
  agentLabelsByProfileId: Readonly<Record<string, string>>;
  onMove: DisclosureMove;
}) {
  return (
    <div
      data-testid="workflow-step-disclosure"
      className={cn(
        "min-h-0 max-h-[70dvh] space-y-1 overflow-y-auto overscroll-contain",
        isTouchSurface && "px-2 pb-[calc(1rem+env(safe-area-inset-bottom))]",
      )}
    >
      {sortedSteps.map((step, index) => {
        const isCurrent = index === currentIndex;
        const isCompleted = !isArchived && currentIndex >= 0 && index < currentIndex;
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
          <StepDisclosureRow
            key={step.id}
            step={step}
            isCurrent={isCurrent}
            isCompleted={isCompleted}
            canMove={canMove}
            isMoving={movingToStepId === step.id}
            movePending={movingToStepId !== null}
            isTouchSurface={isTouchSurface}
            taskId={taskId}
            workflowId={workflowId}
            previewEnabled={canMove}
            progress={progressByStepId[step.id]}
            agentLabelsByProfileId={agentLabelsByProfileId}
            onMove={onMove}
          />
        );
      })}
    </div>
  );
}

/**
 * A single step choice in the compact disclosure. Non-current, movable steps
 * carry the same opt-in one-time move options as the full stepper: the fields
 * stay hidden until the user reveals them, so a quick tap keeps the zero-config
 * move while a revealed, filled draft rides along as one-shot `entry_options`.
 * A successful move unmounts the row (the disclosure closes); a failed move
 * keeps the disclosure open so the draft the user typed is preserved.
 */
function StepDisclosureRow({
  step,
  isCurrent,
  isCompleted,
  canMove,
  isMoving,
  movePending,
  isTouchSurface,
  taskId,
  workflowId,
  previewEnabled,
  progress,
  agentLabelsByProfileId,
  onMove,
}: {
  step: Step;
  isCurrent: boolean;
  isCompleted: boolean;
  canMove: boolean;
  isMoving: boolean;
  movePending: boolean;
  isTouchSurface: boolean;
  taskId: string;
  workflowId: string;
  previewEnabled: boolean;
  progress?: WorkflowStepProgress;
  agentLabelsByProfileId: Readonly<Record<string, string>>;
  onMove: DisclosureMove;
}) {
  const { t } = useTranslation();
  const heading = (
    <div className="flex min-w-0 flex-1 items-center gap-2">
      <StepCircleIndicator
        isCurrent={isCurrent}
        isCompleted={isCompleted}
        isPending={progress?.isPending}
        pendingLabel={
          progress?.isPending ? t(workflowStepProgressTranslationKey(progress.status)) : undefined
        }
      />
      <span className={cn("min-w-0 truncate text-xs text-foreground", isCurrent && "font-medium")}>
        {step.name}
      </span>
      <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
    </div>
  );

  return (
    <div
      data-testid={`workflow-step-disclosure-row-${step.id}`}
      aria-current={isCurrent ? "step" : undefined}
      className={cn("flex flex-col gap-1 rounded-md px-2 py-2", isCurrent && "bg-primary/10")}
    >
      {canMove && !isCurrent ? (
        <StepDisclosureMoveControls
          heading={heading}
          stepId={step.id}
          taskId={taskId}
          workflowId={workflowId}
          isMoving={isMoving}
          movePending={movePending}
          isTouchSurface={isTouchSurface}
          previewEnabled={previewEnabled}
          onMove={onMove}
        />
      ) : (
        <div className={cn("flex min-h-7 items-center gap-2", isTouchSurface && "min-h-11")}>
          {heading}
          {isCurrent && (
            <span className="shrink-0 text-[11px] text-muted-foreground">
              {t("task:currentStep")}
            </span>
          )}
        </div>
      )}
      {progress && (
        <StepProgressDetails
          progress={progress}
          agentProfileId={step.agent_profile_id}
          agentLabelsByProfileId={agentLabelsByProfileId}
          testId={`workflow-step-progress-${step.id}`}
        />
      )}
    </div>
  );
}

export function canMoveToStep(params: {
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

export function getStepLabelClass(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return "text-foreground font-medium";
  if (isCompleted) return "text-muted-foreground";
  return "text-muted-foreground/60";
}
