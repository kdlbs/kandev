"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { SessionDeleteDescription } from "../session-delete-description";

export function MobileSessionDeleteConfirmation({
  open,
  targetKey,
  isPrimary,
  isOnlySession,
  targetName,
  focusReturnRef,
  onCancel,
  onClose,
  onConfirm,
}: {
  open: boolean;
  targetKey: string;
  isPrimary: boolean;
  isOnlySession: boolean;
  targetName: string;
  focusReturnRef: RefObject<HTMLElement | null>;
  onCancel: () => void;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const description = (
    <SessionDeleteDescription isPrimary={isPrimary} isOnlySession={isOnlySession} />
  );
  const actions = {
    testId: "mobile-session-delete-confirmation",
    description,
    cancelLabel: t("common:cancel"),
    confirmLabel: t("task:delete"),
    confirmAriaLabel: t("task:delete2", { name: targetName }),
    confirmTestId: "mobile-session-delete-confirm",
    onClose,
    onConfirm,
  };
  return (
    <MobileActionConfirmation
      {...actions}
      open={open}
      targetKey={targetKey}
      title={t("task:deleteSession")}
      subject={targetName}
      focusReturnRef={focusReturnRef}
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
      fallback={
        <InlineConfirmActions
          {...actions}
          density="touch"
          ariaLabel={t("task:deleteSession")}
          onCancel={onCancel}
        />
      }
    />
  );
}
