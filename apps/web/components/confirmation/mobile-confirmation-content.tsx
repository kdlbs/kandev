"use client";

import { useId, useLayoutEffect, useRef, type ReactNode, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

export type MobileConfirmationContentProps = {
  title: ReactNode;
  subject?: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  titleId?: string;
  descriptionId?: string;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  confirmAriaLabel?: string;
  confirmTestId?: string;
  testId?: string;
  variant?: "default" | "destructive";
  disabled?: boolean;
  confirmDisabled?: boolean;
  onBack?: () => void;
  onCancel: () => void;
  onConfirm: () => void;
};

function cancelOnEscape(event: KeyboardEvent, disabled: boolean | undefined, cancel: () => void) {
  if (event.key !== "Escape") return;
  event.preventDefault();
  event.stopPropagation();
  if (!disabled) cancel();
}

export function MobileConfirmationContent({
  title,
  subject,
  description,
  children,
  titleId,
  descriptionId,
  cancelLabel,
  confirmLabel,
  confirmAriaLabel,
  confirmTestId,
  testId,
  variant = "destructive",
  disabled,
  confirmDisabled,
  onBack,
  onCancel,
  onConfirm,
}: MobileConfirmationContentProps) {
  const { t } = useTranslation();
  const id = useId();
  const headingId = titleId ?? `${id}-title`;
  const bodyId = descriptionId ?? `${id}-description`;
  const cancelRef = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => {
    cancelRef.current?.focus();
  }, []);

  return (
    <section
      role="group"
      aria-labelledby={headingId}
      aria-describedby={bodyId}
      data-testid={testId ?? "mobile-action-confirmation"}
      data-confirmation-boundary=""
      className="flex min-h-0 min-w-0 flex-1 flex-col text-left text-sm text-foreground"
      onPointerDown={(event) => event.stopPropagation()}
      onMouseDown={(event) => event.stopPropagation()}
      onClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") event.stopPropagation();
      }}
      onKeyDownCapture={(event) => cancelOnEscape(event, disabled, onCancel)}
    >
      <header className="shrink-0 px-4 pb-3 pt-2">
        {onBack && (
          <Button
            type="button"
            variant="ghost"
            disabled={disabled}
            onClick={onBack}
            className="-ml-3 mb-1 min-h-11 cursor-pointer px-3 text-sm"
          >
            <IconArrowLeft className="size-4" aria-hidden="true" />
            {t("common:back")}
          </Button>
        )}
        <h2 id={headingId} className="text-lg font-semibold [overflow-wrap:anywhere]">
          {title}
        </h2>
      </header>
      <div
        data-testid="mobile-confirmation-body"
        className="min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 text-sm leading-6 [overflow-wrap:anywhere]"
      >
        <div id={bodyId} className="space-y-3">
          {subject && <p className="font-medium">{subject}</p>}
          {description && <div>{description}</div>}
        </div>
        {children}
      </div>
      <footer className="flex shrink-0 flex-col gap-2 px-4 pt-5 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <Button
          type="button"
          variant={variant}
          disabled={disabled || confirmDisabled}
          aria-label={confirmAriaLabel}
          data-testid={confirmTestId}
          onClick={onConfirm}
          className="h-auto min-h-12 w-full cursor-pointer whitespace-normal px-4 py-3 text-sm"
        >
          {confirmLabel}
        </Button>
        <Button
          ref={cancelRef}
          type="button"
          variant="outline"
          disabled={disabled}
          onClick={onCancel}
          className="h-auto min-h-12 w-full cursor-pointer whitespace-normal px-4 py-3 text-sm"
        >
          {cancelLabel}
        </Button>
      </footer>
    </section>
  );
}
