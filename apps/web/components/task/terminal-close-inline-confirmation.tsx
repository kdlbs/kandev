"use client";

import { useTranslation } from "react-i18next";

import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";

type TerminalCloseInlineConfirmationProps = {
  density?: "compact" | "touch";
  testId?: string;
  onCancel: () => void;
  onConfirm: () => void;
};

export function TerminalCloseInlineConfirmation({
  density = "compact",
  testId = "terminal-menu-close-confirmation",
  onCancel,
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
      onConfirm={onConfirm}
    />
  );
}
