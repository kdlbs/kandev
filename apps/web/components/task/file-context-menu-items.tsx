"use client";

import { IconCopy, IconDownload, IconPencil, IconTrash, IconUpload } from "@tabler/icons-react";
import { ContextMenuItem, ContextMenuSeparator } from "@kandev/ui/context-menu";
import { useTranslation } from "react-i18next";
import type { FileTreeNode } from "@/lib/types/backend";
import { useCopyRepositoryPath } from "@/hooks/use-copy-repository-path";

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
  onDeleteFile,
  onRenameFile,
  onDownloadFile,
  onUploadFilesHere,
}: FileContextMenuItemsProps) {
  const showRename = !!onRenameFile && !isBulk;
  const download = !node.is_dir && !isBulk ? onDownloadFile : undefined;
  const upload = node.is_dir && !isBulk ? onUploadFilesHere : undefined;
  return {
    showRename,
    download,
    upload,
    hasFollowingItem: !!onDeleteFile || showRename || !!download || !!upload,
  };
}

function CopyPathItem({ path, hasFollowingItem }: { path: string; hasFollowingItem: boolean }) {
  const { t } = useTranslation();
  const copyPath = useCopyRepositoryPath();
  return (
    <>
      <ContextMenuItem onSelect={() => void copyPath(path)}>
        <IconCopy className="h-3.5 w-3.5" />
        {t("task:copyPath")}
      </ContextMenuItem>
      {hasFollowingItem && <ContextMenuSeparator />}
    </>
  );
}

export function FileContextMenuItems(props: FileContextMenuItemsProps) {
  const { node, isBulk, selectedCount, onDeleteFile, onStartRename, onDelete } = props;
  const { t } = useTranslation();
  const { showRename, download, upload, hasFollowingItem } = resolveActions(props);
  const deleteLabel = isBulk
    ? t("task:deleteItemsLabel", { count: selectedCount })
    : t("task:delete");
  const hasLeadingItem = showRename || !!onDeleteFile;

  return (
    <>
      {!isBulk && <CopyPathItem path={node.path} hasFollowingItem={hasFollowingItem} />}
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
