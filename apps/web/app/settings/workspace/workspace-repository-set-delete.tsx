"use client";

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

import type { RepositorySet } from "@/lib/types/http";

type RepositorySetDeleteDialogProps = {
  set: RepositorySet | null;
  /** Non-null when the last delete attempt failed; keeps the dialog open. */
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
};

/**
 * Confirms deleting a set. The copy states plainly that no repository is
 * affected, because "delete set" next to a list of repository names reads as
 * though it might remove them.
 */
export function RepositorySetDeleteDialog({
  set,
  error,
  onClose,
  onConfirm,
}: RepositorySetDeleteDialogProps) {
  const { t } = useTranslation();
  if (!set) return null;

  return (
    <AlertDialog open onOpenChange={(next) => (next ? undefined : onClose())}>
      <AlertDialogContent data-testid="repository-set-delete-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("workspaces:repositorySetsDeleteTitle", { name: set.name })}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("workspaces:repositorySetsDeleteDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error ? (
          <p className="text-xs text-destructive" data-testid="repository-set-delete-error">
            {error}
          </p>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer"
            onClick={onConfirm}
            data-testid="repository-set-delete-confirm"
          >
            {t("workspaces:repositorySetsDelete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
