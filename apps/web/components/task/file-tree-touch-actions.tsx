"use client";

import React from "react";
import { IconCopy, IconDotsVertical, IconTrash } from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import { useCopyRepositoryPath } from "@/hooks/use-copy-repository-path";
import type { FileTreeNode } from "@/lib/types/backend";
import { useFileDeleteAction } from "./file-context-menu";

export function FileTreeNodeTouchActions({
  node,
  showTouchActions,
  onAddToChatContext,
  selectedCount = 0,
  isRepositoryRoot = false,
}: {
  node: FileTreeNode;
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
  selectedCount?: number;
  isRepositoryRoot?: boolean;
}) {
  const { t } = useTranslation();
  const copyPath = useCopyRepositoryPath();
  const deleteAction = useFileDeleteAction();
  const deletePendingRef = React.useRef(false);
  const isBulk = selectedCount > 1;
  if (!showTouchActions || (isBulk && !onAddToChatContext && !deleteAction)) return null;

  const stopRowInteraction = (event: React.SyntheticEvent) => event.stopPropagation();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          ref={deleteAction?.triggerRef}
          type="button"
          data-testid="file-tree-node-actions"
          data-path={node.path}
          aria-label={t("chat:fileTreeActions")}
          className="absolute top-1/2 right-0 z-10 flex min-h-11 min-w-11 -translate-y-1/2 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer"
          onPointerDown={stopRowInteraction}
          onMouseDown={stopRowInteraction}
          onClick={stopRowInteraction}
          onKeyDown={stopRowInteraction}
        >
          <IconDotsVertical className="h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        data-testid="file-tree-touch-menu"
        onClick={stopRowInteraction}
        onCloseAutoFocus={(event) => {
          if (!deletePendingRef.current) return;
          event.preventDefault();
          deletePendingRef.current = false;
          deleteAction?.onDelete();
        }}
      >
        {!isBulk && (
          <DropdownMenuItem
            className="min-h-11 cursor-pointer"
            onSelect={() => void copyPath(node.path)}
          >
            <IconCopy className="h-3.5 w-3.5" />
            {t("task:copyPath")}
          </DropdownMenuItem>
        )}
        {onAddToChatContext && (
          <DropdownMenuItem
            data-testid="file-tree-touch-add-to-chat"
            className="min-h-11 cursor-pointer"
            onSelect={() => onAddToChatContext(node)}
          >
            {t("chat:addToChatContext")}
          </DropdownMenuItem>
        )}
        {deleteAction && !isRepositoryRoot && (
          <DropdownMenuItem
            data-testid="file-tree-touch-delete"
            variant="destructive"
            className="min-h-11 cursor-pointer"
            onSelect={() => {
              if (!deleteAction.isBulk) deletePendingRef.current = true;
              else deleteAction.onDelete();
            }}
          >
            <IconTrash className="h-3.5 w-3.5" />
            {deleteAction.isBulk
              ? t("task:deleteItemsLabel", { count: deleteAction.selectedCount })
              : t("task:delete")}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
