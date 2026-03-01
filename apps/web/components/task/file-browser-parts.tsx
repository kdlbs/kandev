"use client";

import React, { useState, useCallback, useRef, useEffect } from "react";
import { Input } from "@kandev/ui/input";
import {
  IconChevronRight,
  IconChevronDown,
  IconFolder,
  IconFolderOpen,
  IconPencil,
  IconTrash,
  IconRefresh,
} from "@tabler/icons-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
  ContextMenuSeparator,
} from "@kandev/ui/context-menu";
import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/ui/file-icon";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileInfo } from "@/lib/state/store";
import { InlineFileInput } from "./inline-file-input";
import { renderSessionOrLoadState } from "./file-browser-load-state";

type GitFileStatus = FileInfo["status"] | undefined;

/** Sort comparator: directories first, then alphabetical by name. */
export const compareTreeNodes = (a: FileTreeNode, b: FileTreeNode): number => {
  if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
  return a.name.localeCompare(b.name);
};

/**
 * Merge a freshly-fetched tree node into an existing one, preserving
 * already-loaded children so expanded folders don't collapse.
 */
export function mergeTreeNodes(existing: FileTreeNode, incoming: FileTreeNode): FileTreeNode {
  if (!incoming.children) return { ...existing, ...incoming, children: existing.children };
  if (!existing.children) return incoming;
  const existingByPath = new Map(existing.children.map((c) => [c.path, c]));
  const mergedChildren = incoming.children.map((inChild) => {
    const exChild = existingByPath.get(inChild.path);
    if (exChild && exChild.is_dir && inChild.is_dir) {
      return mergeTreeNodes(exChild, inChild);
    }
    return inChild;
  });
  return { ...existing, ...incoming, children: mergedChildren };
}

/** Insert a file node into a parent folder, keeping children sorted (dirs first, then alpha). */
export function insertNodeInTree(
  root: FileTreeNode,
  parentPath: string,
  node: FileTreeNode,
): FileTreeNode {
  if (root.path === parentPath || (parentPath === "" && root.path === "")) {
    const children = [...(root.children ?? []), node].sort(compareTreeNodes);
    return { ...root, children };
  }
  if (!root.children) return root;
  return { ...root, children: root.children.map((c) => insertNodeInTree(c, parentPath, node)) };
}

type TreeNodeItemProps = {
  node: FileTreeNode;
  depth: number;
  expandedPaths: Set<string>;
  activeFolderPath: string;
  activeFilePath?: string | null;
  visibleLoadingPaths: Set<string>;
  creatingInPath: string | null;
  fileStatuses: Map<string, GitFileStatus>;
  tree: FileTreeNode | null;
  onToggleExpand: (node: FileTreeNode) => void;
  onOpenFile: (path: string) => void;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onCreateFileSubmit: (parentPath: string, name: string) => void;
  onCancelCreate: () => void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
};

export function removeNodeFromTree(root: FileTreeNode, targetPath: string): FileTreeNode {
  if (!root.children) return root;
  const filtered = root.children.filter((c) => c.path !== targetPath);
  return { ...root, children: filtered.map((c) => removeNodeFromTree(c, targetPath)) };
}

/** Rename a node in the tree, updating its name and path. */
export function renameNodeInTree(
  root: FileTreeNode,
  oldPath: string,
  newName: string,
): FileTreeNode {
  if (root.path === oldPath) {
    const parentPath = oldPath.includes("/") ? oldPath.substring(0, oldPath.lastIndexOf("/")) : "";
    const newPath = parentPath ? `${parentPath}/${newName}` : newName;
    return { ...root, name: newName, path: newPath };
  }
  if (!root.children) return root;
  return {
    ...root,
    children: root.children.map((c) => renameNodeInTree(c, oldPath, newName)),
  };
}

function treeNodePaddingLeft(depth: number, isDir: boolean): string {
  return `${depth * 12 + 8 + (isDir ? 0 : 20)}px`;
}

function handleTreeNodeClick(
  node: FileTreeNode,
  onToggleExpand: (node: FileTreeNode) => void,
  onOpenFile: (path: string) => void,
) {
  if (node.is_dir) {
    onToggleExpand(node);
    return;
  }
  onOpenFile(node.path);
}

/** Expand/collapse chevron for directory nodes. */
function TreeNodeExpandChevron({
  isLoading,
  isExpanded,
}: {
  isLoading: boolean;
  isExpanded: boolean;
}) {
  if (isLoading)
    return <IconRefresh className="h-4 w-4 animate-spin text-muted-foreground shrink-0" />;
  if (isExpanded) return <IconChevronDown className="h-3 w-3 text-muted-foreground/60" />;
  return <IconChevronRight className="h-3 w-3 text-muted-foreground/60" />;
}

/** Directory or file icon for a tree node. */
function TreeNodeFileIcon({
  node,
  isExpanded,
  isActive,
}: {
  node: FileTreeNode;
  isExpanded: boolean;
  isActive: boolean;
}) {
  if (node.is_dir) {
    return isExpanded ? (
      <IconFolderOpen className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
    ) : (
      <IconFolder className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
    );
  }
  return (
    <FileIcon
      fileName={node.name}
      filePath={node.path}
      className="flex-shrink-0"
      style={{ width: "14px", height: "14px", opacity: isActive ? 1 : 0.7 }}
    />
  );
}

function getGitStatusTextClass(status: GitFileStatus): string {
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

/** Context menu for file nodes with Rename and Delete options */
function FileContextMenu({
  children,
  node,
  tree,
  setTree,
  onDeleteFile,
  onRenameFile,
  onStartRename,
}: {
  children: React.ReactNode;
  node: FileTreeNode;
  tree: FileTreeNode | null;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onStartRename: () => void;
}) {
  const handleDelete = useCallback(() => {
    if (!onDeleteFile) return;
    const snapshot = tree;
    setTree((prev) => (prev ? removeNodeFromTree(prev, node.path) : prev));
    onDeleteFile(node.path)
      .then((ok) => {
        if (!ok) setTree(snapshot);
      })
      .catch(() => {
        setTree(snapshot);
      });
  }, [tree, setTree, node.path, onDeleteFile]);

  const hasActions = onDeleteFile || onRenameFile;

  if (!hasActions || node.is_dir) {
    return <>{children}</>;
  }

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>
        {onRenameFile && (
          <ContextMenuItem onSelect={onStartRename}>
            <IconPencil className="h-3.5 w-3.5" />
            Rename
          </ContextMenuItem>
        )}
        {onRenameFile && onDeleteFile && <ContextMenuSeparator />}
        {onDeleteFile && (
          <ContextMenuItem variant="destructive" onSelect={handleDelete}>
            <IconTrash className="h-3.5 w-3.5" />
            Delete
          </ContextMenuItem>
        )}
      </ContextMenuContent>
    </ContextMenu>
  );
}

/** Hook for managing inline file rename state */
function useFileRename(
  node: FileTreeNode,
  tree: FileTreeNode | null,
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>,
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>,
) {
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
    const parentPath = node.path.includes("/")
      ? node.path.substring(0, node.path.lastIndexOf("/"))
      : "";
    const newPath = parentPath ? `${parentPath}/${newName}` : newName;
    const snapshot = tree;
    setTree((prev) => (prev ? renameNodeInTree(prev, node.path, newName) : prev));
    setIsRenaming(false);
    onRenameFile(node.path, newPath)
      .then((ok) => {
        if (!ok) setTree(snapshot);
      })
      .catch(() => {
        setTree(snapshot);
      });
  }, [renameValue, node.name, node.path, onRenameFile, tree, setTree, handleCancelRename]);

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
function TreeNodeName({
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

  // Focus input after context menu closes (autoFocus doesn't work reliably with context menus)
  // Context menus restore focus to trigger on close, so we need to wait for that to complete
  useEffect(() => {
    if (rename.isRenaming) {
      blurEnabledRef.current = false;
      // Focus after context menu has fully closed and restored focus to trigger
      const focusTimer = setTimeout(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      }, 150);
      // Enable blur handling after focus is stable
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
    // Only handle blur after the input has been properly focused
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

/** Expanded directory children */
function TreeNodeChildren({ props, depth }: { props: TreeNodeItemProps; depth: number }) {
  const { node, creatingInPath, onCreateFileSubmit, onCancelCreate } = props;
  return (
    <div>
      {creatingInPath === node.path && (
        <InlineFileInput
          depth={depth + 1}
          onSubmit={(name) => onCreateFileSubmit(node.path, name)}
          onCancel={onCancelCreate}
        />
      )}
      {node.children?.map((child) => (
        <TreeNodeItem key={child.path} {...props} node={child} depth={depth + 1} />
      ))}
    </div>
  );
}

export function TreeNodeItem(props: TreeNodeItemProps) {
  const { node, depth, expandedPaths, activeFolderPath, activeFilePath, visibleLoadingPaths } =
    props;
  const { fileStatuses, tree, onToggleExpand, onOpenFile, onDeleteFile, onRenameFile, setTree } =
    props;

  const isExpanded = expandedPaths.has(node.path);
  const isLoading = visibleLoadingPaths.has(node.path);
  const isActive = !node.is_dir && activeFilePath === node.path;
  const isActiveFolder = node.is_dir && activeFolderPath === node.path;
  const gitStatus = node.is_dir ? undefined : fileStatuses.get(node.path);
  const rename = useFileRename(node, tree, setTree, onRenameFile);

  const rowContent = (
    <div
      className={cn(
        "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
        "hover:bg-muted",
        isActive && "bg-muted",
        isActiveFolder && "bg-muted/50",
      )}
      style={{ paddingLeft: treeNodePaddingLeft(depth, node.is_dir) }}
      onClick={() => handleTreeNodeClick(node, onToggleExpand, onOpenFile)}
    >
      {node.is_dir && (
        <span className="flex-shrink-0">
          <TreeNodeExpandChevron isLoading={isLoading} isExpanded={isExpanded} />
        </span>
      )}
      <TreeNodeFileIcon node={node} isExpanded={isExpanded} isActive={isActive} />
      <TreeNodeName node={node} isActive={isActive} gitStatus={gitStatus} rename={rename} />
    </div>
  );

  return (
    <div>
      <FileContextMenu
        node={node}
        tree={tree}
        setTree={setTree}
        onDeleteFile={onDeleteFile}
        onRenameFile={onRenameFile}
        onStartRename={rename.handleStartRename}
      >
        {rowContent}
      </FileContextMenu>
      {node.is_dir && isExpanded && <TreeNodeChildren props={props} depth={depth} />}
    </div>
  );
}

type SearchResultsListProps = {
  searchResults: string[] | null;
  fileStatuses: Map<string, GitFileStatus>;
  onOpenFile: (path: string) => void;
};

export function SearchResultsList({
  searchResults,
  fileStatuses,
  onOpenFile,
}: SearchResultsListProps) {
  if (!searchResults) return null;

  if (searchResults.length === 0) {
    return <div className="p-4 text-sm text-muted-foreground text-center">No files found</div>;
  }

  return (
    <div className="pb-2">
      {searchResults.map((path) => {
        const name = path.split("/").pop() || path;
        const folder = path.includes("/") ? path.substring(0, path.lastIndexOf("/")) : "";
        const gitStatus = fileStatuses.get(path);
        return (
          <div
            key={path}
            className={cn(
              "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
              "hover:bg-muted",
            )}
            onClick={() => onOpenFile(path)}
          >
            <FileIcon
              fileName={name}
              filePath={path}
              className="flex-shrink-0"
              style={{ width: "14px", height: "14px" }}
            />
            <span
              className={cn(
                "truncate group-hover:text-foreground",
                getGitStatusTextClass(gitStatus) || "text-muted-foreground",
              )}
            >
              {folder && <span>{folder}/</span>}
              <span>{name}</span>
            </span>
          </div>
        );
      })}
    </div>
  );
}

export { FileBrowserToolbar } from "./file-browser-toolbar";

type FileBrowserContentAreaProps = {
  isSearchActive: boolean;
  searchResults: string[] | null;
  isSessionFailed: boolean;
  sessionError?: string | null;
  loadState: string;
  isLoadingTree: boolean;
  tree: FileTreeNode | null;
  loadError: string | null;
  creatingInPath: string | null;
  fileStatuses: Map<string, GitFileStatus>;
  expandedPaths: Set<string>;
  activeFolderPath: string;
  activeFilePath?: string | null;
  visibleLoadingPaths: Set<string>;
  onOpenFile: (path: string) => void;
  onToggleExpand: (node: FileTreeNode) => void;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onCreateFileSubmit: (parentPath: string, name: string) => void;
  onCancelCreate: () => void;
  onRetry: () => void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
};

export function FileBrowserContentArea({
  isSearchActive,
  searchResults,
  isSessionFailed,
  sessionError,
  loadState,
  isLoadingTree,
  tree,
  loadError,
  creatingInPath,
  fileStatuses,
  expandedPaths,
  activeFolderPath,
  activeFilePath,
  visibleLoadingPaths,
  onOpenFile,
  onToggleExpand,
  onDeleteFile,
  onRenameFile,
  onCreateFileSubmit,
  onCancelCreate,
  onRetry,
  setTree,
}: FileBrowserContentAreaProps) {
  if (isSearchActive && searchResults !== null) {
    return (
      <SearchResultsList
        searchResults={searchResults}
        fileStatuses={fileStatuses}
        onOpenFile={onOpenFile}
      />
    );
  }
  const loadStateResult = renderSessionOrLoadState({
    isSessionFailed,
    sessionError,
    loadState,
    isLoadingTree,
    tree,
    loadError,
    onRetry,
  });
  if (loadStateResult) return loadStateResult;
  if (tree) {
    return (
      <div className="pb-2">
        {creatingInPath === "" && (
          <InlineFileInput
            depth={0}
            onSubmit={(name) => onCreateFileSubmit("", name)}
            onCancel={onCancelCreate}
          />
        )}
        {tree.children &&
          [...tree.children]
            .sort(compareTreeNodes)
            .map((child) => (
              <TreeNodeItem
                key={child.path}
                node={child}
                depth={0}
                expandedPaths={expandedPaths}
                activeFolderPath={activeFolderPath}
                activeFilePath={activeFilePath}
                visibleLoadingPaths={visibleLoadingPaths}
                creatingInPath={creatingInPath}
                fileStatuses={fileStatuses}
                tree={tree}
                onToggleExpand={onToggleExpand}
                onOpenFile={onOpenFile}
                onDeleteFile={onDeleteFile}
                onRenameFile={onRenameFile}
                onCreateFileSubmit={onCreateFileSubmit}
                onCancelCreate={onCancelCreate}
                setTree={setTree}
              />
            ))}
      </div>
    );
  }
  return <div className="p-4 text-sm text-muted-foreground">No files found</div>;
}
