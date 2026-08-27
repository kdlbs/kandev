"use client";

import { useEffect, useState } from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Input } from "@kandev/ui/input";
import { Trans, useTranslation } from "react-i18next";
import type { StorageQuarantineEntry, StorageQuarantinePurgeScope } from "@/lib/types/system";

/**
 * `phrase` is a sentinel, never copy: the user must type it verbatim and the
 * confirm button gates on `confirmation !== props.phrase`. Translating one of
 * these would make the dialog impossible to satisfy in that locale — so the
 * union stays English and travels as an interpolated value everywhere it is
 * shown (see docs/i18n.md, "Do not translate").
 */
type ConfirmationPhrase = "DEDICATED" | "ADOPT" | "DELETE" | "DELETE ELIGIBLE" | "DELETE ALL NOW";

type ConfirmationDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  phrase: ConfirmationPhrase;
  actionLabel: string;
  actionTestId: string;
  destructive?: boolean;
  onConfirm: () => void;
};

function ConfirmationDialog(props: ConfirmationDialogProps) {
  const { t } = useTranslation();
  const [confirmation, setConfirmation] = useState("");
  useEffect(() => {
    if (!props.open) setConfirmation("");
  }, [props.open]);
  return (
    <AlertDialog open={props.open} onOpenChange={props.onOpenChange}>
      <AlertDialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>{props.title}</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            {props.description}{" "}
            <Trans i18nKey="system:storageTypeToConfirm" values={{ phrase: props.phrase }}>
              Type <strong>{props.phrase}</strong> to continue.
            </Trans>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Input
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
          className="h-11"
          aria-label={t("system:storageTypeToConfirmAria", { phrase: props.phrase })}
          data-testid={`${props.actionTestId}-confirmation`}
        />
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11 cursor-pointer">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            variant={props.destructive ? "destructive" : "default"}
            disabled={confirmation !== props.phrase}
            onClick={props.onConfirm}
            className="min-h-11 cursor-pointer"
            data-testid={props.actionTestId}
          >
            {props.actionLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function DedicatedDockerDialog(
  props: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm">,
) {
  const { t } = useTranslation();
  return (
    <ConfirmationDialog
      {...props}
      title={t("system:storageDedicatedDialogTitle")}
      description={t("system:storageDedicatedDialogDescription")}
      phrase="DEDICATED"
      actionLabel={t("system:storageAcknowledgeDaemon")}
      actionTestId="storage-docker-confirm"
    />
  );
}

export function ExternalGoCacheDialog({
  path,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & { path: string }) {
  const { t } = useTranslation();
  return (
    <ConfirmationDialog
      {...props}
      title={t("system:storageAdoptDialogTitle")}
      description={t("system:storageAdoptDialogDescription", {
        // A filesystem path the user typed — interpolated, never translated.
        path: path || t("system:storageAdoptDialogFallbackPath"),
      })}
      phrase="ADOPT"
      actionLabel={t("system:storageAdoptCache")}
      actionTestId="storage-go-cache-adopt-confirm"
    />
  );
}

export function TemporaryArtifactsDialog({
  open,
  onOpenChange,
  onConfirm,
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm">) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("system:storageTemporaryArtifactsDialogTitle")}</AlertDialogTitle>
          <AlertDialogDescription className="text-left">
            {t("system:storageTemporaryArtifactsDialogDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11 cursor-pointer">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="min-h-11 cursor-pointer"
            data-testid="storage-temporary-artifacts-confirm"
          >
            {t("system:storageTemporaryArtifactsDialogAction")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function PermanentDeleteDialog({
  entry,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & {
  entry: StorageQuarantineEntry | null;
}) {
  const { t } = useTranslation();
  return (
    <ConfirmationDialog
      {...props}
      title={t("system:storageDeleteDialogTitle")}
      description={t("system:storageDeleteDialogDescription", {
        // The quarantine path comes from the API — interpolated, never translated.
        path: entry?.quarantine_path ?? t("system:storageDeleteDialogFallbackPath"),
      })}
      phrase="DELETE"
      actionLabel={t("system:storageDeletePermanently")}
      actionTestId="storage-quarantine-delete-confirm"
      destructive
    />
  );
}

export function QuarantinePurgeDialog({
  scope,
  eligibleCount,
  protectedCount,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & {
  scope: StorageQuarantinePurgeScope;
  eligibleCount: number;
  protectedCount: number;
}) {
  const { t } = useTranslation();
  const eligible = scope === "eligible";
  // Two independent counts cannot share one `count`, so this is two plural
  // messages joined — not one message with a hand-written English `s`.
  const eligibleDescription = `${t("system:storagePurgeEligibleCount", { count: eligibleCount })} ${t("system:storagePurgeProtectedCount", { count: protectedCount })}`;
  return (
    <ConfirmationDialog
      {...props}
      title={
        eligible
          ? t("system:storageClearEligibleDialogTitle")
          : t("system:storageForceClearDialogTitle")
      }
      description={eligible ? eligibleDescription : t("system:storageForceClearDescription")}
      phrase={eligible ? "DELETE ELIGIBLE" : "DELETE ALL NOW"}
      actionLabel={eligible ? t("system:storageClearEligible") : t("system:storageForceClearAll")}
      actionTestId={
        eligible
          ? "storage-quarantine-clear-eligible-confirm"
          : "storage-quarantine-force-clear-confirm"
      }
      destructive
    />
  );
}
