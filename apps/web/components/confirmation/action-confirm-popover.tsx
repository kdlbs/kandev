"use client";

import { useId, useLayoutEffect, useRef, type ReactNode, type RefObject } from "react";
import { Button } from "@kandev/ui/button";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
} from "@kandev/ui/popover";

export type ActionConfirmPopoverProps = {
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  title: ReactNode;
  description?: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  testId?: string;
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
  anchorRef,
  focusBoundaryRef,
  title,
  description,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  testId = "action-confirm-popover",
  onOpenChange,
  onCancel,
  onConfirm,
}: ActionConfirmPopoverProps) {
  const titleId = useId();
  const descriptionId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const confirmedRef = useRef(false);

  // Intentionally runs on every render: an anchor can disappear through live
  // data without changing the confirmation's open state, so each render must
  // re-check the guard before the shell can invoke a stale action.
  useLayoutEffect(() => {
    if (!open) {
      confirmedRef.current = false;
      return;
    }
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
        titleId={titleId}
        descriptionId={descriptionId}
        title={title}
        description={description}
        cancelLabel={cancelLabel}
        confirmLabel={confirmLabel}
        confirmAriaLabel={confirmAriaLabel}
        confirmTestId={confirmTestId}
        testId={testId}
        cancelRef={cancelRef}
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
  titleId: string;
  descriptionId: string;
  title: ReactNode;
  description?: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  testId: string;
  cancelRef: RefObject<HTMLButtonElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  confirmedRef: { current: boolean };
  anchorRef: RefObject<HTMLElement | null>;
  onCancel: () => void;
  onConfirm: () => void;
};

function ActionConfirmPopoverContent({
  titleId,
  descriptionId,
  title,
  description,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  testId,
  cancelRef,
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
      side="bottom"
      align="end"
      sideOffset={8}
      className="w-64 gap-3 p-3"
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        cancelRef.current?.focus();
      }}
      onFocusOutside={(event) => {
        if (focusBoundaryRef?.current?.contains(event.target as Node)) event.preventDefault();
      }}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        if (!confirmedRef.current && isConnected(anchorRef.current)) anchorRef.current.focus();
        confirmedRef.current = false;
      }}
    >
      <PopoverHeader>
        <PopoverTitle id={titleId}>{title}</PopoverTitle>
        {description ? (
          <PopoverDescription id={descriptionId}>{description}</PopoverDescription>
        ) : null}
      </PopoverHeader>
      <div className="flex justify-end gap-2">
        <Button
          ref={cancelRef}
          type="button"
          variant="outline"
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
