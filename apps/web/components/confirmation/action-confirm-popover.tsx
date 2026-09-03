"use client";

import { useId, useLayoutEffect, useRef, type ReactNode, type RefObject } from "react";
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

  // Intentionally runs on every render: an anchor can disappear through live
  // data without changing the confirmation's open state, so each render must
  // re-check the guard before the shell can invoke a stale action.
  useLayoutEffect(() => {
    // Keep a confirmed close marked until Radix finishes close-autofocus. An
    // early reset can refocus this anchor while a following popover is opening.
    if (!open) return;
    if (isConnected(anchorRef.current)) return;
    onCancel?.();
    onOpenChange(false);
  });

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      confirmedRef.current = false;
      onOpenChange(true);
      return;
    }
    closeActionConfirm(confirmedRef, onCancel, onOpenChange);
  };

  const handleConfirm = () => {
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
  };

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
        onCancel={() => handleOpenChange(false)}
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

function ActionConfirmPopoverContent({
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
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        cancelRef.current?.focus();
      }}
      onFocusOutside={(event) => {
        if (focusBoundaryRef?.current?.contains(event.target as Node)) event.preventDefault();
      }}
      onInteractOutside={(event) => {
        const target = event.target as Node;
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
