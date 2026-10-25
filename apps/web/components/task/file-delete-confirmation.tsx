"use client";

import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Trans, useTranslation } from "react-i18next";

export function DeleteFileDescription({ name }: { name: string }) {
  return (
    <Trans i18nKey="task:deleteFileConfirm" values={{ name }}>
      This will permanently delete <span className="font-semibold">{name}</span>. This action cannot
      be undone.
    </Trans>
  );
}

export function DeleteFolderDescription({ name, fileCount }: { name: string; fileCount: number }) {
  if (fileCount > 0) {
    return (
      <Trans
        i18nKey="task:deleteFolderWithFilesConfirm"
        count={fileCount}
        values={{ name, count: fileCount }}
      >
        This will permanently delete <span className="font-semibold">{name}</span> and{" "}
        <span className="font-semibold">{fileCount}</span> files inside it. This action cannot be
        undone.
      </Trans>
    );
  }
  return (
    <Trans i18nKey="task:deleteFolderConfirm" values={{ name }}>
      This will permanently delete <span className="font-semibold">{name}</span>. This action cannot
      be undone.
    </Trans>
  );
}

export function DeleteConfirmDialog({
  selectedCount,
  onConfirm,
}: {
  selectedCount: number;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{t("task:deleteItemsTitle", { count: selectedCount })}</AlertDialogTitle>
        <AlertDialogDescription>
          {t("task:thisWillPermanentlyDeleteSelectedItems", { selectedCount })}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
        <AlertDialogAction onClick={onConfirm} variant="destructive" className="cursor-pointer">
          {t("task:delete")}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  );
}
