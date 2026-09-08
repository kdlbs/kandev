import type React from "react";
import type { TFunction } from "i18next";
import type { FileTreeNode } from "@/lib/types/backend";
import { countFilesInTree } from "./file-tree-utils";
import { DeleteFileDescription, DeleteFolderDescription } from "./file-delete-confirmation";

export type FileDeleteAction = {
  isBulk: boolean;
  selectedCount: number;
  confirming: boolean;
  title: string;
  label: string;
  cancelLabel: string;
  description: React.ReactNode;
  onDelete: () => void;
  onCancel: () => void;
  onConfirm: () => void;
};

export function createFileDeleteAction({
  node,
  onDeleteFile,
  selectedCount,
  isBulk,
  deleteConfirmationOpen,
  t,
  handleDelete,
  setDeleteConfirmationOpen,
  handleConfirmDelete,
}: {
  node: FileTreeNode;
  onDeleteFile?: (path: string) => Promise<boolean>;
  selectedCount: number;
  isBulk: boolean;
  deleteConfirmationOpen: boolean;
  t: TFunction;
  handleDelete: () => void;
  setDeleteConfirmationOpen: (open: boolean) => void;
  handleConfirmDelete: () => void;
}): FileDeleteAction | null {
  if (!onDeleteFile) return null;
  return {
    isBulk,
    selectedCount,
    confirming: deleteConfirmationOpen,
    title: isBulk
      ? t("task:deleteItemsTitle", { count: selectedCount })
      : t("task:delete2", { name: node.name }),
    label: isBulk ? t("task:deleteItemsLabel", { count: selectedCount }) : t("task:delete"),
    cancelLabel: t("common:cancel"),
    description: node.is_dir ? (
      <DeleteFolderDescription name={node.name} fileCount={countFilesInTree(node)} />
    ) : (
      <DeleteFileDescription name={node.name} />
    ),
    onDelete: handleDelete,
    onCancel: () => setDeleteConfirmationOpen(false),
    onConfirm: handleConfirmDelete,
  };
}
