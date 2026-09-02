"use client";

import { IconDownload, IconPencil, IconTrash, IconUpload } from "@tabler/icons-react";
import { ContextMenuItem, ContextMenuSeparator } from "@kandev/ui/context-menu";
import { useTranslation } from "react-i18next";
import type { FileTreeNode } from "@/lib/types/backend";

export type FileContextMenuItemsProps = {
  node: FileTreeNode;
  isBulk: boolean;
  selectedCount: number;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  onUploadFilesHere?: (path: string) => void;
  onStartRename: () => void;
  onDelete: (event: Event) => void;
};

/**
 * Which node-scoped actions apply, resolved once so the render stays flat.
 *
 * Download and upload are exact inverses: a file can be downloaded, a folder can
 * receive an upload, and a multi-selection supports neither because it has no
 * single subject or destination.
 */
function resolveActions({
  node,
  isBulk,
  onRenameFile,
  onDownloadFile,
  onUploadFilesHere,
}: FileContextMenuItemsProps) {
  return {
    showRename: !!onRenameFile && !isBulk,
    download: !node.is_dir && !isBulk ? onDownloadFile : undefined,
    upload: node.is_dir && !isBulk ? onUploadFilesHere : undefined,
  };
}

export function FileContextMenuItems(props: FileContextMenuItemsProps) {
  const { node, isBulk, selectedCount, onDeleteFile, onStartRename, onDelete } = props;
  const { t } = useTranslation();
  const { showRename, download, upload } = resolveActions(props);
  const deleteLabel = isBulk
    ? t("task:deleteItemsLabel", { count: selectedCount })
    : t("task:delete");
  const hasLeadingItem = showRename || !!onDeleteFile;

  return (
    <>
      {onDeleteFile && (
        <ContextMenuItem variant="destructive" onSelect={onDelete}>
          <IconTrash className="h-3.5 w-3.5" />
          {deleteLabel}
        </ContextMenuItem>
      )}
      {showRename && onDeleteFile && <ContextMenuSeparator />}
      {showRename && (
        <ContextMenuItem onSelect={onStartRename}>
          <IconPencil className="h-3.5 w-3.5" />
          {t("task:rename")}
        </ContextMenuItem>
      )}
      {download && hasLeadingItem && <ContextMenuSeparator />}
      {download && (
        <ContextMenuItem onSelect={() => void download(node.path)}>
          <IconDownload className="h-3.5 w-3.5" />
          {t("task:download")}
        </ContextMenuItem>
      )}
      {upload && hasLeadingItem && <ContextMenuSeparator />}
      {upload && (
        <ContextMenuItem onSelect={() => upload(node.path)}>
          <IconUpload className="h-3.5 w-3.5" />
          {t("task:uploadFilesHere")}
        </ContextMenuItem>
      )}
    </>
  );
}
