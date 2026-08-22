"use client";

import React, { useState, useCallback, useRef, useEffect } from "react";
import { Input } from "@kandev/ui/input";
import { IconDownload, IconMessageDots, IconPencil, IconTrash } from "@tabler/icons-react";
import { AlertDialog } from "@kandev/ui/alert-dialog";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
  ContextMenuSeparator,
} from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTranslation } from "react-i18next";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileInfo } from "@/lib/state/store";
import {
  removeNodeFromTree,
  renameNodeInTree,
  treeContainsPath,
  countFilesInTree,
} from "./file-tree-utils";
import {
  OpenInEditorMenuItems,
  canOpenNodeInEditor,
  useFileTreeEditorActions,
} from "./file-tree-editor-menu";
import {
  DeleteConfirmDialog,
  DeleteFileDescription,
  DeleteFolderDescription,
} from "./file-delete-confirmation";

type GitFileStatus = FileInfo["status"] | undefined;

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

const FileDeleteActionContext = React.createContext<FileDeleteAction | null>(null);

export function useFileDeleteAction() {
  return React.useContext(FileDeleteActionContext);
}

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

function removeSuccessfullyDeletedPaths(
  snapshot: FileTreeNode | null,
  paths: string[],
  results: PromiseSettledResult<boolean>[],
) {
  let nextTree = snapshot;
  results.forEach((result, index) => {
    if (result.status === "fulfilled" && result.value && nextTree) {
      nextTree = removeNodeFromTree(nextTree, paths[index]);
    }
  });
  return nextTree;
}

type FileContextMenuItemsProps = {
  node: FileTreeNode;
  isBulk: boolean;
  selectedCount: number;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  onStartRename: () => void;
  onDelete: (event: Event) => void;
};

function FileContextMenuItems({
  node,
  isBulk,
  selectedCount,
  onDeleteFile,
  onRenameFile,
  onDownloadFile,
  onStartRename,
  onDelete,
}: FileContextMenuItemsProps) {
  const { t } = useTranslation();
  const deleteLabel = isBulk
    ? t("task:deleteItemsLabel", { count: selectedCount })
    : t("task:delete");
  const showRename = !!onRenameFile && !isBulk;
  const download = !node.is_dir && !isBulk ? onDownloadFile : undefined;
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
      {download && (showRename || onDeleteFile) && <ContextMenuSeparator />}
      {download && (
        <ContextMenuItem onSelect={() => void download(node.path)}>
          <IconDownload className="h-3.5 w-3.5" />
          {t("task:download")}
        </ContextMenuItem>
      )}
    </>
  );
}

function ChatContextMenuItem({
  node,
  onAddToChatContext,
}: {
  node: FileTreeNode;
  onAddToChatContext: (node: FileTreeNode) => void;
}) {
  const { t } = useTranslation("chat");
  return (
    <ContextMenuItem
      data-testid="file-context-add-to-chat"
      className="cursor-pointer"
      onSelect={() => onAddToChatContext(node)}
    >
      <IconMessageDots className="h-3.5 w-3.5" />
      {t("chat:addToChatContext")}
    </ContextMenuItem>
  );
}

function attachAnchorRef(
  children: React.ReactNode,
  anchorRef: React.RefObject<HTMLElement | null>,
): React.ReactNode {
  if (!React.isValidElement(children)) {
    return <span ref={anchorRef}>{children}</span>;
  }
  return React.cloneElement(children as React.ReactElement<{ ref?: React.Ref<HTMLElement> }>, {
    ref: anchorRef,
  });
}

function useFileContextMenuDelete({
  tree,
  setTree,
  node,
  isBulk,
  onDeleteFile,
  selectedPaths,
}: {
  tree: FileTreeNode | null;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  node: FileTreeNode;
  isBulk: boolean;
  onDeleteFile?: (path: string) => Promise<boolean>;
  selectedPaths?: Set<string>;
}) {
  const [deleteConfirmationOpen, setDeleteConfirmationOpen] = useState(false);

  const handleConfirmDelete = useCallback(() => {
    setDeleteConfirmationOpen(false);
    if (!onDeleteFile) return;
    if (isBulk && selectedPaths) {
      const snapshot = tree;
      const paths = [...selectedPaths];
      setTree((prev) => {
        let nextTree = prev;
        for (const path of paths) {
          nextTree = nextTree ? removeNodeFromTree(nextTree, path) : nextTree;
        }
        return nextTree;
      });
      Promise.allSettled(paths.map((path) => onDeleteFile(path))).then((results) => {
        setTree(removeSuccessfullyDeletedPaths(snapshot, paths, results));
      });
      return;
    }
    deleteNodeOptimistically(tree, setTree, node.path, onDeleteFile);
  }, [tree, setTree, node.path, onDeleteFile, isBulk, selectedPaths]);

  const handleDelete = useCallback(() => {
    if (!onDeleteFile) return;
    setDeleteConfirmationOpen(true);
  }, [onDeleteFile]);

  return {
    deleteConfirmationOpen,
    setDeleteConfirmationOpen,
    handleConfirmDelete,
    handleDelete,
  };
}

function useFileMenuDeleteHandler({
  handleDelete,
  isBulk,
  isFinePointer,
  contextMenuRef,
}: {
  handleDelete: () => void;
  isBulk: boolean;
  isFinePointer: boolean;
  contextMenuRef: React.RefObject<HTMLElement | null>;
}) {
  return useCallback(
    (event: Event) => {
      const item = event.currentTarget;
      contextMenuRef.current =
        item instanceof HTMLElement
          ? (item.closest('[data-slot="context-menu-content"]') as HTMLElement | null)
          : null;
      if (isBulk || !isFinePointer) {
        handleDelete();
      } else {
        // Let the 100 ms context-menu exit animation finish before anchoring the popover.
        setTimeout(handleDelete, 150);
      }
    },
    [handleDelete, isBulk, isFinePointer, contextMenuRef],
  );
}

type FileContextMenuSurfaceProps = {
  children: React.ReactNode;
  node: FileTreeNode;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  onStartRename: () => void;
  onAddToChatContext?: (node: FileTreeNode) => void;
  selectedCount: number;
  isBulk: boolean;
  isFinePointer: boolean;
  showOpenInEditor: boolean;
  hasFileActions: boolean;
  showAddToChatContext: boolean;
  deleteConfirmationOpen: boolean;
  setDeleteConfirmationOpen: (open: boolean) => void;
  anchorRef: React.RefObject<HTMLElement | null>;
  focusBoundaryRef: React.RefObject<HTMLElement | null>;
  deleteAction: FileDeleteAction | null;
  onConfirmDelete: () => void;
  onDelete: (event: Event) => void;
};

function FileContextMenuSurface({
  children,
  node,
  onDeleteFile,
  onRenameFile,
  onDownloadFile,
  onStartRename,
  onAddToChatContext,
  selectedCount,
  isBulk,
  isFinePointer,
  showOpenInEditor,
  hasFileActions,
  showAddToChatContext,
  deleteConfirmationOpen,
  setDeleteConfirmationOpen,
  anchorRef,
  focusBoundaryRef,
  deleteAction,
  onConfirmDelete,
  onDelete,
}: FileContextMenuSurfaceProps) {
  const renamePendingRef = useRef(false);

  const handleStartRename = useCallback(() => {
    renamePendingRef.current = true;
  }, []);

  const handleCloseAutoFocus = useCallback(
    (event: Event) => {
      if (!renamePendingRef.current) return;
      event.preventDefault();
      renamePendingRef.current = false;
      onStartRename();
    },
    [onStartRename],
  );

  return (
    <FileDeleteActionContext.Provider value={deleteAction}>
      <ContextMenu>
        <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
        <ContextMenuContent onCloseAutoFocus={handleCloseAutoFocus}>
          {showOpenInEditor && <OpenInEditorMenuItems node={node} />}
          {showOpenInEditor && (hasFileActions || showAddToChatContext) && <ContextMenuSeparator />}
          {showAddToChatContext && onAddToChatContext && (
            <ChatContextMenuItem node={node} onAddToChatContext={onAddToChatContext} />
          )}
          {showAddToChatContext && hasFileActions && <ContextMenuSeparator />}
          <FileContextMenuItems
            node={node}
            isBulk={isBulk}
            selectedCount={selectedCount}
            onDeleteFile={onDeleteFile}
            onRenameFile={onRenameFile}
            onDownloadFile={onDownloadFile}
            onStartRename={handleStartRename}
            onDelete={onDelete}
          />
        </ContextMenuContent>
      </ContextMenu>
      {isBulk && (
        <AlertDialog open={deleteConfirmationOpen} onOpenChange={setDeleteConfirmationOpen}>
          <DeleteConfirmDialog selectedCount={selectedCount} onConfirm={onConfirmDelete} />
        </AlertDialog>
      )}
      {!isBulk && isFinePointer && deleteAction && (
        <ActionConfirmPopover
          open={deleteConfirmationOpen}
          anchorRef={anchorRef}
          focusBoundaryRef={focusBoundaryRef}
          title={deleteAction.title}
          description={deleteAction.description}
          cancelLabel={deleteAction.cancelLabel}
          confirmLabel={deleteAction.label}
          confirmAriaLabel={deleteAction.title}
          confirmTestId="file-delete-confirm"
          testId="file-delete-confirm-popover"
          onOpenChange={setDeleteConfirmationOpen}
          onCancel={() => setDeleteConfirmationOpen(false)}
          onConfirm={onConfirmDelete}
        />
      )}
    </FileDeleteActionContext.Provider>
  );
}

/** Context menu for file nodes with Download, Rename, and Delete options */
export function FileContextMenu({
  children,
  node,
  tree,
  setTree,
  onDeleteFile,
  onRenameFile,
  onDownloadFile,
  onStartRename,
  onAddToChatContext,
  selectedCount = 0,
  selectedPaths,
  anchorRef: providedAnchorRef,
}: {
  children: React.ReactNode;
  node: FileTreeNode;
  tree: FileTreeNode | null;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  onStartRename: () => void;
  onAddToChatContext?: (node: FileTreeNode) => void;
  selectedCount?: number;
  selectedPaths?: Set<string>;
  anchorRef?: React.RefObject<HTMLElement | null>;
}) {
  const editorActions = useFileTreeEditorActions();
  const { isFinePointer } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  const isBulk = selectedCount > 1;
  // A bulk selection would make a single-node "Open in <editor>" ambiguous.
  const showOpenInEditor = !isBulk && canOpenNodeInEditor(editorActions, node);
  const hasFileActions = !!onDeleteFile || !!onRenameFile || !!onDownloadFile;
  const showAddToChatContext = !isBulk && !!onAddToChatContext;
  const fallbackAnchorRef = useRef<HTMLElement>(null);
  const anchorRef = providedAnchorRef ?? fallbackAnchorRef;
  const contextMenuRef = useRef<HTMLElement>(null);
  const { deleteConfirmationOpen, setDeleteConfirmationOpen, handleConfirmDelete, handleDelete } =
    useFileContextMenuDelete({
      tree,
      setTree,
      node,
      isBulk,
      onDeleteFile,
      selectedPaths,
    });
  const handleMenuDelete = useFileMenuDeleteHandler({
    handleDelete,
    isBulk,
    isFinePointer,
    contextMenuRef,
  });

  const deleteAction = onDeleteFile
    ? {
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
      }
    : null;

  if (!hasFileActions && !showOpenInEditor && !showAddToChatContext) return <>{children}</>;

  return (
    <FileContextMenuSurface
      node={node}
      onDeleteFile={onDeleteFile}
      onRenameFile={onRenameFile}
      onDownloadFile={onDownloadFile}
      onStartRename={onStartRename}
      onAddToChatContext={onAddToChatContext}
      selectedCount={selectedCount}
      isBulk={isBulk}
      isFinePointer={isFinePointer}
      showOpenInEditor={showOpenInEditor}
      hasFileActions={hasFileActions}
      showAddToChatContext={showAddToChatContext}
      deleteConfirmationOpen={deleteConfirmationOpen}
      setDeleteConfirmationOpen={setDeleteConfirmationOpen}
      anchorRef={anchorRef}
      focusBoundaryRef={contextMenuRef}
      deleteAction={deleteAction}
      onConfirmDelete={handleConfirmDelete}
      onDelete={handleMenuDelete}
    >
      {providedAnchorRef ? children : attachAnchorRef(children, anchorRef)}
    </FileContextMenuSurface>
  );
}

/** Hook for managing inline file rename state */
export function useFileRename(
  node: FileTreeNode,
  tree: FileTreeNode | null,
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>,
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>,
) {
  const { t } = useTranslation();
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
        title: t("task:invalidName"),
        description: t("task:fileNamesCannotContainPathSeparators"),
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
        title: t("task:failedToRenameItem"),
        description: t("task:targetAlreadyExists", { newPath }),
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
      inputRef.current?.focus();
      inputRef.current?.select();
      const blurTimer = setTimeout(() => {
        blurEnabledRef.current = true;
      }, 400);
      return () => {
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
        "min-w-0 flex-1 truncate group-hover:text-foreground",
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
