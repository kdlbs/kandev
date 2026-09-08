"use client";

import type { RefObject } from "react";
import { Trans, useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";

import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import type { SecretListItem, SecretReference } from "@/lib/types/http-secrets";
import { secretReferenceLabel } from "./secret-delete-error";

type SecretDeleteConfirmationProps = {
  secret: SecretListItem;
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  onOpenChange: (open: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
  loading?: boolean;
};

/** Anchors secret deletion confirmation to its row action on fine pointers. */
export function SecretDeleteConfirmation({
  secret,
  open,
  anchorRef,
  onOpenChange,
  onCancel,
  onConfirm,
  loading = false,
}: SecretDeleteConfirmationProps) {
  const { t } = useTranslation();

  return (
    <ActionConfirmPopover
      open={open}
      anchorRef={anchorRef}
      title={t("settings:deleteSecret")}
      description={
        loading ? (
          t("settings:checkingSecretReferences")
        ) : (
          <Trans i18nKey="settings:thisWillPermanentlyRemoveSecret" values={{ name: secret.name }}>
            This will permanently remove{" "}
            <span className="font-medium text-foreground">{secret.name}</span>. This action cannot
            be undone.
          </Trans>
        )
      }
      cancelLabel={t("settings:cancel")}
      confirmLabel={t("settings:deleteSecret")}
      confirmAriaLabel={t("settings:deleteSecretNamed", { name: secret.name })}
      confirmTestId="secret-delete-confirm"
      confirmDisabled={loading}
      testId="secret-delete-confirm-popover"
      onOpenChange={onOpenChange}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}

type SecretDeleteConflictDialogProps = {
  secret: SecretListItem | null;
  references: SecretReference[];
  onClose: () => void;
};

/** Lists every visible blocker before an in-use secret can be deleted. */
export function SecretDeleteConflictDialog({
  secret,
  references,
  onClose,
}: SecretDeleteConflictDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={secret !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent
        data-testid="secret-delete-conflict-dialog"
        data-layout="contained"
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
      >
        <AlertDialogHeader>
          <AlertDialogTitle>{t("settings:cannotDeleteSecret")}</AlertDialogTitle>
        </AlertDialogHeader>
        <AlertDialogDescription asChild>
          <div className="min-h-0 min-w-0 space-y-3 overflow-x-hidden overflow-y-auto overscroll-contain text-left">
            <p>{t("settings:secretInUseUnknown")}</p>
            <ul className="list-inside list-disc space-y-1">
              {references.map((reference, index) => (
                <li
                  key={`${reference.kind}:${reference.id ?? "hidden"}:${reference.key ?? index}`}
                  className="text-sm [overflow-wrap:anywhere]"
                >
                  {secretReferenceLabel(reference, t)}
                </li>
              ))}
            </ul>
          </div>
        </AlertDialogDescription>
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11 w-full cursor-pointer sm:min-h-9 sm:w-auto">
            {t("common:close")}
          </AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
