"use client";

import {
  memo,
  useCallback,
  useId,
  useLayoutEffect,
  useRef,
  type ReactNode,
  type RefObject,
} from "react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
} from "@kandev/ui/popover";

export type ActionConfirmPopoverSize = "default" | "wide";

export type ActionConfirmPopoverProps = {
  open: boolean;
  size?: ActionConfirmPopoverSize;
  disabled?: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  title: ReactNode;
  description?: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  confirmDisabled?: boolean;
  testId?: string;
  confirmationBoundary?: boolean;
  onOpenChange: (open: boolean) => void;
  onCancel?: () => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * A non-modal confirmation surface for one anchored action.
 *
 * The component owns only the confirmation shell. Consumers retain mutation,
 * pending, error, and success state, and the shell closes before confirmation
 * invokes the consumer callback.
 */
export function ActionConfirmPopover({
  open,
  size = "default",
  disabled = false,
  anchorRef,
  focusReturnRef,
  focusBoundaryRef,
  title,
  description,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  confirmDisabled = false,
  testId = "action-confirm-popover",
  confirmationBoundary = false,
  onOpenChange,
  onCancel,
  onConfirm,
}: ActionConfirmPopoverProps) {
  const titleId = useId();
  const descriptionId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmedRef = useRef(false);
  const confirmIsDisabled = disabled || confirmDisabled;

  useLayoutEffect(() => {
    if (!open) return;
    if (isConnected(anchorRef.current)) return;
    onCancel?.();
    onOpenChange(false);
  });

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) {
        confirmedRef.current = false;
        onOpenChange(true);
        return;
      }
      closeActionConfirm(confirmedRef, onCancel, onOpenChange);
    },
    [onCancel, onOpenChange],
  );

  const handleConfirm = useCallback(() => {
    if (confirmIsDisabled) return;
    if (!isConnected(anchorRef.current)) {
      handleOpenChange(false);
      return;
    }
    confirmedRef.current = true;
    handleOpenChange(false);
    queueMicrotask(() => {
      void Promise.resolve()
        .then(onConfirm)
        .catch(() => undefined);
    });
  }, [anchorRef, confirmIsDisabled, handleOpenChange, onConfirm]);

  const handleCancel = useCallback(() => handleOpenChange(false), [handleOpenChange]);

  return (
    <Popover modal={false} open={open} onOpenChange={handleOpenChange}>
      {/* Radix accepts a null current value at runtime while its public type omits it. */}
      <PopoverAnchor virtualRef={anchorRef as RefObject<HTMLElement>} />
      <ActionConfirmPopoverContent
        size={size}
        titleId={titleId}
        descriptionId={descriptionId}
        title={title}
        description={description}
        cancelLabel={cancelLabel}
        confirmLabel={confirmLabel}
        confirmAriaLabel={confirmAriaLabel}
        confirmTestId={confirmTestId}
        confirmDisabled={confirmIsDisabled}
        testId={testId}
        confirmationBoundary={confirmationBoundary}
        disabled={disabled}
        cancelRef={cancelRef}
        focusReturnRef={focusReturnRef}
        focusBoundaryRef={focusBoundaryRef}
        confirmedRef={confirmedRef}
        anchorRef={anchorRef}
        onCancel={handleCancel}
        onConfirm={handleConfirm}
      />
    </Popover>
  );
}

type ActionConfirmPopoverContentProps = {
  size: ActionConfirmPopoverSize;
  titleId: string;
  descriptionId: string;
  title: ReactNode;
  description?: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  confirmDisabled: boolean;
  testId: string;
  confirmationBoundary: boolean;
  disabled: boolean;
  cancelRef: RefObject<HTMLButtonElement | null>;
  focusReturnRef?: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  confirmedRef: { current: boolean };
  anchorRef: RefObject<HTMLElement | null>;
  onCancel: () => void;
  onConfirm: () => void;
};

const ActionConfirmPopoverContent = memo(function ActionConfirmPopoverContent({
  size,
  titleId,
  descriptionId,
  title,
  description,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  confirmDisabled,
  testId,
  confirmationBoundary,
  disabled,
  cancelRef,
  focusReturnRef,
  focusBoundaryRef,
  confirmedRef,
  anchorRef,
  onCancel,
  onConfirm,
}: ActionConfirmPopoverContentProps) {
  return (
    <PopoverContent
      role="dialog"
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      data-testid={testId}
      data-confirmation-boundary={confirmationBoundary ? "" : undefined}
      side="bottom"
      align="end"
      sideOffset={8}
      className={cn("gap-3 p-3", size === "wide" ? "w-72 max-w-[calc(100vw-1rem)]" : "w-64")}
      onOpenAutoFocus={(event) => preventAndFocusCancel(event, cancelRef)}
      onFocusOutside={(event) => {
        // Ignore stale focus events from the closing context-menu portal.
        if (isClosingRadixMenuContent(event.target, focusBoundaryRef))
          return event.preventDefault();
        // A replaced boundary fails contains(), so the live trigger ref stays correct.
        if (focusBoundaryRef?.current?.contains(event.target as Node)) event.preventDefault();
      }}
      onInteractOutside={(event) => {
        const target = event.target as Node;
        // Ignore stale interaction events from the closing context-menu portal.
        if (isClosingRadixMenuContent(target, focusBoundaryRef)) return event.preventDefault();
        // If the interaction target is no longer in the DOM (e.g., the context
        // menu content was just removed), it's a stale event from a preceding
        // close — prevent it from closing the popover.
        if (!target.isConnected) {
          event.preventDefault();
          return;
        }
        if (anchorRef.current?.contains(target) || focusBoundaryRef?.current?.contains(target)) {
          event.preventDefault();
        }
      }}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        if (!confirmedRef.current) {
          const focusReturnTarget = focusReturnRef?.current ?? null;
          if (isConnected(focusReturnTarget)) focusReturnTarget.focus();
          else if (isConnected(anchorRef.current)) anchorRef.current.focus();
        }
        confirmedRef.current = false;
      }}
    >
      <PopoverHeader>
        <PopoverTitle id={titleId}>{title}</PopoverTitle>
        {description ? (
          <PopoverDescription
            id={descriptionId}
            className={size === "wide" ? "text-pretty" : undefined}
          >
            {description}
          </PopoverDescription>
        ) : null}
      </PopoverHeader>
      <div className="flex justify-end gap-2">
        <Button
          ref={cancelRef}
          type="button"
          variant="outline"
          disabled={disabled}
          className="min-h-11 px-3 transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]"
          onClick={onCancel}
        >
          {cancelLabel}
        </Button>
        <Button
          type="button"
          variant="destructive"
          aria-label={confirmAriaLabel}
          data-testid={confirmTestId}
          disabled={confirmDisabled}
          className="min-h-11 px-3 transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]"
          onClick={onConfirm}
        >
          {confirmLabel}
        </Button>
      </div>
    </PopoverContent>
  );
});

function preventAndFocusCancel(event: Event, cancelRef: RefObject<HTMLButtonElement | null>) {
  event.preventDefault();
  cancelRef.current?.focus();
}

function closeActionConfirm(
  confirmedRef: { current: boolean },
  onCancel: (() => void) | undefined,
  onOpenChange: (open: boolean) => void,
) {
  if (!confirmedRef.current) onCancel?.();
  onOpenChange(false);
}

function isConnected(element: HTMLElement | null): element is HTMLElement {
  return element !== null && element.isConnected;
}

function isClosingRadixMenuContent(
  target: EventTarget | null,
  boundaryRef?: RefObject<HTMLElement | null>,
): boolean {
  return (
    target instanceof Element &&
    target.closest('[data-radix-menu-content][data-state="closed"]') !== null &&
    boundaryRef?.current?.contains(target) === true
  );
}
