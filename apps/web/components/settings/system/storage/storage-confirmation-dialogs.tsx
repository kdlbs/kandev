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
import type { StorageQuarantineEntry, StorageQuarantinePurgeScope } from "@/lib/types/system";

type ConfirmationDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  phrase: "DEDICATED" | "ADOPT" | "DELETE" | "DELETE ELIGIBLE" | "DELETE ALL NOW";
  actionLabel: string;
  actionTestId: string;
  destructive?: boolean;
  onConfirm: () => void;
};

function ConfirmationDialog(props: ConfirmationDialogProps) {
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
            {props.description} Type <strong>{props.phrase}</strong> to continue.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Input
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
          className="h-11"
          aria-label={`Type ${props.phrase} to confirm`}
          data-testid={`${props.actionTestId}-confirmation`}
        />
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11 cursor-pointer">Cancel</AlertDialogCancel>
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
  return (
    <ConfirmationDialog
      {...props}
      title="Use this dedicated Docker daemon"
      description="Build-cache and unused-image cleanup affect the entire configured daemon, including resources created outside Kandev. Only acknowledge a daemon dedicated to this installation."
      phrase="DEDICATED"
      actionLabel="Acknowledge daemon"
      actionTestId="storage-docker-confirm"
    />
  );
}

export function ExternalGoCacheDialog({
  path,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & { path: string }) {
  return (
    <ConfirmationDialog
      {...props}
      title="Adopt an external Go build cache"
      description={`Kandev will be allowed to rotate and quarantine the existing cache at ${path || "the selected path"}. This path must be absolute and on the same filesystem as Kandev trash.`}
      phrase="ADOPT"
      actionLabel="Adopt cache"
      actionTestId="storage-go-cache-adopt-confirm"
    />
  );
}

export function PermanentDeleteDialog({
  entry,
  ...props
}: Pick<ConfirmationDialogProps, "open" | "onOpenChange" | "onConfirm"> & {
  entry: StorageQuarantineEntry | null;
}) {
  return (
    <ConfirmationDialog
      {...props}
      title="Permanently delete quarantined data"
      description={`This cannot be undone. Kandev will permanently remove ${entry?.quarantine_path ?? "the selected quarantine entry"}.`}
      phrase="DELETE"
      actionLabel="Delete permanently"
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
  const eligible = scope === "eligible";
  return (
    <ConfirmationDialog
      {...props}
      title={eligible ? "Clear eligible quarantine" : "Force clear all quarantine"}
      description={
        eligible
          ? `This will permanently remove ${eligibleCount} eligible item${eligibleCount === 1 ? "" : "s"}. ${protectedCount} protected item${protectedCount === 1 ? " remains" : "s remain"} until ${protectedCount === 1 ? "its" : "their"} retention deadline${protectedCount === 1 ? "" : "s"}.`
          : "This attempts to permanently remove every quarantined item immediately and cannot be undone for entries that succeed. Failed entries may remain visible and retryable."
      }
      phrase={eligible ? "DELETE ELIGIBLE" : "DELETE ALL NOW"}
      actionLabel={eligible ? "Clear eligible" : "Force clear all"}
      actionTestId={
        eligible
          ? "storage-quarantine-clear-eligible-confirm"
          : "storage-quarantine-force-clear-confirm"
      }
      destructive
    />
  );
}
