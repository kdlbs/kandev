"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";

export type InlineConfirmActionsProps = {
  density?: "compact" | "touch";
  testId?: string;
  ariaLabel?: string;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Inline confirmation actions for rows and menus, without owning mutation state.
 */
export function InlineConfirmActions({
  density = "compact",
  testId = "inline-confirm-actions",
  ariaLabel,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  onCancel,
  onConfirm,
}: InlineConfirmActionsProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const [confirmed, setConfirmed] = useState(false);
  const touch = density === "touch";
  const actionClass = touch ? "h-11 min-w-11 px-2" : "h-10 min-w-10 px-2 text-xs";

  useEffect(() => {
    cancelRef.current?.focus();
  }, []);

  const handleConfirm = () => {
    setConfirmed(true);
    queueMicrotask(() => void onConfirm());
  };

  if (confirmed) return null;

  return (
    <div
      role="group"
      aria-label={ariaLabel}
      data-testid={testId}
      className={`flex shrink-0 items-center justify-end gap-1 ${touch ? "min-h-11" : "w-full min-h-10"}`}
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.preventDefault();
        event.stopPropagation();
        onCancel();
      }}
    >
      <Button
        ref={cancelRef}
        type="button"
        variant="ghost"
        size="sm"
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
        className={`${actionClass} transition-[color,background-color,border-color,transform] duration-100 active:scale-[0.96]`}
        onClick={handleConfirm}
      >
        {confirmLabel}
      </Button>
    </div>
  );
}
