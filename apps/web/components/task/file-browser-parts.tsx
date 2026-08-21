"use client";

import React from "react";
import {
  IconChevronRight,
  IconChevronDown,
  IconFolder,
  IconFolderOpen,
  IconRefresh,
  IconDotsVertical,
  IconTrash,
} from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/ui/file-icon";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileInfo } from "@/lib/state/store";
import type { FileBrowserRow } from "./file-browser-hooks";
import { InlineFileInput } from "./inline-file-input";
import { renderSessionOrLoadState } from "./file-browser-load-state";
import {
  FileContextMenu,
  useFileDeleteAction,
  useFileRename,
  TreeNodeName,
  getGitStatusTextClass,
} from "./file-context-menu";

export {
  compareTreeNodes,
  mergeTreeNodes,
  insertNodeInTree,
  removeNodeFromTree,
  renameNodeInTree,
} from "./file-tree-utils";

type GitFileStatus = FileInfo["status"] | undefined;

type TreeNodeRowProps = {
  row: FileBrowserRow;
  activeFolderPath: string;
  activeFilePath?: string | null;
  visibleLoadingPaths: Set<string>;
  fileStatuses: Map<string, GitFileStatus>;
  tree: FileTreeNode | null;
  onToggleExpand: (node: FileTreeNode) => void;
  onOpenFile: (path: string) => void;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  isSelectedFn?: (path: string) => boolean;
  onSelect?: (path: string, e: React.MouseEvent) => boolean;
  isDragging?: boolean;
  dragOverPath?: string | null;
  onDragStart?: (path: string, e: React.DragEvent) => void;
  onDragEnd?: () => void;
  onDragOver?: (path: string, e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (targetPath: string, e: React.DragEvent) => void;
  selectedCount?: number;
  selectedPaths?: Set<string>;
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
};

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

function getTreeNodeRowClass(
  isActive: boolean,
  isActiveFolder: boolean,
  isSelected: boolean,
  isDragging: boolean | undefined,
  isDropTarget: boolean,
) {
  return cn(
    "group flex w-full items-center gap-1 border border-transparent px-2 py-0.5 text-left text-sm cursor-pointer",
    isSelected ? "border-primary/50 bg-card text-foreground hover:bg-muted/70" : "hover:bg-muted",
    isActive && !isSelected && "bg-muted",
    isActiveFolder && !isSelected && "bg-muted/50",
    isDragging && isSelected && "opacity-50",
    isDropTarget && "bg-accent/40 ring-1 ring-accent",
  );
}

export function FileTreeNodeTouchActions({
  node,
  showTouchActions,
  onAddToChatContext,
}: {
  node: FileTreeNode;
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
}) {
  const { t } = useTranslation();
  const deleteAction = useFileDeleteAction();
  if (!showTouchActions || (!onAddToChatContext && !deleteAction)) return null;

  const stopRowInteraction = (event: React.SyntheticEvent) => event.stopPropagation();

  if (deleteAction?.confirming && !deleteAction.isBulk) {
    return (
      <InlineConfirmActions
        density="touch"
        testId="file-delete-inline-confirmation"
        ariaLabel={deleteAction.title}
        description={deleteAction.description}
        cancelLabel={deleteAction.cancelLabel}
        confirmLabel={deleteAction.label}
        confirmAriaLabel={deleteAction.title}
        confirmTestId="file-delete-confirm"
        onCancel={deleteAction.onCancel}
        onConfirm={deleteAction.onConfirm}
      />
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          data-testid="file-tree-node-actions"
          data-path={node.path}
          aria-label={t("chat:fileTreeActions")}
          className="ml-auto flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer"
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
      >
        {onAddToChatContext && (
          <DropdownMenuItem
            data-testid="file-tree-touch-add-to-chat"
            className="min-h-11 cursor-pointer"
            onSelect={() => onAddToChatContext(node)}
          >
            {t("chat:addToChatContext")}
          </DropdownMenuItem>
        )}
        {deleteAction && (
          <DropdownMenuItem
            data-testid="file-tree-touch-delete"
            variant="destructive"
            className="min-h-11 cursor-pointer"
            onSelect={deleteAction.onDelete}
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

export function TreeNodeItem(props: TreeNodeRowProps) {
  const { row, activeFolderPath, activeFilePath, visibleLoadingPaths } = props;
  const {
    fileStatuses,
    tree,
    onToggleExpand,
    onOpenFile,
    onDeleteFile,
    onRenameFile,
    onDownloadFile,
    setTree,
    showTouchActions,
    onAddToChatContext,
  } = props;
  const node = row.node;

  const isExpanded = row.isExpanded;
  const isActive = !node.is_dir && activeFilePath === node.path;
  const isActiveFolder = node.is_dir && activeFolderPath === node.path;
  const gitStatus = node.is_dir ? undefined : fileStatuses.get(node.path);
  const rename = useFileRename(node, tree, setTree, onRenameFile);
  const isSelected = props.isSelectedFn?.(node.path) ?? false;
  const isDropTarget = node.is_dir && props.dragOverPath === node.path;
  const rowAnchorRef = React.useRef<HTMLDivElement>(null);

  const handleClick = (e: React.MouseEvent) => {
    if (e.button === 2) return;
    const consumed = props.onSelect?.(node.path, e);
    if (!consumed) {
      handleTreeNodeClick(node, onToggleExpand, onOpenFile);
    }
  };

  // Inline the row JSX so ContextMenuTrigger asChild can attach directly to the DOM div
  const rowContent = (
    <div
      ref={rowAnchorRef}
      data-testid="file-tree-node"
      data-path={node.path}
      data-is-dir={node.is_dir ? "true" : "false"}
      data-active={isActive ? "true" : "false"}
      data-selected={isSelected ? "true" : "false"}
      aria-selected={isSelected}
      role="treeitem"
      className={cn(
        getTreeNodeRowClass(isActive, isActiveFolder, isSelected, props.isDragging, isDropTarget),
        props.showTouchActions && "flex-wrap",
      )}
      style={{ paddingLeft: treeNodePaddingLeft(row.depth, node.is_dir) }}
      onClick={handleClick}
      draggable={!!props.onDragStart}
      onDragStart={(e) => props.onDragStart?.(node.path, e)}
      onDragEnd={() => props.onDragEnd?.()}
      onDragOver={(e) => {
        if (node.is_dir) props.onDragOver?.(node.path, e);
      }}
      onDragLeave={(e) => props.onDragLeave?.(e)}
      onDrop={(e) => {
        if (node.is_dir) props.onDrop?.(node.path, e);
      }}
    >
      {node.is_dir && (
        <span className="flex-shrink-0">
          <TreeNodeExpandChevron
            isLoading={visibleLoadingPaths.has(node.path)}
            isExpanded={isExpanded}
          />
        </span>
      )}
      <TreeNodeFileIcon node={node} isExpanded={isExpanded} isActive={isActive} />
      <TreeNodeName node={node} isActive={isActive} gitStatus={gitStatus} rename={rename} />
      <FileTreeNodeTouchActions
        node={node}
        showTouchActions={showTouchActions}
        onAddToChatContext={onAddToChatContext}
      />
    </div>
  );

  return (
    <FileContextMenu
      node={node}
      tree={tree}
      setTree={setTree}
      onDeleteFile={onDeleteFile}
      onRenameFile={onRenameFile}
      onDownloadFile={onDownloadFile}
      onStartRename={rename.handleStartRename}
      onAddToChatContext={onAddToChatContext}
      selectedCount={props.selectedCount}
      selectedPaths={props.selectedPaths}
      anchorRef={rowAnchorRef}
    >
      {rowContent}
    </FileContextMenu>
  );
}

type SearchResultsListProps = {
  searchResults: string[] | null;
  fileStatuses: Map<string, GitFileStatus>;
  onOpenFile: (path: string) => void;
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
};

function searchResultNode(path: string): FileTreeNode {
  return {
    name: path.split("/").pop() || path,
    path,
    is_dir: false,
    size: 0,
  };
}

export function SearchResultsList({
  searchResults,
  fileStatuses,
  onOpenFile,
  showTouchActions,
  onAddToChatContext,
}: SearchResultsListProps) {
  const { t } = useTranslation();
  if (!searchResults) return null;

  if (searchResults.length === 0) {
    return (
      <div className="p-4 text-sm text-muted-foreground text-center">{t("task:noFilesFound")}</div>
    );
  }

  return (
    <div className="pb-2">
      {searchResults.map((path) => {
        const node = searchResultNode(path);
        const name = node.name;
        const folder = path.includes("/") ? path.substring(0, path.lastIndexOf("/")) : "";
        const gitStatus = fileStatuses.get(path);
        const row = (
          <div
            data-testid="file-search-result"
            data-path={path}
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
            <FileTreeNodeTouchActions
              node={node}
              showTouchActions={showTouchActions}
              onAddToChatContext={onAddToChatContext}
            />
          </div>
        );

        return (
          <FileContextMenu
            key={path}
            node={node}
            tree={null}
            setTree={() => {}}
            onStartRename={() => {}}
            onAddToChatContext={onAddToChatContext}
          >
            {row}
          </FileContextMenu>
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
  visibleRows: FileBrowserRow[];
  activeFolderPath: string;
  activeFilePath?: string | null;
  visibleLoadingPaths: Set<string>;
  onOpenFile: (path: string) => void;
  onToggleExpand: (node: FileTreeNode) => void;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  onCreateFileSubmit: (parentPath: string, name: string) => void;
  onCancelCreate: () => void;
  onRetry: () => void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  isSelectedFn?: (path: string) => boolean;
  onSelect?: (path: string, e: React.MouseEvent) => boolean;
  isDragging?: boolean;
  dragOverPath?: string | null;
  onDragStart?: (path: string, e: React.DragEvent) => void;
  onDragEnd?: () => void;
  onDragOver?: (path: string, e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (targetPath: string, e: React.DragEvent) => void;
  selectedCount?: number;
  selectedPaths?: Set<string>;
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
};

function rowToItemProps(props: FileBrowserContentAreaProps, row: FileBrowserRow): TreeNodeRowProps {
  return {
    row,
    activeFolderPath: props.activeFolderPath,
    activeFilePath: props.activeFilePath,
    visibleLoadingPaths: props.visibleLoadingPaths,
    fileStatuses: props.fileStatuses,
    tree: props.tree,
    onToggleExpand: props.onToggleExpand,
    onOpenFile: props.onOpenFile,
    onDeleteFile: props.onDeleteFile,
    onRenameFile: props.onRenameFile,
    onDownloadFile: props.onDownloadFile,
    setTree: props.setTree,
    isSelectedFn: props.isSelectedFn,
    onSelect: props.onSelect,
    isDragging: props.isDragging,
    dragOverPath: props.dragOverPath,
    onDragStart: props.onDragStart,
    onDragEnd: props.onDragEnd,
    onDragOver: props.onDragOver,
    onDragLeave: props.onDragLeave,
    onDrop: props.onDrop,
    selectedCount: props.selectedCount,
    selectedPaths: props.selectedPaths,
    showTouchActions: props.showTouchActions,
    onAddToChatContext: props.onAddToChatContext,
  };
}

function FileTreeView(props: FileBrowserContentAreaProps) {
  const { tree, visibleRows, creatingInPath, onCreateFileSubmit, onCancelCreate } = props;
  if (!tree) return null;
  return (
    <div className="w-full min-w-0 max-w-full pb-2">
      {creatingInPath === "" && (
        <InlineFileInput
          depth={0}
          onSubmit={(name) => onCreateFileSubmit("", name)}
          onCancel={onCancelCreate}
        />
      )}
      {visibleRows.map((row) => (
        <React.Fragment key={row.path}>
          <TreeNodeItem {...rowToItemProps(props, row)} />
          {creatingInPath === row.path && row.isDir && row.isExpanded && (
            <InlineFileInput
              depth={row.depth + 1}
              onSubmit={(name) => onCreateFileSubmit(row.path, name)}
              onCancel={onCancelCreate}
            />
          )}
        </React.Fragment>
      ))}
    </div>
  );
}

export function FileBrowserContentArea(props: FileBrowserContentAreaProps) {
  const { t } = useTranslation();
  if (props.isSearchActive && props.searchResults !== null) {
    return (
      <SearchResultsList
        searchResults={props.searchResults}
        fileStatuses={props.fileStatuses}
        onOpenFile={props.onOpenFile}
        showTouchActions={props.showTouchActions}
        onAddToChatContext={props.onAddToChatContext}
      />
    );
  }
  const loadStateResult = renderSessionOrLoadState({
    isSessionFailed: props.isSessionFailed,
    sessionError: props.sessionError,
    loadState: props.loadState,
    isLoadingTree: props.isLoadingTree,
    tree: props.tree,
    loadError: props.loadError,
    onRetry: props.onRetry,
  });
  if (loadStateResult) return loadStateResult;
  if (props.tree) return <FileTreeView {...props} />;
  return <div className="p-4 text-sm text-muted-foreground">{t("task:noFilesFound")}</div>;
}
