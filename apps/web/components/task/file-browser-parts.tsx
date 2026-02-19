'use client';

import React from 'react';
import {
  IconChevronRight, IconChevronDown, IconFolder, IconFolderOpen,
  IconSearch, IconX, IconLoader2, IconTrash, IconRefresh,
  IconListTree, IconFolderShare, IconCopy, IconCheck, IconPlus,
} from '@tabler/icons-react';
import { Input } from '@kandev/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { FileIcon } from '@/components/ui/file-icon';
import type { FileTreeNode } from '@/lib/types/backend';
import type { FileInfo } from '@/lib/state/store';
import { InlineFileInput } from './inline-file-input';
import { PanelHeaderBar, PanelHeaderBarSplit } from './panel-primitives';

type GitFileStatus = FileInfo['status'] | undefined;

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
export function insertNodeInTree(root: FileTreeNode, parentPath: string, node: FileTreeNode): FileTreeNode {
  if (root.path === parentPath || (parentPath === '' && root.path === '')) {
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
  onCreateFileSubmit: (parentPath: string, name: string) => void;
  onCancelCreate: () => void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
};

export function removeNodeFromTree(root: FileTreeNode, targetPath: string): FileTreeNode {
  if (!root.children) return root;
  const filtered = root.children.filter((c) => c.path !== targetPath);
  return { ...root, children: filtered.map((c) => removeNodeFromTree(c, targetPath)) };
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
function TreeNodeExpandChevron({ isLoading, isExpanded }: { isLoading: boolean; isExpanded: boolean }) {
  if (isLoading) return <IconRefresh className="h-4 w-4 animate-spin text-muted-foreground shrink-0" />;
  if (isExpanded) return <IconChevronDown className="h-3 w-3 text-muted-foreground/60" />;
  return <IconChevronRight className="h-3 w-3 text-muted-foreground/60" />;
}

/** Directory or file icon for a tree node. */
function TreeNodeFileIcon({ node, isExpanded, isActive }: { node: FileTreeNode; isExpanded: boolean; isActive: boolean }) {
  if (node.is_dir) {
    return isExpanded
      ? <IconFolderOpen className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
      : <IconFolder className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />;
  }
  return (
    <FileIcon
      fileName={node.name}
      filePath={node.path}
      className="flex-shrink-0"
      style={{ width: '14px', height: '14px', opacity: isActive ? 1 : 0.7 }}
    />
  );
}

function getGitStatusTextClass(status: GitFileStatus): string {
  switch (status) {
    case 'added':
    case 'untracked':
      return 'text-emerald-600';
    case 'modified':
      return 'text-yellow-600';
    default:
      return '';
  }
}

function getGitStatusDotClass(status: GitFileStatus): string {
  switch (status) {
    case 'added':
    case 'untracked':
      return 'bg-emerald-500';
    case 'modified':
      return 'bg-yellow-500';
    default:
      return 'bg-transparent';
  }
}

function TreeNodeGitStatusDot({ status }: { status: GitFileStatus }) {
  if (!status || (status !== 'added' && status !== 'untracked' && status !== 'modified')) return null;
  return <span className={cn('h-1.5 w-1.5 rounded-full', getGitStatusDotClass(status))} />;
}

/** Delete button shown on hover for file nodes. */
function TreeNodeDeleteButton({
  node, tree, setTree, onDeleteFile,
}: {
  node: FileTreeNode;
  tree: FileTreeNode | null;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  onDeleteFile: (path: string) => Promise<boolean>;
}) {
  return (
    <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="text-muted-foreground hover:text-destructive cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              const snapshot = tree;
              setTree((prev) => (prev ? removeNodeFromTree(prev, node.path) : prev));
              onDeleteFile(node.path).then((ok) => {
                if (!ok) setTree(snapshot);
              }).catch(() => {
                setTree(snapshot);
              });
            }}
          >
            <IconTrash className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Delete file</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function TreeNodeItem({
  node, depth, expandedPaths, activeFolderPath, activeFilePath,
  visibleLoadingPaths, creatingInPath, fileStatuses, tree,
  onToggleExpand, onOpenFile, onDeleteFile, onCreateFileSubmit, onCancelCreate, setTree,
}: TreeNodeItemProps) {
  const isExpanded = expandedPaths.has(node.path);
  const isLoading = visibleLoadingPaths.has(node.path);
  const isActive = !node.is_dir && activeFilePath === node.path;
  const isActiveFolder = node.is_dir && activeFolderPath === node.path;
  const gitStatus = node.is_dir ? undefined : fileStatuses.get(node.path);
  const rowPadding = treeNodePaddingLeft(depth, node.is_dir);
  const handleNodeClick = () => handleTreeNodeClick(node, onToggleExpand, onOpenFile);

  return (
    <div>
      <div
        className={cn(
          "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
          "hover:bg-muted",
          isActive && "bg-muted",
          isActiveFolder && "bg-muted/50"
        )}
        style={{ paddingLeft: rowPadding }}
        onClick={handleNodeClick}
      >
        {node.is_dir && (
          <span className="flex-shrink-0">
            <TreeNodeExpandChevron isLoading={isLoading} isExpanded={isExpanded} />
          </span>
        )}
        <TreeNodeFileIcon node={node} isExpanded={isExpanded} isActive={isActive} />
        <span
          className={cn(
            'flex-1 truncate group-hover:text-foreground',
            isActive ? 'text-foreground' : 'text-muted-foreground',
            node.is_dir ? 'font-medium' : getGitStatusTextClass(gitStatus),
          )}
        >
          {node.name}
        </span>
        {!node.is_dir && (
          <div className="ml-auto flex items-center gap-1">
            <TreeNodeGitStatusDot status={gitStatus} />
            {onDeleteFile && (
              <TreeNodeDeleteButton node={node} tree={tree} setTree={setTree} onDeleteFile={onDeleteFile} />
            )}
          </div>
        )}
      </div>
      {node.is_dir && isExpanded && (
        <div>
          {creatingInPath === node.path && (
            <InlineFileInput
              depth={depth + 1}
              onSubmit={(name) => onCreateFileSubmit(node.path, name)}
              onCancel={onCancelCreate}
            />
          )}
          {node.children && [...node.children]
            .sort(compareTreeNodes)
            .map((child) => (
              <TreeNodeItem
                key={child.path}
                node={child}
                depth={depth + 1}
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
                onCreateFileSubmit={onCreateFileSubmit}
                onCancelCreate={onCancelCreate}
                setTree={setTree}
              />
            ))}
        </div>
      )}
    </div>
  );
}

type SearchResultsListProps = {
  searchResults: string[] | null;
  fileStatuses: Map<string, GitFileStatus>;
  onOpenFile: (path: string) => void;
};

export function SearchResultsList({ searchResults, fileStatuses, onOpenFile }: SearchResultsListProps) {
  if (!searchResults) return null;

  if (searchResults.length === 0) {
    return (
      <div className="p-4 text-sm text-muted-foreground text-center">
        No files found
      </div>
    );
  }

  return (
    <div className="pb-2">
      {searchResults.map((path) => {
        const name = path.split('/').pop() || path;
        const folder = path.includes('/') ? path.substring(0, path.lastIndexOf('/')) : '';
        const gitStatus = fileStatuses.get(path);
        return (
          <div
            key={path}
            className={cn(
              "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
              "hover:bg-muted"
            )}
            onClick={() => onOpenFile(path)}
          >
            <FileIcon
              fileName={name}
              filePath={path}
              className="flex-shrink-0"
              style={{ width: '14px', height: '14px' }}
            />
            <span className={cn('truncate group-hover:text-foreground', getGitStatusTextClass(gitStatus) || 'text-muted-foreground')}>
              {folder && <span>{folder}/</span>}
              <span>{name}</span>
            </span>
            <div className="ml-auto flex items-center">
              <TreeNodeGitStatusDot status={gitStatus} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

type FileBrowserSearchHeaderProps = {
  isSearching: boolean;
  localSearchQuery: string;
  searchInputRef: React.RefObject<HTMLInputElement | null>;
  onSearchChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onCloseSearch: () => void;
};

export function FileBrowserSearchHeader({
  isSearching, localSearchQuery, searchInputRef, onSearchChange, onCloseSearch,
}: FileBrowserSearchHeaderProps) {
  return (
    <PanelHeaderBar className="group/header">
      {isSearching ? (
        <IconLoader2 className="h-3.5 w-3.5 text-muted-foreground animate-spin shrink-0" />
      ) : (
        <IconSearch className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      )}
      <Input
        ref={searchInputRef}
        type="text"
        value={localSearchQuery}
        onChange={onSearchChange}
        onKeyDown={(e) => { if (e.key === 'Escape') onCloseSearch(); }}
        placeholder="Search files..."
        className="flex-1 min-w-0 h-5 text-xs border-none bg-transparent shadow-none focus-visible:ring-0 px-2"
      />
      <button
        className="text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
        onClick={onCloseSearch}
      >
        <IconX className="h-3.5 w-3.5" />
      </button>
    </PanelHeaderBar>
  );
}

type FileBrowserToolbarProps = {
  displayPath: string;
  fullPath: string;
  copied: boolean;
  expandedPathsSize: number;
  onCopyPath: (text: string) => void;
  onStartCreate?: () => void;
  onOpenFolder: () => void;
  onStartSearch: () => void;
  onCollapseAll: () => void;
  showCreateButton: boolean;
};

export function FileBrowserToolbar({
  displayPath, fullPath, copied, expandedPathsSize,
  onCopyPath, onStartCreate, onOpenFolder, onStartSearch, onCollapseAll, showCreateButton,
}: FileBrowserToolbarProps) {
  return (
    <PanelHeaderBarSplit
      className="group/header"
      left={
        <>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="relative shrink-0 cursor-pointer"
                onClick={() => { if (fullPath) void onCopyPath(fullPath); }}
              >
                <IconFolderOpen className={cn("h-3.5 w-3.5 text-muted-foreground transition-opacity", copied ? "opacity-0" : "group-hover/header:opacity-0")} />
                {copied ? (
                  <IconCheck className="absolute inset-0 h-3.5 w-3.5 text-green-600/70" />
                ) : (
                  <IconCopy className="absolute inset-0 h-3.5 w-3.5 text-muted-foreground opacity-0 group-hover/header:opacity-100 hover:text-foreground transition-opacity" />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent>Copy workspace path</TooltipContent>
          </Tooltip>
          <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">
            {displayPath}
          </span>
        </>
      }
      right={
        <>
          {showCreateButton && onStartCreate && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                  onClick={onStartCreate}
                >
                  <IconPlus className="h-3.5 w-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>New file</TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                onClick={onOpenFolder}
              >
                <IconFolderShare className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent>Open workspace folder</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                onClick={onStartSearch}
              >
                <IconSearch className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent>Search files</TooltipContent>
          </Tooltip>
          {expandedPathsSize > 0 && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                  onClick={onCollapseAll}
                >
                  <IconListTree className="h-3.5 w-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>Collapse all</TooltipContent>
            </Tooltip>
          )}
        </>
      }
    />
  );
}

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
  onCreateFileSubmit: (parentPath: string, name: string) => void;
  onCancelCreate: () => void;
  onRetry: () => void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
};

export function FileBrowserContentArea({
  isSearchActive, searchResults, isSessionFailed, sessionError,
  loadState, isLoadingTree, tree, loadError, creatingInPath, fileStatuses,
  expandedPaths, activeFolderPath, activeFilePath, visibleLoadingPaths,
  onOpenFile, onToggleExpand, onDeleteFile, onCreateFileSubmit, onCancelCreate, onRetry, setTree,
}: FileBrowserContentAreaProps) {
  if (isSearchActive && searchResults !== null) {
    return <SearchResultsList searchResults={searchResults} fileStatuses={fileStatuses} onOpenFile={onOpenFile} />;
  }
  if (isSessionFailed) {
    return (
      <div className="p-4 text-sm text-destructive/80 space-y-2">
        <div>Session failed</div>
        {sessionError && (
          <div className="text-xs text-muted-foreground">{sessionError}</div>
        )}
      </div>
    );
  }
  if ((loadState === 'loading' || isLoadingTree) && !tree) {
    return <div className="p-4 text-sm text-muted-foreground">Loading files...</div>;
  }
  if (loadState === 'waiting') {
    return (
      <div className="p-4 text-sm text-muted-foreground flex items-center gap-2">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        Preparing workspace...
      </div>
    );
  }
  if (loadState === 'manual') {
    return (
      <div className="p-4 text-sm text-muted-foreground space-y-2">
        <div>{loadError ?? 'Workspace is still starting.'}</div>
        <button
          type="button"
          className="text-xs text-foreground underline cursor-pointer"
          onClick={onRetry}
        >
          Retry
        </button>
      </div>
    );
  }
  if (tree) {
    return (
      <div className="pb-2">
        {creatingInPath === '' && (
          <InlineFileInput
            depth={0}
            onSubmit={(name) => onCreateFileSubmit('', name)}
            onCancel={onCancelCreate}
          />
        )}
        {tree.children && [...tree.children]
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
