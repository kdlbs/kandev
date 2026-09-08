"use client";

import type { RefObject } from "react";
import { Trans, useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import type { PluginRecord } from "@/lib/types/plugins";

type UninstallPluginConfirmationProps = {
  target: PluginRecord;
  open: boolean;
  isFinePointer: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

/**
 * Keeps plugin uninstall confirmation at its initiating control. The action
 * hook still owns the target mutation, pending state, and failure feedback.
 */
export function PluginUninstallConfirmation({
  target,
  open,
  isFinePointer,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
}: UninstallPluginConfirmationProps) {
  const { t } = useTranslation();
  const description = (
    <Trans i18nKey="plugins:uninstallConfirm" values={{ name: target.display_name }}>
      This will permanently remove{" "}
      <span className="font-medium text-foreground">{target.display_name}</span> and revoke its API
      key. This action cannot be undone.
    </Trans>
  );
  const confirmAriaLabel = t("plugins:confirmUninstall");

  if (!isFinePointer) {
    if (!open) return null;
    return (
      <InlineConfirmActions
        density="touch"
        testId="plugin-uninstall-inline-confirmation"
        ariaLabel={t("plugins:uninstallPlugin")}
        description={description}
        cancelLabel={t("plugins:cancel")}
        confirmLabel={t("plugins:confirmUninstall")}
        confirmAriaLabel={confirmAriaLabel}
        confirmTestId="plugin-uninstall-confirm"
        onCancel={onCancel}
        onClose={() => onOpenChange(false)}
        onConfirm={onConfirm}
      />
    );
  }

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      title={t("plugins:uninstallPlugin")}
      description={description}
      cancelLabel={t("plugins:cancel")}
      confirmLabel={t("plugins:confirmUninstall")}
      confirmAriaLabel={confirmAriaLabel}
      confirmTestId="plugin-uninstall-confirm"
      testId="plugin-uninstall-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}
