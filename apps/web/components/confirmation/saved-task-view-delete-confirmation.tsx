"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "./action-confirm-popover";
import { InlineConfirmActions } from "./inline-confirm-actions";
import { MobileActionConfirmation } from "./mobile-action-confirmation";
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

  const actions = {
    description,
    cancelLabel: t("common:cancel"),
    confirmLabel: t("common:delete"),
    confirmAriaLabel: t("common:deleteSavedTaskViewAction", { name: target.label }),
    confirmDisabled,
    testId,
    confirmTestId,
    onConfirm: () => onConfirm(target.id),
  };
  const fallback =
    presentation === "popover" ? (
      <ActionConfirmPopover
        {...actions}
        open={open}
        anchorRef={anchorRef}
        focusBoundaryRef={focusBoundaryRef}
        title={title}
        confirmationBoundary
        onOpenChange={onOpenChange}
      />
    ) : (
      <InlineConfirmActions
        {...actions}
        density="touch"
        ariaLabel={title}
        description={
          <>
            <span className="block font-medium text-foreground">{title}</span>
            <span className="mt-1 block">{description}</span>
          </>
        }
        onCancel={() => {
          onOpenChange(false);
          queueMicrotask(() => {
            if (anchorRef.current?.isConnected) anchorRef.current.focus();
          });
        }}
        onClose={() => onOpenChange(false)}
      />
    );
  return (
    <MobileActionConfirmation
      {...actions}
      open={open}
      title={title}
      targetKey={target.id}
      focusReturnRef={anchorRef}
      onOpenChange={onOpenChange}
      fallback={fallback}
    />
  );
}
