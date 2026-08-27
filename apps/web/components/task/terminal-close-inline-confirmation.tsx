"use client";

import { useTranslation } from "react-i18next";

import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";

type TerminalCloseInlineConfirmationProps = {
  density?: "compact" | "touch";
  testId?: string;
  onCancel: () => void;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
};

export function TerminalCloseInlineConfirmation({
  density = "compact",
  testId = "terminal-menu-close-confirmation",
  onCancel,
  onClose,
  onConfirm,
}: TerminalCloseInlineConfirmationProps) {
  const { t } = useTranslation();

  return (
    <InlineConfirmActions
      density={density}
      testId={testId}
      ariaLabel={t("task:closeTerminal")}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:closeTerminal2")}
      onCancel={onCancel}
      onClose={onClose}
      onConfirm={onConfirm}
    />
  );
}
