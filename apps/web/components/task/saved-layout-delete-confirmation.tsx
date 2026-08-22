"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import type { SavedLayout } from "@/lib/types/http";

type SavedLayoutDeleteConfirmationProps = {
  layout: SavedLayout | null;
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: (layoutId: string) => void | Promise<void>;
};

export function SavedLayoutDeleteConfirmation({
  layout,
  open,
  anchorRef,
  focusBoundaryRef,
  onOpenChange,
  onConfirm,
}: SavedLayoutDeleteConfirmationProps) {
  const { t } = useTranslation();
  if (!layout) return null;

  const title = t("task:deleteLayoutConfirm", { name: layout.name });
  const description = layout.is_default
    ? t("task:theBuiltInDefaultLayoutWill")
    : t("task:thisSavedLayoutWillBePermanently");

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      focusBoundaryRef={focusBoundaryRef}
      title={title}
      description={description}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:delete")}
      confirmAriaLabel={t("task:delete2", { name: layout.name })}
      confirmTestId="layout-saved-delete-confirm"
      testId="layout-saved-delete-confirm-popover"
      confirmationBoundary
      onOpenChange={onOpenChange}
      onConfirm={() => onConfirm(layout.id)}
    />
  );
}
