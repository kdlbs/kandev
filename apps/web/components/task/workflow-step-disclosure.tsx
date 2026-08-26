"use client";

import {
  forwardRef,
  useEffect,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type FocusEvent,
  type RefObject,
} from "react";
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
import { IconArrowRight, IconChevronDown } from "@tabler/icons-react";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";
import { useTranslation } from "react-i18next";

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
  onMove: (stepId: string) => Promise<boolean>;
};

export function MinimalWorkflowStepper({
  sortedSteps,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
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
    />
  );
}

type CompactWorkflowDisclosureControls = {
  open: boolean;
  setOpen: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
  setTriggerRef: (node: HTMLButtonElement | null) => void;
  contentRef: RefObject<HTMLDivElement | null>;
  openDisclosure: () => void;
  openDisclosureFromFocus: () => void;
  scheduleClose: () => void;
  cancelScheduledClose: () => void;
  handleTriggerFocus: () => void;
  handleTriggerBlur: (event: FocusEvent<HTMLButtonElement>) => void;
  handleContentFocus: () => void;
  handleContentBlur: (event: FocusEvent<HTMLDivElement>) => void;
  handleOpenAutoFocus: (event: Event) => void;
  handleCloseAutoFocus: (event: Event) => void;
};

function isElementWithin<T extends HTMLElement>(
  target: EventTarget | null,
  ref: RefObject<T | null>,
): boolean {
  return target instanceof Node && ref.current?.contains(target) === true;
}

function useCompactDisclosureCloseTimer(
  triggerRef: RefObject<HTMLButtonElement | null>,
  contentRef: RefObject<HTMLDivElement | null>,
  setOpen: (open: boolean) => void,
) {
  const closeTimerRef = useRef<number | null>(null);
  const cancelScheduledClose = () => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  };
  const scheduleClose = () => {
    cancelScheduledClose();
    closeTimerRef.current = window.setTimeout(() => {
      closeTimerRef.current = null;
      const activeElement = document.activeElement;
      const focusIsInsideDisclosure =
        isElementWithin(activeElement, triggerRef) || isElementWithin(activeElement, contentRef);
      if (!focusIsInsideDisclosure) setOpen(false);
    }, 100);
  };
  useEffect(
    () => () => {
      if (closeTimerRef.current !== null) window.clearTimeout(closeTimerRef.current);
    },
    [],
  );
  return { cancelScheduledClose, scheduleClose };
}

function useCompactWorkflowDisclosure(): CompactWorkflowDisclosureControls {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const setTriggerRef = (node: HTMLButtonElement | null) => {
    triggerRef.current = node;
  };
  const contentRef = useRef<HTMLDivElement>(null);
  const suppressFocusOpenRef = useRef(false);
  const openedFromFocusRef = useRef(false);
  const contentHasFocusRef = useRef(false);
  const { cancelScheduledClose, scheduleClose } = useCompactDisclosureCloseTimer(
    triggerRef,
    contentRef,
    setOpen,
  );
  const openDisclosure = () => {
    openedFromFocusRef.current = false;
    cancelScheduledClose();
    setOpen(true);
  };
  const openDisclosureFromFocus = () => {
    openedFromFocusRef.current = !open;
    cancelScheduledClose();
    setOpen(true);
  };
  const handleTriggerFocus = () => {
    if (suppressFocusOpenRef.current) {
      suppressFocusOpenRef.current = false;
      return;
    }
    openDisclosureFromFocus();
  };
  const handleTriggerBlur = (event: FocusEvent<HTMLButtonElement>) => {
    suppressFocusOpenRef.current = false;
    if (isElementWithin(event.relatedTarget, contentRef)) {
      cancelScheduledClose();
      return;
    }
    scheduleClose();
  };
  const handleContentBlur = (event: FocusEvent<HTMLDivElement>) => {
    const relatedTarget = event.relatedTarget;
    if (isElementWithin(relatedTarget, contentRef)) {
      contentHasFocusRef.current = true;
      cancelScheduledClose();
      return;
    }
    contentHasFocusRef.current = false;
    if (isElementWithin(relatedTarget, triggerRef)) {
      cancelScheduledClose();
      return;
    }
    scheduleClose();
  };
  const handleContentFocus = () => {
    contentHasFocusRef.current = true;
    cancelScheduledClose();
  };
  const handleCloseAutoFocus = (event: Event) => {
    event.preventDefault();
    if (contentHasFocusRef.current) {
      contentHasFocusRef.current = false;
      suppressFocusOpenRef.current = true;
      triggerRef.current?.focus();
    }
  };
  const handleOpenAutoFocus = (event: Event) => {
    if (!openedFromFocusRef.current) event.preventDefault();
    openedFromFocusRef.current = false;
  };
  return {
    open,
    setOpen,
    triggerRef,
    setTriggerRef,
    contentRef,
    openDisclosure,
    openDisclosureFromFocus,
    scheduleClose,
    cancelScheduledClose,
    handleTriggerFocus,
    handleTriggerBlur,
    handleContentFocus,
    handleContentBlur,
    handleOpenAutoFocus,
    handleCloseAutoFocus,
  };
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
}: {
  sortedSteps: Step[];
  current: Step;
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  onMove: (stepId: string) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const controls = useCompactWorkflowDisclosure();
  const trigger = (
    <CompactWorkflowTrigger
      current={current}
      currentIndex={currentIndex}
      total={sortedSteps.length}
      usesTouchDrawer={usesTouchDrawer}
      controls={controls}
    />
  );
  const handleDisclosureMove = async (stepId: string) => {
    const moved = await onMove(stepId);
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
        <StepCircleIndicator isCurrent isCompleted={false} />
        <span className="truncate text-xs font-medium leading-none text-foreground">
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
  onMove,
}: {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
  taskId: string;
  workflowId: string;
  movingToStepId: string | null;
  onMove: (stepId: string) => Promise<boolean>;
}) {
  const { t } = useTranslation();
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
          <div
            key={step.id}
            data-testid={`workflow-step-disclosure-row-${step.id}`}
            aria-current={isCurrent ? "step" : undefined}
            className="flex min-h-11 items-center gap-2 rounded-md px-2 py-1.5"
          >
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <StepCircleIndicator isCurrent={isCurrent} isCompleted={isCompleted} />
              <span
                className={cn(
                  "min-w-0 truncate text-xs",
                  getStepLabelClass(isCurrent, isCompleted),
                )}
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
                <Button
                  type="button"
                  data-testid={`workflow-step-disclosure-move-${step.id}`}
                  size="sm"
                  variant="default"
                  className="h-7 shrink-0 cursor-pointer rounded-sm px-2.5 text-xs [@media(pointer:coarse)]:h-11"
                  disabled={movingToStepId !== null}
                  onClick={() => void onMove(step.id)}
                >
                  <IconArrowRight className="h-3 w-3" />
                  {movingToStepId === step.id ? t("task:moving") : t("task:moveHere")}
                </Button>
              )
            )}
          </div>
        );
      })}
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

export function StepCircleIndicator({
  isCurrent,
  isCompleted,
}: {
  isCurrent: boolean;
  isCompleted: boolean;
}) {
  if (isCurrent) {
    return (
      <span className="relative flex items-center justify-center shrink-0">
        <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
        <span className="h-2 w-2 rounded-full bg-primary" />
      </span>
    );
  }
  if (isCompleted) {
    return (
      <span className="relative flex items-center justify-center shrink-0">
        <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
      </span>
    );
  }
  return (
    <span className="relative flex items-center justify-center shrink-0">
      <span className="h-2 w-2 rounded-full border border-muted-foreground/40" />
    </span>
  );
}

export function getStepLabelClass(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return "text-foreground font-medium";
  if (isCompleted) return "text-muted-foreground";
  return "text-muted-foreground/60";
}
