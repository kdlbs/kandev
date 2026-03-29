"use client";

import React from "react";
import {
  IconChevronRight,
  IconChevronDown,
  IconFolder,
  IconFolderOpen,
  IconRefresh,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { FileIcon } from "@/components/ui/file-icon";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileInfo } from "@/lib/state/store";
import { InlineFileInput } from "./inline-file-input";
import { renderSessionOrLoadState } from "./file-browser-load-state";
import { compareTreeNodes } from "./file-tree-utils";
import {
  FileContextMenu,
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
  isSelected?: boolean;
  onSelect?: (path: string, e: React.MouseEvent) => void;
  isDragging?: boolean;
  dragOverPath?: string | null;
  onDragStart?: (path: string, e: React.DragEvent) => void;
  onDragEnd?: () => void;
  onDragOver?: (path: string, e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (targetPath: string, e: React.DragEvent) => void;
  selectedCount?: number;
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

function getTreeNodeRowClass(
  isActive: boolean,
  isActiveFolder: boolean,
  isSelected: boolean | undefined,
  isDragging: boolean | undefined,
  isDropTarget: boolean,
) {
  return cn(
    "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
    "hover:bg-muted",
    isActive && !isSelected && "bg-muted",
    isActiveFolder && !isSelected && "bg-muted/50",
    isSelected && "bg-accent",
    isDragging && isSelected && "opacity-50",
    isDropTarget && "bg-accent/40 ring-1 ring-accent",
  );
}

function TreeNodeRow({
  node,
  depth,
  isActive,
  isActiveFolder,
  isExpanded,
  isLoading,
  isSelected,
  isDragging,
  isDropTarget,
  gitStatus,
  rename,
  onClick,
  onDragStart,
  onDragEnd,
  onDragOver,
  onDragLeave,
  onDrop,
}: {
  node: FileTreeNode;
  depth: number;
  isActive: boolean;
  isActiveFolder: boolean;
  isExpanded: boolean;
  isLoading: boolean;
  isSelected?: boolean;
  isDragging?: boolean;
  isDropTarget: boolean;
  gitStatus: GitFileStatus;
  rename: ReturnType<typeof useFileRename>;
  onClick: (e: React.MouseEvent) => void;
  onDragStart?: (path: string, e: React.DragEvent) => void;
  onDragEnd?: () => void;
  onDragOver?: (path: string, e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (targetPath: string, e: React.DragEvent) => void;
}) {
  return (
    <div
      data-testid={`tree-node-${node.path}`}
      data-selected={isSelected ? "true" : "false"}
      className={getTreeNodeRowClass(isActive, isActiveFolder, isSelected, isDragging, isDropTarget)}
      style={{ paddingLeft: treeNodePaddingLeft(depth, node.is_dir) }}
      onClick={onClick}
      draggable={!!onDragStart}
      onDragStart={(e) => onDragStart?.(node.path, e)}
      onDragEnd={() => onDragEnd?.()}
      onDragOver={(e) => {
        if (node.is_dir) onDragOver?.(node.path, e);
      }}
      onDragLeave={(e) => onDragLeave?.(e)}
      onDrop={(e) => {
        if (node.is_dir) onDrop?.(node.path, e);
      }}
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
}

export function TreeNodeItem(props: TreeNodeItemProps) {
  const { node, depth, expandedPaths, activeFolderPath, activeFilePath, visibleLoadingPaths } =
    props;
  const { fileStatuses, tree, onToggleExpand, onOpenFile, onDeleteFile, onRenameFile, setTree } =
    props;

  const isExpanded = expandedPaths.has(node.path);
  const isActive = !node.is_dir && activeFilePath === node.path;
  const isActiveFolder = node.is_dir && activeFolderPath === node.path;
  const gitStatus = node.is_dir ? undefined : fileStatuses.get(node.path);
  const rename = useFileRename(node, tree, setTree, onRenameFile);

  const handleClick = (e: React.MouseEvent) => {
    if (props.onSelect) {
      props.onSelect(node.path, e);
      if (!e.ctrlKey && !e.metaKey && !e.shiftKey) {
        handleTreeNodeClick(node, onToggleExpand, onOpenFile);
      }
    } else {
      handleTreeNodeClick(node, onToggleExpand, onOpenFile);
    }
  };

  return (
    <div>
      <FileContextMenu
        node={node}
        tree={tree}
        setTree={setTree}
        onDeleteFile={onDeleteFile}
        onRenameFile={onRenameFile}
        onStartRename={rename.handleStartRename}
        selectedCount={props.selectedCount}
      >
        <TreeNodeRow
          node={node}
          depth={depth}
          isActive={isActive}
          isActiveFolder={isActiveFolder}
          isExpanded={isExpanded}
          isLoading={visibleLoadingPaths.has(node.path)}
          isSelected={props.isSelected}
          isDragging={props.isDragging}
          isDropTarget={node.is_dir && props.dragOverPath === node.path}
          gitStatus={gitStatus}
          rename={rename}
          onClick={handleClick}
          onDragStart={props.onDragStart}
          onDragEnd={props.onDragEnd}
          onDragOver={props.onDragOver}
          onDragLeave={props.onDragLeave}
          onDrop={props.onDrop}
        />
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
  isSelected?: (path: string) => boolean;
  onSelect?: (path: string, e: React.MouseEvent) => void;
  isDragging?: boolean;
  dragOverPath?: string | null;
  onDragStart?: (path: string, e: React.DragEvent) => void;
  onDragEnd?: () => void;
  onDragOver?: (path: string, e: React.DragEvent) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (targetPath: string, e: React.DragEvent) => void;
  selectedCount?: number;
};

function FileTreeView(props: FileBrowserContentAreaProps) {
  const { tree, creatingInPath, onCreateFileSubmit, onCancelCreate } = props;
  if (!tree) return null;
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
        [...tree.children].sort(compareTreeNodes).map((child) => (
          <TreeNodeItem
            key={child.path}
            node={child}
            depth={0}
            expandedPaths={props.expandedPaths}
            activeFolderPath={props.activeFolderPath}
            activeFilePath={props.activeFilePath}
            visibleLoadingPaths={props.visibleLoadingPaths}
            creatingInPath={creatingInPath}
            fileStatuses={props.fileStatuses}
            tree={tree}
            onToggleExpand={props.onToggleExpand}
            onOpenFile={props.onOpenFile}
            onDeleteFile={props.onDeleteFile}
            onRenameFile={props.onRenameFile}
            onCreateFileSubmit={onCreateFileSubmit}
            onCancelCreate={onCancelCreate}
            setTree={props.setTree}
            isSelected={props.isSelected?.(child.path)}
            onSelect={props.onSelect}
            isDragging={props.isDragging}
            dragOverPath={props.dragOverPath}
            onDragStart={props.onDragStart}
            onDragEnd={props.onDragEnd}
            onDragOver={props.onDragOver}
            onDragLeave={props.onDragLeave}
            onDrop={props.onDrop}
            selectedCount={props.selectedCount}
          />
        ))}
    </div>
  );
}

export function FileBrowserContentArea(props: FileBrowserContentAreaProps) {
  if (props.isSearchActive && props.searchResults !== null) {
    return (
      <SearchResultsList
        searchResults={props.searchResults}
        fileStatuses={props.fileStatuses}
        onOpenFile={props.onOpenFile}
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
  return <div className="p-4 text-sm text-muted-foreground">No files found</div>;
}
