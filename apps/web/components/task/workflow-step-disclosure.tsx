"use client";

import { forwardRef, useEffect, useState, type ComponentPropsWithoutRef } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { IconAdjustments, IconArrowRight, IconChevronDown } from "@tabler/icons-react";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import {
  WorkflowMoveOptionsFields,
  useWorkflowMoveOptionsForm,
  workflowMoveOptionsPayload,
} from "./workflow-move-options";
import {
  useCompactWorkflowDisclosure,
  type CompactWorkflowDisclosureControls,
} from "./workflow-step-disclosure-controls";
import { useTranslation } from "react-i18next";

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
  /** Notified whenever the disclosure surface opens or closes. */
  onDisclosureOpenChange?: (open: boolean) => void;
};

export function MinimalWorkflowStepper({
  sortedSteps,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
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
      onDisclosureOpenChange={onDisclosureOpenChange}
    />
  );
}

type CompactWorkflowTriggerProps = ComponentPropsWithoutRef<"button"> & {
  current: Step;
  currentIndex: number;
  total: number;
  usesTouchDrawer: boolean;
  controls: CompactWorkflowDisclosureControls;
};

const CompactWorkflowTrigger = forwardRef<HTMLButtonElement, CompactWorkflowTriggerProps>(
  function CompactWorkflowTrigger(
    { current, currentIndex, total, usesTouchDrawer, controls, className, ...buttonProps },
    ref,
  ) {
    const { t } = useTranslation();
    const currentDisplayIndex = currentIndex >= 0 ? currentIndex : 0;
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
        aria-label={t("task:stepOf", {
          stepNumber: currentDisplayIndex + 1,
          totalSteps: total,
          stepLabel: current.name,
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
        <MinimalStepContents current={current} currentIndex={currentIndex} total={total} />
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
  onDisclosureOpenChange?: (open: boolean) => void;
}) {
  const { t } = useTranslation();
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
      onMove={handleDisclosureMove}
    />
  );

  if (usesTouchDrawer) {
    return (
      <Drawer open={controls.open} onOpenChange={controls.setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent className="max-h-[80dvh]">
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{t("task:moveTo")}</DrawerTitle>
            <DrawerDescription>
              {t("task:stepCount", { count: sortedSteps.length })}
            </DrawerDescription>
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
        className="w-72 max-w-[calc(100vw-1rem)] p-2"
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
}: {
  current: Step;
  currentIndex: number;
  total: number;
}) {
  return (
    <div
      data-testid="workflow-stepper-minimal"
      className="flex min-w-0 items-center gap-1.5 rounded-md px-2 py-0.5"
    >
      <MinimalStepContents current={current} currentIndex={currentIndex} total={total} />
    </div>
  );
}

function MinimalStepContents({
  current,
  currentIndex,
  total,
}: {
  current: Step;
  currentIndex: number;
  total: number;
}) {
  const displayIndex = currentIndex >= 0 ? currentIndex : 0;
  return (
    <>
      <div
        data-testid={`workflow-step-${current.name}`}
        aria-current={currentIndex >= 0 ? "step" : undefined}
        className="flex min-w-0 items-center gap-1.5 text-xs"
      >
        <StepCircleIndicator isCurrent={currentIndex >= 0} isCompleted={false} />
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
  onMove,
}: {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  isTouchSurface: boolean;
  onMove: DisclosureMove;
}) {
  return (
    <div
      data-testid="workflow-step-disclosure"
      className="min-h-0 max-h-[70dvh] overflow-y-auto px-2 pb-[calc(1rem+env(safe-area-inset-bottom))]"
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
  onMove,
}: {
  step: Step;
  isCurrent: boolean;
  isCompleted: boolean;
  canMove: boolean;
  isMoving: boolean;
  movePending: boolean;
  isTouchSurface: boolean;
  onMove: DisclosureMove;
}) {
  const { t } = useTranslation();
  const [showOptions, setShowOptions] = useState(false);
  const { draft, patchDraft } = useWorkflowMoveOptionsForm();
  const buttonSizeClass = isTouchSurface ? "h-11" : "h-7 [@media(pointer:coarse)]:h-11";

  return (
    <div
      data-testid={`workflow-step-disclosure-row-${step.id}`}
      aria-current={isCurrent ? "step" : undefined}
      className="flex flex-col gap-1.5 rounded-md px-2 py-1.5"
    >
      <div className="flex min-h-11 items-center gap-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <StepCircleIndicator isCurrent={isCurrent} isCompleted={isCompleted} />
          <span
            className={cn("min-w-0 truncate text-xs", getStepLabelClass(isCurrent, isCompleted))}
          >
            {step.name}
          </span>
          <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
        </div>
        {isCurrent ? (
          <span className="shrink-0 text-[11px] text-muted-foreground">
            {t("task:currentStep")}
          </span>
        ) : (
          canMove && (
            <div className="flex shrink-0 items-center gap-1">
              <Button
                type="button"
                data-testid={`workflow-step-disclosure-options-${step.id}`}
                size="sm"
                variant="ghost"
                aria-expanded={showOptions}
                aria-label={t("task:workflowMoveOptions")}
                className={cn(
                  "shrink-0 cursor-pointer rounded-sm px-2 text-muted-foreground",
                  buttonSizeClass,
                  showOptions && "bg-muted/60 text-foreground",
                )}
                onClick={() => setShowOptions((value) => !value)}
              >
                <IconAdjustments className="h-3.5 w-3.5" />
              </Button>
              <Button
                type="button"
                data-testid={`workflow-step-disclosure-move-${step.id}`}
                size="sm"
                variant="default"
                className={cn("shrink-0 cursor-pointer rounded-sm px-2.5 text-xs", buttonSizeClass)}
                disabled={movePending}
                onClick={() => void onMove(step.id, workflowMoveOptionsPayload(draft))}
              >
                <IconArrowRight className="h-3 w-3" />
                {isMoving ? t("task:moving") : t("task:moveHere")}
              </Button>
            </div>
          )
        )}
      </div>
      {canMove && !isCurrent && showOptions && (
        <div
          className="pb-1 pl-4 pr-1"
          onKeyDown={(event) => event.stopPropagation()}
          data-testid={`workflow-step-disclosure-options-panel-${step.id}`}
        >
          <WorkflowMoveOptionsFields
            draft={draft}
            onDraftChange={patchDraft}
            isTouchSurface={isTouchSurface}
            instructionsRows={3}
          />
        </div>
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

export type StepMarkerState = "current" | "completed" | "upcoming";

export function StepCircleIndicator({
  isCurrent,
  isCompleted,
}: {
  isCurrent: boolean;
  isCompleted: boolean;
}) {
  let state: StepMarkerState = "upcoming";
  if (isCurrent) state = "current";
  else if (isCompleted) state = "completed";
  if (isCurrent) {
    return (
      <span
        data-marker-state={state}
        className="relative flex items-center justify-center shrink-0"
      >
        <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
        <span className="h-2 w-2 rounded-full bg-primary" />
      </span>
    );
  }
  if (isCompleted) {
    return (
      <span
        data-marker-state={state}
        className="relative flex items-center justify-center shrink-0"
      >
        <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
      </span>
    );
  }
  return (
    <span data-marker-state={state} className="relative flex items-center justify-center shrink-0">
      <span className="h-2 w-2 rounded-full border border-muted-foreground/40" />
    </span>
  );
}

export function getStepLabelClass(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return "text-foreground font-medium";
  if (isCompleted) return "text-muted-foreground";
  return "text-muted-foreground/60";
}
