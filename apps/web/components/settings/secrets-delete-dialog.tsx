"use client";

import type { RefObject } from "react";
import type { TFunction } from "i18next";
import { Trans, useTranslation } from "react-i18next";
import { IconAlertTriangle, IconGitBranch, IconRobot, IconServer } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
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
        className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-lg"
      >
        <AlertDialogHeader className="border-b px-5 py-4 text-left sm:px-6 sm:text-left">
          <div className="flex items-center gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-destructive/10 text-destructive">
              <IconAlertTriangle className="size-5" aria-hidden="true" />
            </span>
            <AlertDialogTitle>{t("settings:cannotDeleteSecret")}</AlertDialogTitle>
          </div>
        </AlertDialogHeader>
        <AlertDialogDescription asChild>
          <div className="flex min-h-0 min-w-0 flex-col gap-3 overflow-hidden px-5 py-4 text-left sm:px-6">
            <p className="leading-6">{t("settings:secretInUseUnknown")}</p>
            <ul
              data-testid="secret-delete-reference-list"
              className="min-h-0 space-y-2 overflow-x-hidden overflow-y-auto overscroll-contain pr-1 [scrollbar-gutter:stable]"
            >
              {references.map((reference, index) => (
                <SecretReferenceCard
                  key={`${reference.kind}:${reference.id ?? "hidden"}:${reference.key ?? index}`}
                  reference={reference}
                />
              ))}
            </ul>
          </div>
        </AlertDialogDescription>
        <AlertDialogFooter className="border-t bg-muted/20 px-5 py-4 sm:px-6">
          <AlertDialogCancel className="min-h-12 w-full cursor-pointer sm:min-h-9 sm:w-auto">
            {t("common:close")}
          </AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function SecretReferenceCard({ reference }: { reference: SecretReference }) {
  const { t } = useTranslation();
  const detailsVisible = Boolean(reference.name && reference.key);
  const name = detailsVisible ? reference.name : secretReferenceLabel(reference, t);

  return (
    <li
      data-testid="secret-delete-reference"
      className="flex min-w-0 items-start gap-3 rounded-lg border bg-muted/30 p-3 text-foreground shadow-sm"
    >
      <SecretReferenceIcon kind={reference.kind} />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="min-w-0 text-sm font-medium leading-5 [overflow-wrap:anywhere]">
            {name}
          </span>
          {detailsVisible ? (
            <Badge variant="secondary" className="shrink-0 text-[10px] font-medium">
              {secretReferenceTypeLabel(reference.kind, t)}
            </Badge>
          ) : null}
        </div>
        {detailsVisible ? (
          <code className="mt-1 block w-fit max-w-full rounded bg-background px-1.5 py-0.5 text-xs text-muted-foreground [overflow-wrap:anywhere]">
            {reference.key}
          </code>
        ) : null}
      </div>
    </li>
  );
}

function SecretReferenceIcon({ kind }: { kind: SecretReference["kind"] }) {
  let Icon = IconGitBranch;
  if (kind === "agent_profile") Icon = IconRobot;
  if (kind === "executor_profile") Icon = IconServer;
  return (
    <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground">
      <Icon className="size-4" aria-hidden="true" />
    </span>
  );
}

function secretReferenceTypeLabel(kind: SecretReference["kind"], t: TFunction) {
  switch (kind) {
    case "agent_profile":
      return t("settings:secretReferenceTypeAgent");
    case "executor_profile":
      return t("settings:secretReferenceTypeExecutor");
    case "repository":
      return t("settings:secretReferenceTypeRepository");
  }
}
