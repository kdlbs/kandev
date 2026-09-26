"use client";

import { useTranslation } from "react-i18next";
import type { RefObject } from "react";

import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";

type TerminalCloseInlineConfirmationProps = {
  open?: boolean;
  terminalId: string;
  terminalLabel: string;
  focusReturnRef?: RefObject<HTMLElement | null>;
  density?: "compact" | "touch";
  testId?: string;
  onCancel: () => void;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
};

export function TerminalCloseInlineConfirmation({
  open = true,
  terminalId,
  terminalLabel,
  focusReturnRef,
  density = "compact",
  testId = "terminal-menu-close-confirmation",
  onCancel,
  onClose,
  onConfirm,
}: TerminalCloseInlineConfirmationProps) {
  const { t } = useTranslation();

  const inline = (
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
  return (
    <MobileActionConfirmation
      open={open}
      targetKey={terminalId}
      title={t("task:closeTerminal")}
      subject={terminalLabel}
      testId={testId}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:closeTerminal2")}
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
      onClose={onClose}
      onConfirm={onConfirm}
      focusReturnRef={focusReturnRef}
      fallback={inline}
    />
  );
}
