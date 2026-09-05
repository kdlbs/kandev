"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type MouseEvent as ReactMouseEvent,
  type MouseEventHandler as ReactMouseEventHandler,
  type RefObject,
} from "react";
import { IconArrowRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useHoverPopover } from "@/components/integrations/use-hover-popover";
import { WorkflowMoveOptions, WorkflowMoveOptionsForm } from "./workflow-move-options";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

export const WORKFLOW_MOVE_LONG_PRESS_MS = 450;
export const WORKFLOW_MOVE_LONG_PRESS_SLOP_PX = 10;

export function isWithinLongPressSlop(
  startX: number,
  startY: number,
  x: number,
  y: number,
): boolean {
  const dx = x - startX;
  const dy = y - startY;
  return Math.sqrt(dx * dx + dy * dy) <= WORKFLOW_MOVE_LONG_PRESS_SLOP_PX;
}

type LongPressPointerHandlers = {
  onPointerDown: (event: ReactPointerEvent<HTMLElement>) => void;
  onPointerMove: (event: ReactPointerEvent<HTMLElement>) => void;
  onPointerUp: (event: ReactPointerEvent<HTMLElement>) => void;
  onPointerCancel: (event: ReactPointerEvent<HTMLElement>) => void;
};

/**
 * Long-press gesture for the coarse-pointer proceed button. A short tap must
 * still fire the normal click, so the hook only reports "this click belongs
 * to a completed long press" through `consumePendingClick`, which the caller
 * checks (and consumes) in its click handler.
 */
export function useWorkflowMoveLongPress(onLongPress: () => void): {
  pointerHandlers: LongPressPointerHandlers;
  consumePendingClick: () => boolean;
} {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const originRef = useRef<{ x: number; y: number } | null>(null);
  const pendingClickRef = useRef(false);
  const onLongPressRef = useRef(onLongPress);
  onLongPressRef.current = onLongPress;

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    originRef.current = null;
  }, []);

  // Unmount cancels any in-flight long press and its pending click.
  useEffect(
    () => () => {
      clearTimer();
      pendingClickRef.current = false;
    },
    [clearTimer],
  );

  const onPointerDown = useCallback(
    (event: ReactPointerEvent<HTMLElement>) => {
      pendingClickRef.current = false;
      // Secondary buttons never start a long press.
      if (event.button !== 0) {
        clearTimer();
        return;
      }
      clearTimer();
      originRef.current = { x: event.clientX, y: event.clientY };
      timerRef.current = setTimeout(() => {
        originRef.current = null;
        pendingClickRef.current = true;
        onLongPressRef.current();
      }, WORKFLOW_MOVE_LONG_PRESS_MS);
    },
    [clearTimer],
  );

  const onPointerMove = useCallback(
    (event: ReactPointerEvent<HTMLElement>) => {
      const origin = originRef.current;
      if (!origin) return;
      if (!isWithinLongPressSlop(origin.x, origin.y, event.clientX, event.clientY)) {
        clearTimer();
      }
    },
    [clearTimer],
  );

  const onPointerUp = useCallback(() => clearTimer(), [clearTimer]);
  const onPointerCancel = useCallback(() => clearTimer(), [clearTimer]);

  const consumePendingClick = useCallback(() => {
    if (!pendingClickRef.current) return false;
    pendingClickRef.current = false;
    return true;
  }, []);

  return {
    pointerHandlers: { onPointerDown, onPointerMove, onPointerUp, onPointerCancel },
    consumePendingClick,
  };
}

type ProceedSurfaceCommon = {
  nextStepName: string;
  isMoving: boolean;
  className?: string;
  testId: string;
  triggerRef: RefObject<HTMLButtonElement | null>;
  onDirectClick: (event: ReactMouseEvent<HTMLButtonElement>) => void;
  submit: (options?: WorkflowMoveEntryOptions) => Promise<boolean>;
};

/**
 * Coarse-pointer surface: a long press opens the shared options Drawer under
 * the held pointer; a short tap moves immediately. The click-capture guard
 * swallows the synthetic click a completed long press leaves behind before it
 * can retarget onto the Drawer submit button.
 */
function CoarseProceedSurface({
  nextStepName,
  isMoving,
  className,
  testId,
  triggerRef,
  onDirectClick,
  submit,
  optionsOpen,
  onOptionsOpenChange,
  pointerHandlers,
  onTouchSurfaceClickCapture,
}: ProceedSurfaceCommon & {
  optionsOpen: boolean;
  onOptionsOpenChange: (open: boolean) => void;
  pointerHandlers: LongPressPointerHandlers;
  onTouchSurfaceClickCapture: ReactMouseEventHandler<HTMLDivElement>;
}) {
  return (
    <div className="contents" onClickCapture={onTouchSurfaceClickCapture}>
      <Button
        type="button"
        variant="outline"
        size="sm"
        ref={triggerRef}
        className={cn("gap-1 px-2.5 text-xs cursor-pointer text-primary min-h-11", className)}
        onClick={onDirectClick}
        disabled={isMoving}
        data-testid={testId}
        {...pointerHandlers}
      >
        {nextStepName}
        <IconArrowRight className="h-3.5 w-3.5" />
      </Button>
      <WorkflowMoveOptions
        open={optionsOpen}
        onOpenChange={onOptionsOpenChange}
        targetStepName={nextStepName}
        isMoving={isMoving}
        onSubmit={async (options) => {
          const ok = await submit(options);
          if (ok) onOptionsOpenChange(false);
          return ok;
        }}
      />
    </div>
  );
}

/**
 * Fine-pointer surface: hovering the button reveals the shared options form in
 * an anchored popover, so the one-shot fields are discoverable and opt-in; a
 * plain click still moves immediately. A failed move keeps the form open.
 */
function FineProceedSurface({
  nextStepName,
  isMoving,
  className,
  testId,
  triggerRef,
  onDirectClick,
  submit,
  open,
  onOpenChange,
  onTriggerEnter,
  onTriggerLeave,
  onContentEnter,
  onContentLeave,
}: ProceedSurfaceCommon & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onTriggerEnter: () => void;
  onTriggerLeave: () => void;
  onContentEnter: () => void;
  onContentLeave: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          ref={triggerRef}
          className={cn("gap-1 px-2.5 text-xs cursor-pointer text-primary", className)}
          onClick={onDirectClick}
          disabled={isMoving}
          data-testid={testId}
          onMouseEnter={onTriggerEnter}
          onMouseMove={onTriggerEnter}
          onPointerEnter={onTriggerEnter}
          onMouseLeave={onTriggerLeave}
          onPointerLeave={onTriggerLeave}
          onFocus={onTriggerEnter}
          onBlur={onTriggerLeave}
        >
          {nextStepName}
          <IconArrowRight className="h-3.5 w-3.5" />
        </Button>
      </PopoverAnchor>
      <PopoverContent
        align="end"
        side="top"
        sideOffset={6}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
        data-testid={`${testId}-options`}
        onMouseEnter={onContentEnter}
        onMouseMove={onContentEnter}
        onMouseLeave={onContentLeave}
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <div className="mb-2 text-xs font-medium">
          {t("task:workflowMoveOptionsTitle", { step: nextStepName })}
        </div>
        <WorkflowMoveOptionsForm
          isMoving={isMoving}
          isTouchSurface={false}
          instructionsRows={3}
          onSubmit={async (options) => {
            const ok = await submit(options);
            if (ok) onOpenChange(false);
            return ok;
          }}
        />
      </PopoverContent>
    </Popover>
  );
}

/** Returns the trigger ref plus a deferred focus restore for the triggerless Drawer close. */
function useDrawerFocusReturn() {
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const focusRestoreTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (focusRestoreTimerRef.current !== null) clearTimeout(focusRestoreTimerRef.current);
    },
    [],
  );

  const scheduleDrawerFocusReturn = useCallback(() => {
    if (focusRestoreTimerRef.current !== null) {
      clearTimeout(focusRestoreTimerRef.current);
    }
    focusRestoreTimerRef.current = setTimeout(() => {
      focusRestoreTimerRef.current = null;
      if (triggerRef.current?.isConnected && !triggerRef.current.disabled)
        triggerRef.current.focus();
    }, 0);
  }, []);

  return { triggerRef, scheduleDrawerFocusReturn };
}

type WorkflowMoveProceedButtonProps = {
  nextStepName: string;
  onProceed: (options?: WorkflowMoveEntryOptions) => boolean | void | Promise<boolean | void>;
  isMoving: boolean;
  className?: string;
  testId: string;
};

/**
 * One primary "move to next step" button. A plain click/tap moves immediately.
 * On fine pointers, hovering the button reveals the one-shot options in an
 * anchored popover; on coarse pointers a long press opens the same fields in a
 * Drawer. A failed move keeps the form and its draft open.
 */
export function WorkflowMoveProceedButton({
  nextStepName,
  onProceed,
  isMoving,
  className,
  testId,
}: WorkflowMoveProceedButtonProps) {
  const usesTouchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const proceedInFlightRef = useRef(false);
  const { triggerRef, scheduleDrawerFocusReturn } = useDrawerFocusReturn();
  const hover = useHoverPopover({ openDelayMs: 120, closeDelayMs: 200, disabled: usesTouchDrawer });
  const { pointerHandlers, consumePendingClick } = useWorkflowMoveLongPress(() => {
    setDrawerOpen(true);
  });

  useEffect(() => {
    if (!isMoving) proceedInFlightRef.current = false;
  }, [isMoving]);

  const handleDrawerOpenChange = useCallback(
    (open: boolean) => {
      setDrawerOpen(open);
      if (!open) {
        scheduleDrawerFocusReturn();
      }
    },
    [scheduleDrawerFocusReturn],
  );

  const tryProceed = useCallback(
    async (options?: WorkflowMoveEntryOptions): Promise<boolean> => {
      if (isMoving || proceedInFlightRef.current) return false;
      proceedInFlightRef.current = true;
      try {
        const result = await onProceed(options);
        return result !== false;
      } finally {
        proceedInFlightRef.current = false;
      }
    },
    [isMoving, onProceed],
  );

  const handleClick = (event: ReactMouseEvent<HTMLButtonElement>) => {
    // On coarse pointers a completed long press produces a synthetic click; the
    // Drawer is already open, so swallow the direct move. Fine pointers never
    // long-press, so a click there always moves.
    if (usesTouchDrawer && consumePendingClick()) {
      event.preventDefault();
      return;
    }
    void tryProceed();
  };

  const handleTouchSurfaceClickCapture = (event: ReactMouseEvent<HTMLDivElement>) => {
    if (!consumePendingClick()) return;
    // When a long press opens the Drawer under the held pointer, the browser's
    // compatibility click can retarget to the Drawer submit button on release.
    // Consume that one click before it can submit an empty options form.
    event.preventDefault();
    event.stopPropagation();
  };

  if (usesTouchDrawer) {
    return (
      <CoarseProceedSurface
        nextStepName={nextStepName}
        isMoving={isMoving}
        className={className}
        testId={testId}
        triggerRef={triggerRef}
        onDirectClick={handleClick}
        submit={tryProceed}
        optionsOpen={drawerOpen}
        onOptionsOpenChange={handleDrawerOpenChange}
        pointerHandlers={pointerHandlers}
        onTouchSurfaceClickCapture={handleTouchSurfaceClickCapture}
      />
    );
  }

  return (
    <FineProceedSurface
      nextStepName={nextStepName}
      isMoving={isMoving}
      className={className}
      testId={testId}
      triggerRef={triggerRef}
      onDirectClick={handleClick}
      submit={tryProceed}
      open={hover.open}
      onOpenChange={hover.onOpenChange}
      onTriggerEnter={hover.onTriggerEnter}
      onTriggerLeave={hover.onTriggerLeave}
      onContentEnter={hover.onContentEnter}
      onContentLeave={hover.onContentLeave}
    />
  );
}
