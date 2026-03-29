"use client";

import React, { useState, useCallback, useRef, useEffect } from "react";
import { Input } from "@kandev/ui/input";
import { IconPencil, IconTrash } from "@tabler/icons-react";
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
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
  ContextMenuSeparator,
} from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileInfo } from "@/lib/state/store";
import {
  removeNodeFromTree,
  renameNodeInTree,
  treeContainsPath,
  countFilesInTree,
} from "./file-tree-utils";

type GitFileStatus = FileInfo["status"] | undefined;

function deleteNodeOptimistically(
  tree: FileTreeNode | null,
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>,
  path: string,
  onDeleteFile: (path: string) => Promise<boolean>,
) {
  const snapshot = tree;
  setTree((prev) => (prev ? removeNodeFromTree(prev, path) : prev));
  onDeleteFile(path)
    .then((ok) => {
      if (!ok) setTree(snapshot);
    })
    .catch(() => setTree(snapshot));
}

function DeleteConfirmDialog({
  isBulk,
  selectedCount,
  node,
  fileCount,
  onConfirm,
}: {
  isBulk: boolean;
  selectedCount: number;
  node: FileTreeNode;
  fileCount: number;
  onConfirm: () => void;
}) {
  const title = isBulk ? `Delete ${selectedCount} items?` : "Delete folder?";
  return (
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{title}</AlertDialogTitle>
        <AlertDialogDescription>
          {isBulk ? (
            `This will permanently delete ${selectedCount} selected items. This action cannot be undone.`
          ) : (
            <>
              This will permanently delete{" "}
              <span className="font-semibold">{node.name}</span> and{" "}
              <span className="font-semibold">{fileCount}</span>{" "}
              {fileCount === 1 ? "file" : "files"} inside it. This action cannot be undone.
            </>
          )}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
        <AlertDialogAction onClick={onConfirm} variant="destructive" className="cursor-pointer">
          Delete
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  );
}

/** Context menu for file nodes with Rename and Delete options */
export function FileContextMenu({
  children,
  node,
  tree,
  setTree,
  onDeleteFile,
  onRenameFile,
  onStartRename,
  selectedCount = 0,
}: {
  children: React.ReactNode;
  node: FileTreeNode;
  tree: FileTreeNode | null;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onStartRename: () => void;
  selectedCount?: number;
}) {
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const isBulk = selectedCount > 1;
  const needsConfirmation = node.is_dir || isBulk;

  const handleConfirmDelete = useCallback(() => {
    setDeleteDialogOpen(false);
    if (onDeleteFile) deleteNodeOptimistically(tree, setTree, node.path, onDeleteFile);
  }, [tree, setTree, node.path, onDeleteFile]);

  const handleDelete = useCallback(() => {
    if (!onDeleteFile) return;
    if (needsConfirmation) {
      setDeleteDialogOpen(true);
    } else {
      deleteNodeOptimistically(tree, setTree, node.path, onDeleteFile);
    }
  }, [tree, setTree, node.path, onDeleteFile, needsConfirmation]);

  if (!onDeleteFile && !onRenameFile) return <>{children}</>;

  const deleteLabel = isBulk ? `Delete ${selectedCount} items` : "Delete";

  return (
    <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
      <ContextMenu>
        <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
        <ContextMenuContent>
          {onRenameFile && !isBulk && (
            <ContextMenuItem onSelect={onStartRename}>
              <IconPencil className="h-3.5 w-3.5" />
              Rename
            </ContextMenuItem>
          )}
          {onRenameFile && onDeleteFile && !isBulk && <ContextMenuSeparator />}
          {onDeleteFile && (
            <ContextMenuItem variant="destructive" onSelect={handleDelete}>
              <IconTrash className="h-3.5 w-3.5" />
              {deleteLabel}
            </ContextMenuItem>
          )}
        </ContextMenuContent>
      </ContextMenu>
      {needsConfirmation && (
        <DeleteConfirmDialog
          isBulk={isBulk}
          selectedCount={selectedCount}
          node={node}
          fileCount={countFilesInTree(node)}
          onConfirm={handleConfirmDelete}
        />
      )}
    </AlertDialog>
  );
}

/** Hook for managing inline file rename state */
export function useFileRename(
  node: FileTreeNode,
  tree: FileTreeNode | null,
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>,
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>,
) {
  const { toast } = useToast();
  const [isRenaming, setIsRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState(node.name);

  const handleStartRename = useCallback(() => {
    setRenameValue(node.name);
    setIsRenaming(true);
  }, [node.name]);

  const handleCancelRename = useCallback(() => {
    setIsRenaming(false);
    setRenameValue(node.name);
  }, [node.name]);

  const handleConfirmRename = useCallback(() => {
    const newName = renameValue.trim();
    if (!newName || newName === node.name || !onRenameFile) {
      handleCancelRename();
      return;
    }
    if (newName.includes("/") || newName.includes("\\")) {
      toast({
        title: "Invalid name",
        description: "File names cannot contain path separators",
        variant: "error",
      });
      handleCancelRename();
      return;
    }
    const parentPath = node.path.includes("/")
      ? node.path.substring(0, node.path.lastIndexOf("/"))
      : "";
    const newPath = parentPath ? `${parentPath}/${newName}` : newName;
    const snapshot = tree;
    setIsRenaming(false);
    if (tree && treeContainsPath(tree, newPath)) {
      toast({
        title: "Failed to rename item",
        description: `Target already exists: ${newPath}`,
        variant: "error",
      });
      handleCancelRename();
      return;
    }
    setTree((prev) => (prev ? renameNodeInTree(prev, node.path, newPath) : prev));
    onRenameFile(node.path, newPath)
      .then((ok) => {
        if (!ok) setTree(snapshot);
      })
      .catch(() => {
        setTree(snapshot);
      });
  }, [renameValue, node.name, node.path, onRenameFile, tree, setTree, handleCancelRename, toast]);

  const handleRenameKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        handleConfirmRename();
      } else if (e.key === "Escape") {
        handleCancelRename();
      }
    },
    [handleConfirmRename, handleCancelRename],
  );

  return {
    isRenaming,
    renameValue,
    setRenameValue,
    handleStartRename,
    handleConfirmRename,
    handleRenameKeyDown,
  };
}

/** Inline rename input or static file name */
export function TreeNodeName({
  node,
  isActive,
  gitStatus,
  rename,
}: {
  node: FileTreeNode;
  isActive: boolean;
  gitStatus: GitFileStatus;
  rename: ReturnType<typeof useFileRename>;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const blurEnabledRef = useRef(false);

  useEffect(() => {
    if (rename.isRenaming) {
      blurEnabledRef.current = false;
      const focusTimer = setTimeout(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      }, 150);
      const blurTimer = setTimeout(() => {
        blurEnabledRef.current = true;
      }, 400);
      return () => {
        clearTimeout(focusTimer);
        clearTimeout(blurTimer);
      };
    }
  }, [rename.isRenaming]);

  const handleBlur = useCallback(() => {
    if (blurEnabledRef.current) {
      rename.handleConfirmRename();
    }
  }, [rename]);

  if (rename.isRenaming) {
    return (
      <Input
        ref={inputRef}
        value={rename.renameValue}
        onChange={(e) => rename.setRenameValue(e.target.value)}
        onKeyDown={rename.handleRenameKeyDown}
        onBlur={handleBlur}
        onClick={(e) => e.stopPropagation()}
        className="h-5 text-xs px-1 py-0 flex-1 min-w-0"
      />
    );
  }
  return (
    <span
      className={cn(
        "flex-1 truncate group-hover:text-foreground",
        isActive ? "text-foreground" : "text-muted-foreground",
        node.is_dir ? "font-medium" : getGitStatusTextClass(gitStatus),
      )}
    >
      {node.name}
    </span>
  );
}

export function getGitStatusTextClass(status: GitFileStatus): string {
  switch (status) {
    case "added":
    case "untracked":
      return "text-green-700 dark:text-green-600";
    case "modified":
      return "text-yellow-600";
    default:
      return "";
  }
}
