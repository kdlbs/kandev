"use client";

import React, { useMemo, useCallback, useRef, useState } from "react";
import { ScrollArea } from "@kandev/ui/scroll-area";
import type { FileTreeNode, OpenFileTab } from "@/lib/types/backend";
import { useSession } from "@/hooks/domains/session/use-session";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useOpenSessionFolder } from "@/hooks/use-open-session-folder";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useToast } from "@/components/toast-provider";
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
  onOpenFile: (file: OpenFileTab) => void;
  onCreateFile?: (path: string) => Promise<boolean>;
  onDeleteFile?: (path: string) => Promise<boolean>;
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
  const handleCancelCreate = useCallback(() => {
    setCreatingInPath(null);
  }, []);

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

export function FileBrowser({
  sessionId,
  onOpenFile,
  onCreateFile,
  onDeleteFile,
  activeFilePath,
}: FileBrowserProps) {
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const repository = useRepository(session?.repository_id ?? null);
  const gitStatus = useSessionGitStatus(sessionId);
  const { open: openFolder } = useOpenSessionFolder(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);
  const scrollAreaRef = useRef<HTMLDivElement>(null);

  const search = useFileBrowserSearch(sessionId);
  const treeState = useFileBrowserTree(sessionId);
  const isTreeLoaded = !treeState.isLoadingTree && treeState.tree !== null;
  useScrollPersistence(sessionId, isTreeLoaded, scrollAreaRef, treeState.tree);

  const fileStatuses = useMemo(
    () =>
      new Map(Object.entries(gitStatus?.files ?? {}).map(([path, info]) => [path, info.status])),
    [gitStatus?.files],
  );
  const fullPath = session?.worktree_path || repository?.local_path || "";
  const displayPath = fullPath.replace(/^\/(?:Users|home)\/[^/]+\//, "~/");

  const {
    creatingInPath,
    activeFolderPath,
    handleStartCreate,
    handleCreateFileSubmit,
    toggleExpand,
    openFileByPath,
    handleCancelCreate,
  } = useFileBrowserHandlers(sessionId, onOpenFile, onCreateFile, treeState);

  return (
    <div className="flex flex-col h-full">
      <FileBrowserHeader
        treeLoaded={Boolean(treeState.tree && treeState.loadState === "loaded")}
        search={search}
        displayPath={displayPath}
        fullPath={fullPath}
        copied={copied}
        expandedPathsSize={treeState.expandedPaths.size}
        onCopyPath={copyPath}
        onStartCreate={onCreateFile ? handleStartCreate : undefined}
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
          creatingInPath={creatingInPath}
          fileStatuses={fileStatuses}
          expandedPaths={treeState.expandedPaths}
          activeFolderPath={activeFolderPath}
          activeFilePath={activeFilePath}
          visibleLoadingPaths={treeState.visibleLoadingPaths}
          onOpenFile={openFileByPath}
          onToggleExpand={toggleExpand}
          onDeleteFile={onDeleteFile}
          onCreateFileSubmit={handleCreateFileSubmit}
          onCancelCreate={handleCancelCreate}
          onRetry={() => void treeState.loadTree({ resetRetry: true })}
          setTree={treeState.setTree}
        />
      </ScrollArea>
    </div>
  );
}
