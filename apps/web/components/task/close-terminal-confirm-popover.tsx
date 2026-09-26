"use client";

import type { RefObject } from "react";
import { useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";

type CloseTerminalConfirmPopoverProps = {
  open: boolean;
  terminalName: string;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Compact confirmation anchored to the close control that opened it.
 * Teardown starts only after confirmation; the popover releases immediately.
 */
export function CloseTerminalConfirmPopover({
  open,
  terminalName,
  anchorRef,
  focusBoundaryRef,
  onOpenChange,
  onConfirm,
}: CloseTerminalConfirmPopoverProps) {
  const { t } = useTranslation();

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      focusBoundaryRef={focusBoundaryRef}
      title={t("task:closeTerminal")}
      description={t("task:thisStopsTheShellAndAny", { terminalName })}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("task:closeTerminal2")}
      testId="terminal-close-confirm-popover"
      onOpenChange={onOpenChange}
      onConfirm={onConfirm}
    />
  );
}
