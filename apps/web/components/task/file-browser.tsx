"use client";

import React, { useEffect, useMemo, useCallback, useRef, useState } from "react";
import { ScrollArea } from "@kandev/ui/scroll-area";
import type { FileTreeNode, OpenFileTab } from "@/lib/types/backend";
import { useSession } from "@/hooks/domains/session/use-session";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useOpenSessionFolder } from "@/hooks/use-open-session-folder";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useToast } from "@/components/toast-provider";
import { useMultiSelect } from "@/hooks/use-multi-select";
import { FileBrowserSearchHeader } from "./file-browser-search-header";
import {
  insertNodeInTree,
  removeNodeFromTree,
  FileBrowserToolbar,
  FileBrowserContentArea,
} from "./file-browser-parts";
import {
  useFileBrowserSearch,
  useFileBrowserTree,
  useScrollPersistence,
  loadNodeChildren,
  fetchAndOpenFile,
} from "./file-browser-hooks";
import { getVisiblePaths, moveNodesInTree } from "./file-tree-utils";

type FileBrowserHeaderProps = {
  treeLoaded: boolean;
  search: ReturnType<typeof useFileBrowserSearch>;
  displayPath: string;
  fullPath: string;
  copied: boolean;
  expandedPathsSize: number;
  onCopyPath: (value: string) => void | Promise<void>;
  onStartCreate?: () => void;
  onOpenFolder: () => void;
  onCollapseAll: () => void;
  showCreateButton: boolean;
};

function FileBrowserHeader({
  treeLoaded,
  search,
  displayPath,
  fullPath,
  copied,
  expandedPathsSize,
  onCopyPath,
  onStartCreate,
  onOpenFolder,
  onCollapseAll,
  showCreateButton,
}: FileBrowserHeaderProps) {
  if (!treeLoaded) return null;
  if (search.isSearchActive) {
    return (
      <FileBrowserSearchHeader
        isSearching={search.isSearching}
        localSearchQuery={search.localSearchQuery}
        searchInputRef={search.searchInputRef}
        onSearchChange={search.handleSearchChange}
        onCloseSearch={search.handleCloseSearch}
      />
    );
  }
  return (
    <FileBrowserToolbar
      displayPath={displayPath}
      fullPath={fullPath}
      copied={copied}
      expandedPathsSize={expandedPathsSize}
      onCopyPath={onCopyPath}
      onStartCreate={onStartCreate}
      onOpenFolder={onOpenFolder}
      onStartSearch={() => search.setIsSearchActive(true)}
      onCollapseAll={onCollapseAll}
      showCreateButton={showCreateButton}
    />
  );
}

type FileBrowserProps = {
  sessionId: string;
  environmentId?: string | null;
  onOpenFile: (file: OpenFileTab) => void;
  onCreateFile?: (path: string) => Promise<boolean>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  activeFilePath?: string | null;
};

function useFileBrowserHandlers(
  sessionId: string,
  onOpenFile: (file: OpenFileTab) => void,
  onCreateFile: FileBrowserProps["onCreateFile"],
  treeState: ReturnType<typeof useFileBrowserTree>,
) {
  const { toast } = useToast();
  const [creatingInPath, setCreatingInPath] = useState<string | null>(null);
  const [activeFolderPath, setActiveFolderPath] = useState<string>("");

  const handleStartCreate = useCallback(() => {
    if (activeFolderPath && !treeState.expandedPaths.has(activeFolderPath)) {
      treeState.setExpandedPaths((prev) => new Set(prev).add(activeFolderPath));
    }
    setCreatingInPath(activeFolderPath);
  }, [activeFolderPath, treeState]);

  const handleCreateFileSubmit = useCallback(
    (parentPath: string, name: string) => {
      setCreatingInPath(null);
      const newPath = parentPath ? `${parentPath}/${name}` : name;
      const newNode: FileTreeNode = { name, path: newPath, is_dir: false, size: 0 };
      treeState.setTree((prev) => (prev ? insertNodeInTree(prev, parentPath, newNode) : prev));
      onCreateFile?.(newPath)
        .then((ok) => {
          if (!ok) treeState.setTree((prev) => (prev ? removeNodeFromTree(prev, newPath) : prev));
        })
        .catch(() => {
          treeState.setTree((prev) => (prev ? removeNodeFromTree(prev, newPath) : prev));
        });
    },
    [onCreateFile, treeState],
  );

  const toggleExpand = useCallback(
    async (node: FileTreeNode) => {
      if (!node.is_dir) return;
      setActiveFolderPath(node.path);
      const newExpanded = new Set(treeState.expandedPaths);
      if (newExpanded.has(node.path)) {
        newExpanded.delete(node.path);
      } else {
        await loadNodeChildren(node, sessionId, treeState);
        newExpanded.add(node.path);
      }
      treeState.setExpandedPaths(newExpanded);
    },
    [treeState, sessionId],
  );

  const openFileByPath = useCallback(
    (path: string) => fetchAndOpenFile(sessionId, path, onOpenFile, toast),
    [sessionId, onOpenFile, toast],
  );
  const handleCancelCreate = useCallback(() => setCreatingInPath(null), []);

  return {
    creatingInPath,
    activeFolderPath,
    handleStartCreate,
    handleCreateFileSubmit,
    toggleExpand,
    openFileByPath,
    handleCancelCreate,
  };
}

function isDropInvalid(sources: string[], targetPath: string): boolean {
  return sources.some((s) => s === targetPath || targetPath.startsWith(`${s}/`));
}

type MoveFilesParams = {
  sources: string[];
  targetPath: string;
  treeState: ReturnType<typeof useFileBrowserTree>;
  setSelectedPaths: (paths: Set<string>) => void;
  onRenameFile: (oldPath: string, newPath: string) => Promise<boolean>;
};

function executeMoveFiles(params: MoveFilesParams, toast: ReturnType<typeof useToast>["toast"]) {
  const { sources, targetPath, treeState, setSelectedPaths, onRenameFile } = params;
  const snapshot = treeState.tree;
  treeState.setTree((prev) => (prev ? moveNodesInTree(prev, sources, targetPath) : prev));
  setSelectedPaths(new Set());

  const movePromises = sources.map((sourcePath) => {
    const name = sourcePath.split("/").pop() || sourcePath;
    const newPath = targetPath ? `${targetPath}/${name}` : name;
    return onRenameFile(sourcePath, newPath);
  });

  Promise.all(movePromises)
    .then((results) => {
      if (results.some((ok) => !ok)) {
        treeState.setTree(snapshot);
        toast({
          title: "Move failed",
          description: "Some files could not be moved",
          variant: "error",
        });
      }
    })
    .catch(() => {
      treeState.setTree(snapshot);
      toast({
        title: "Move failed",
        description: "An error occurred while moving files",
        variant: "error",
      });
    });
}

function useDragAndDrop(
  treeState: ReturnType<typeof useFileBrowserTree>,
  selectedPaths: Set<string>,
  setSelectedPaths: (paths: Set<string>) => void,
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>,
) {
  const { toast } = useToast();
  const [isDragging, setIsDragging] = useState(false);
  const [dragOverPath, setDragOverPath] = useState<string | null>(null);
  const dragPathsRef = useRef<string[]>([]);

  const handleDragStart = useCallback(
    (path: string, e: React.DragEvent) => {
      const paths = selectedPaths.has(path) ? [...selectedPaths] : [path];
      if (!selectedPaths.has(path)) setSelectedPaths(new Set([path]));
      dragPathsRef.current = paths;
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", JSON.stringify(paths));
      setIsDragging(true);
    },
    [selectedPaths, setSelectedPaths],
  );

  const handleDragEnd = useCallback(() => {
    setIsDragging(false);
    setDragOverPath(null);
    dragPathsRef.current = [];
  }, []);

  const handleDragOver = useCallback((targetPath: string, e: React.DragEvent) => {
    const sources = dragPathsRef.current;
    if (isDropInvalid(sources, targetPath)) return;
    const allSameParent = sources.every((s) => {
      const parent = s.includes("/") ? s.substring(0, s.lastIndexOf("/")) : "";
      return parent === targetPath;
    });
    if (allSameParent) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setDragOverPath(targetPath);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    const related = e.relatedTarget as Node | null;
    if (related && (e.currentTarget as Node).contains(related)) return;
    setDragOverPath(null);
  }, []);

  const handleDrop = useCallback(
    (targetPath: string, e: React.DragEvent) => {
      e.preventDefault();
      setDragOverPath(null);
      setIsDragging(false);
      if (!onRenameFile) return;
      const sources = dragPathsRef.current;
      if (sources.length === 0 || isDropInvalid(sources, targetPath)) return;
      executeMoveFiles({ sources, targetPath, treeState, setSelectedPaths, onRenameFile }, toast);
    },
    [onRenameFile, treeState, setSelectedPaths, toast],
  );

  return {
    isDragging,
    dragOverPath,
    handleDragStart,
    handleDragEnd,
    handleDragOver,
    handleDragLeave,
    handleDrop,
  };
}

function useAutoExpandAncestors(
  activeFilePath: string | null | undefined,
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>,
) {
  useEffect(() => {
    if (!activeFilePath) return;
    const parts = activeFilePath.split("/");
    if (parts.length <= 1) return;
    const ancestors: string[] = [];
    for (let i = 1; i < parts.length; i++) {
      ancestors.push(parts.slice(0, i).join("/"));
    }
    setExpandedPaths((prev) => {
      if (ancestors.every((p) => prev.has(p))) return prev;
      const next = new Set(prev);
      for (const p of ancestors) next.add(p);
      return next;
    });
  }, [activeFilePath, setExpandedPaths]);
}

function useKeyboardShortcuts(
  containerRef: React.RefObject<HTMLDivElement | null>,
  multiSelect: ReturnType<typeof useMultiSelect>,
) {
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!container.contains(document.activeElement) && document.activeElement !== container)
        return;
      if (e.key === "Escape") {
        multiSelect.clearSelection();
      } else if ((e.ctrlKey || e.metaKey) && e.key === "a") {
        e.preventDefault();
        multiSelect.selectAll();
      }
    };
    container.addEventListener("keydown", handleKeyDown);
    return () => container.removeEventListener("keydown", handleKeyDown);
  }, [containerRef, multiSelect]);
}

export function FileBrowser({
  sessionId,
  environmentId,
  onOpenFile,
  onCreateFile,
  onDeleteFile,
  onRenameFile,
  activeFilePath,
}: FileBrowserProps) {
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const repository = useRepository(session?.repository_id ?? null);
  const gitStatus = useSessionGitStatus(sessionId);
  const { open: openFolder } = useOpenSessionFolder(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const search = useFileBrowserSearch(sessionId);
  const treeState = useFileBrowserTree(sessionId, environmentId ?? undefined);
  const isTreeLoaded = !treeState.isLoadingTree && treeState.tree !== null;
  useScrollPersistence(sessionId, isTreeLoaded, scrollAreaRef, treeState.tree);

  const fileStatuses = useMemo(
    () =>
      new Map(Object.entries(gitStatus?.files ?? {}).map(([path, info]) => [path, info.status])),
    [gitStatus?.files],
  );
  const fullPath = session?.worktree_path || repository?.local_path || "";
  const displayPath = fullPath.replace(/^\/(?:Users|home)\/[^/]+\//, "~/");

  const handlers = useFileBrowserHandlers(sessionId, onOpenFile, onCreateFile, treeState);

  const visiblePaths = useMemo(
    () => (treeState.tree ? getVisiblePaths(treeState.tree, treeState.expandedPaths) : []),
    [treeState.tree, treeState.expandedPaths],
  );
  const multiSelect = useMultiSelect({ items: visiblePaths });
  const dnd = useDragAndDrop(
    treeState,
    multiSelect.selectedPaths,
    multiSelect.setSelectedPaths,
    onRenameFile,
  );

  useKeyboardShortcuts(containerRef, multiSelect);
  useAutoExpandAncestors(activeFilePath, treeState.setExpandedPaths);

  return (
    <div className="flex flex-col h-full" ref={containerRef} tabIndex={-1}>
      <FileBrowserHeader
        treeLoaded={Boolean(treeState.tree && treeState.loadState === "loaded")}
        search={search}
        displayPath={displayPath}
        fullPath={fullPath}
        copied={copied}
        expandedPathsSize={treeState.expandedPaths.size}
        onCopyPath={copyPath}
        onStartCreate={onCreateFile ? handlers.handleStartCreate : undefined}
        onOpenFolder={openFolder}
        onCollapseAll={treeState.collapseAll}
        showCreateButton={Boolean(onCreateFile)}
      />
      <ScrollArea className="flex-1" ref={scrollAreaRef}>
        <FileBrowserContentArea
          isSearchActive={search.isSearchActive}
          searchResults={search.searchResults}
          isSessionFailed={isSessionFailed}
          sessionError={sessionError}
          loadState={treeState.loadState}
          isLoadingTree={treeState.isLoadingTree}
          tree={treeState.tree}
          loadError={treeState.loadError}
          creatingInPath={handlers.creatingInPath}
          fileStatuses={fileStatuses}
          expandedPaths={treeState.expandedPaths}
          activeFolderPath={handlers.activeFolderPath}
          activeFilePath={activeFilePath}
          visibleLoadingPaths={treeState.visibleLoadingPaths}
          onOpenFile={handlers.openFileByPath}
          onToggleExpand={handlers.toggleExpand}
          onDeleteFile={onDeleteFile}
          onRenameFile={onRenameFile}
          onCreateFileSubmit={handlers.handleCreateFileSubmit}
          onCancelCreate={handlers.handleCancelCreate}
          onRetry={() => void treeState.loadTree({ resetRetry: true })}
          setTree={treeState.setTree}
          isSelected={multiSelect.isSelected}
          onSelect={multiSelect.handleClick}
          isDragging={dnd.isDragging}
          dragOverPath={dnd.dragOverPath}
          onDragStart={dnd.handleDragStart}
          onDragEnd={dnd.handleDragEnd}
          onDragOver={dnd.handleDragOver}
          onDragLeave={dnd.handleDragLeave}
          onDrop={dnd.handleDrop}
          selectedCount={multiSelect.selectedPaths.size}
          selectedPaths={multiSelect.selectedPaths}
        />
      </ScrollArea>
    </div>
  );
}
