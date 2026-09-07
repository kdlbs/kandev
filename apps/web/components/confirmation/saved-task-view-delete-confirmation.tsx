"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "./action-confirm-popover";
import { InlineConfirmActions } from "./inline-confirm-actions";
import type { SavedTaskViewDeleteTarget } from "./use-saved-task-view-delete-confirmation";

export type { SavedTaskViewDeleteTarget } from "./use-saved-task-view-delete-confirmation";

type SavedTaskViewDeleteConfirmationProps = {
  target: SavedTaskViewDeleteTarget;
  presentation: "popover" | "inline";
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  confirmDisabled?: boolean;
  testId?: string;
  confirmTestId?: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: (id: string) => void | Promise<void>;
};

export function SavedTaskViewDeleteConfirmation({
  target,
  presentation,
  open,
  anchorRef,
  focusBoundaryRef,
  confirmDisabled = false,
  testId = "saved-task-view-delete-confirmation",
  confirmTestId = "saved-task-view-delete-confirm",
  onOpenChange,
  onConfirm,
}: SavedTaskViewDeleteConfirmationProps) {
  const { t } = useTranslation();
  const title = t("common:deleteSavedTaskViewTitle", { name: target.label });
  const description = t("common:deleteSavedTaskViewDescription");

  if (presentation === "popover") {
    return (
      <ActionConfirmPopover
        open={open}
        anchorRef={anchorRef}
        focusBoundaryRef={focusBoundaryRef}
        title={title}
        description={description}
        cancelLabel={t("common:cancel")}
        confirmLabel={t("common:delete")}
        confirmAriaLabel={t("common:deleteSavedTaskViewAction", { name: target.label })}
        confirmDisabled={confirmDisabled}
        testId={testId}
        confirmTestId={confirmTestId}
        confirmationBoundary
        onOpenChange={onOpenChange}
        onConfirm={() => onConfirm(target.id)}
      />
    );
  }

  if (!open) return null;

  return (
    <InlineConfirmActions
      density="touch"
      testId={testId}
      ariaLabel={title}
      description={
        <>
          <span className="block font-medium text-foreground">{title}</span>
          <span className="mt-1 block">{description}</span>
        </>
      }
      cancelLabel={t("common:cancel")}
      confirmLabel={t("common:delete")}
      confirmAriaLabel={t("common:deleteSavedTaskViewAction", { name: target.label })}
      confirmTestId={confirmTestId}
      confirmDisabled={confirmDisabled}
      onCancel={() => {
        onOpenChange(false);
        queueMicrotask(() => {
          if (anchorRef.current?.isConnected) anchorRef.current.focus();
        });
      }}
      onClose={() => onOpenChange(false)}
      onConfirm={() => onConfirm(target.id)}
    />
  );
}
