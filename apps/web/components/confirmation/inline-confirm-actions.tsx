"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";

export type InlineConfirmActionsProps = {
  density?: "compact" | "touch";
  disabled?: boolean;
  testId?: string;
  ariaLabel?: string;
  description?: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  confirmDisabled?: boolean;
  onCancel: () => void;
  onClose?: () => void;
  /** Reject the returned promise to restore the confirmation UI for retry. */
  onConfirm: () => void | Promise<void>;
};

/**
 * Inline confirmation actions for rows and menus, without owning mutation state.
 */
export function InlineConfirmActions({
  density = "compact",
  disabled = false,
  testId = "inline-confirm-actions",
  ariaLabel,
  description,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  confirmDisabled = false,
  onCancel,
  onClose,
  onConfirm,
}: InlineConfirmActionsProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const [confirmed, setConfirmed] = useState(false);
  const touch = density === "touch";
  const actionClass = touch ? "h-11 min-w-11 px-2" : "h-10 min-w-10 px-2 text-xs";
  const confirmIsDisabled = disabled || confirmDisabled;

  useEffect(() => {
    cancelRef.current?.focus();
  }, []);

  const handleConfirm = () => {
    if (confirmIsDisabled) return;
    onClose?.();
    setConfirmed(true);
    queueMicrotask(() => {
      void Promise.resolve()
        .then(onConfirm)
        .catch(() => {
          setConfirmed(false);
        });
    });
  };

  if (confirmed) return null;

  return (
    <div
      role="group"
      aria-label={ariaLabel}
      data-testid={testId}
      className={
        description
          ? "flex min-w-0 flex-1 basis-full flex-col items-stretch gap-2"
          : `flex shrink-0 items-center justify-end gap-1 ${touch ? "min-h-11" : "w-full min-h-10"}`
      }
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.preventDefault();
        event.stopPropagation();
        onCancel();
      }}
    >
      {description ? (
        <p className="min-w-0 max-w-full text-xs text-muted-foreground [overflow-wrap:anywhere]">
          {description}
        </p>
      ) : null}
      <div className="flex items-center justify-end gap-1">
        <Button
          ref={cancelRef}
          type="button"
          variant="ghost"
          size="sm"
          disabled={disabled}
          className={`${actionClass} transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]`}
          onClick={onCancel}
        >
          {cancelLabel}
        </Button>
        <Button
          type="button"
          variant="destructive"
          size="sm"
          aria-label={confirmAriaLabel}
          data-testid={confirmTestId}
          disabled={confirmIsDisabled}
          className={`${actionClass} transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]`}
          onClick={handleConfirm}
        >
          {confirmLabel}
        </Button>
      </div>
    </div>
  );
}
