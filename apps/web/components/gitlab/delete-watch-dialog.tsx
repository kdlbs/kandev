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
import { useTranslation } from "react-i18next";

export function DeleteWatchDialog({
  open,
  onOpenChange,
  watchLabel,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watchLabel: string;
  onConfirm: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (open) setError("");
  }, [open]);
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          {/* One message, not "Delete" + label: the label's position in the
              sentence is the translator's to choose. */}
          <AlertDialogTitle>{t("gitlab:deleteWatchTitle", { label: watchLabel })}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("gitlab:thisWillDeleteEveryTaskCreated")}
          </AlertDialogDescription>
          {error && (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          )}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleting} className="cursor-pointer">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={deleting}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={async (event) => {
              event.preventDefault();
              setDeleting(true);
              setError("");
              try {
                await onConfirm();
                onOpenChange(false);
              } catch (cause) {
                setError(cause instanceof Error ? cause.message : t("gitlab:watchDeletionFailed"));
              } finally {
                setDeleting(false);
              }
            }}
          >
            {deleting ? t("gitlab:deleting") : t("gitlab:deleteWatch")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
