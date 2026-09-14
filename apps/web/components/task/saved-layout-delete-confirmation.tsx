"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import {
  MobileActionConfirmation,
  useConfirmationBoundary,
} from "@/components/confirmation/mobile-action-confirmation";
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
  const { changed } = useConfirmationBoundary(open, layout?.id ?? "", onOpenChange);
  if (changed) return null;
  if (!layout) return null;

  const title = t("task:deleteLayoutConfirm", { name: layout.name });
  const description = layout.is_default
    ? t("task:theBuiltInDefaultLayoutWill")
    : t("task:thisSavedLayoutWillBePermanently");

  const actions = {
    open,
    title,
    description,
    cancelLabel: t("common:cancel"),
    confirmLabel: t("task:delete"),
    confirmAriaLabel: t("task:delete2", { name: layout.name }),
    confirmTestId: "layout-saved-delete-confirm",
    onOpenChange,
    onConfirm: () => onConfirm(layout.id),
  };
  return (
    <MobileActionConfirmation
      {...actions}
      targetKey={layout.id}
      focusReturnRef={anchorRef}
      fallback={
        <ActionConfirmPopover
          {...actions}
          anchorRef={anchorRef}
          focusBoundaryRef={focusBoundaryRef}
          testId="layout-saved-delete-confirm-popover"
          confirmationBoundary
        />
      }
    />
  );
}
