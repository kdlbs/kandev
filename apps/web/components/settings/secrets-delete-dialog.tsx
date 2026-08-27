"use client";

import type { RefObject } from "react";
import { Trans, useTranslation } from "react-i18next";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import type { SecretListItem } from "@/lib/types/http-secrets";

type SecretDeleteConfirmationProps = {
  secret: SecretListItem;
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
};

/** Anchors secret deletion confirmation to its row action on fine pointers. */
export function SecretDeleteConfirmation({
  secret,
  open,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
}: SecretDeleteConfirmationProps) {
  const { t } = useTranslation();

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      title={t("settings:deleteSecret")}
      description={
        <Trans i18nKey="settings:thisWillPermanentlyRemoveSecret" values={{ name: secret.name }}>
          This will permanently remove{" "}
          <span className="font-medium text-foreground">{secret.name}</span>. This action cannot be
          undone.
        </Trans>
      }
      cancelLabel={t("settings:cancel")}
      confirmLabel={t("settings:deleteSecret")}
      confirmAriaLabel={t("settings:deleteSecretNamed", { name: secret.name })}
      confirmTestId="secret-delete-confirm"
      testId="secret-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}
