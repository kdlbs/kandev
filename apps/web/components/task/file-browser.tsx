"use client";

import React, {
  useEffect,
  useMemo,
  useCallback,
  useLayoutEffect,
  useRef,
  useState,
  type Ref,
} from "react";
import { ScrollArea } from "@kandev/ui/scroll-area";
import type { FileTreeNode, OpenFileTab } from "@/lib/types/backend";
import { useSession } from "@/hooks/domains/session/use-session";
import { useRepository } from "@/hooks/domains/workspace/use-repository";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useAppStore } from "@/components/state-provider";
import { useOpenSessionFolder } from "@/hooks/use-open-session-folder";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useMultiSelect } from "@/hooks/use-multi-select";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isEditableKeydownTarget } from "@/lib/keyboard/utils";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { FileBrowserHeader } from "./file-browser-header";
import { insertNodeInTree, removeNodeFromTree, FileBrowserContentArea } from "./file-browser-parts";
import {
  useFileBrowserSearch,
  useFileBrowserTree,
  useScrollPersistence,
  loadNodeChildren,
  toggleFolderExpand,
  fetchAndOpenFile,
} from "./file-browser-hooks";
import { getFileBrowserSessionWorkspacePath, resolveFileBrowserPaths } from "./file-browser-path";
import { FileTreeEditorProvider } from "./file-tree-editor-menu";
import { computeMoveTargets, getVisiblePaths, moveNodesInTree } from "./file-tree-utils";
import { useFileTreeReveal } from "./file-tree-reveal";

type FileBrowserProps = {
  sessionId: string;
  environmentId?: string | null;
  onOpenFile: (file: OpenFileTab) => void;
  onCreateFile?: (path: string) => Promise<boolean>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>;
  onDownloadFile?: (path: string) => Promise<boolean>;
  activeFilePath?: string | null;
  onAddSources?: (opener: HTMLButtonElement) => void;
  addSourcesButtonRef?: Ref<HTMLButtonElement>;
  addSourcesDisabledReason?: string;
};

function useFileBrowserHandlers(
  sessionId: string,
  onOpenFile: (file: OpenFileTab) => void,
  onCreateFile: FileBrowserProps["onCreateFile"],
  treeState: ReturnType<typeof useFileBrowserTree>,
) {
  const { toast } = useToast();
  const { t } = useTranslation("chat");
  const addContextFile = useContextFilesStore((state) => state.addFile);
  const [creatingInPath, setCreatingInPath] = useState<string | null>(null);
  const [activeFolderPath, setActiveFolderPath] = useState<string>("");
  const openFileAbortRef = useRef<AbortController | null>(null);

  useLayoutEffect(
    () => () => {
      openFileAbortRef.current?.abort();
      openFileAbortRef.current = null;
    },
    [sessionId],
  );

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
    (node: FileTreeNode) => toggleFolderExpand({ node, sessionId, treeState, setActiveFolderPath }),
    [treeState, sessionId],
  );

  const openFileByPath = useCallback(
    (path: string) => {
      openFileAbortRef.current?.abort();
      const controller = new AbortController();
      openFileAbortRef.current = controller;
      return fetchAndOpenFile(sessionId, path, onOpenFile, toast, {
        signal: controller.signal,
      }).finally(() => {
        if (openFileAbortRef.current === controller) {
          openFileAbortRef.current = null;
        }
      });
    },
    [sessionId, onOpenFile, toast],
  );
  const handleCancelCreate = useCallback(() => setCreatingInPath(null), []);
  const handleAddToChatContext = useCallback(
    (node: FileTreeNode) => {
      addContextFile(sessionId, {
        path: node.path,
        name: node.name,
        isDirectory: node.is_dir,
      });
      toast({
        description: t("chat:addedToChatContext", { name: node.name }),
        variant: "success",
      });
    },
    [addContextFile, sessionId, t, toast],
  );

  return {
    creatingInPath,
    activeFolderPath,
    handleStartCreate,
    handleCreateFileSubmit,
    toggleExpand,
    openFileByPath,
    handleCancelCreate,
    handleAddToChatContext,
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

function executeMoveFiles(
  params: MoveFilesParams,
  toast: ReturnType<typeof useToast>["toast"],
  t: TFunction,
) {
  const { sources, targetPath, treeState, setSelectedPaths, onRenameFile } = params;
  const snapshot = treeState.tree;

  // Compute deduplicated target paths before modifying the tree
  const targets = treeState.tree ? computeMoveTargets(treeState.tree, sources, targetPath) : [];
  treeState.setTree((prev) => (prev ? moveNodesInTree(prev, sources, targetPath) : prev));
  setSelectedPaths(new Set());

  const movePromises = targets.map(({ oldPath, newPath }) => onRenameFile(oldPath, newPath));

  Promise.all(movePromises)
    .then((results) => {
      if (results.some((ok) => !ok)) {
        treeState.setTree(snapshot);
        toast({
          title: t("task:moveFailed"),
          description: t("task:moveFailedFiles"),
          variant: "error",
        });
      }
    })
    .catch(() => {
      treeState.setTree(snapshot);
      toast({
        title: t("task:moveFailed"),
        description: t("task:moveFailedUnexpected"),
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
  const { t } = useTranslation();
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

  // Safety net: clear drag state if dragend doesn't fire on the element
  // (e.g. Escape key, drag outside browser window)
  useEffect(() => {
    if (!isDragging) return;
    const cleanup = () => {
      setIsDragging(false);
      setDragOverPath(null);
      dragPathsRef.current = [];
    };
    document.addEventListener("dragend", cleanup);
    return () => document.removeEventListener("dragend", cleanup);
  }, [isDragging]);

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
      executeMoveFiles(
        { sources, targetPath, treeState, setSelectedPaths, onRenameFile },
        toast,
        t,
      );
    },
    [onRenameFile, treeState, setSelectedPaths, t, toast],
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

function useSelectionInteractions(
  sessionId: string,
  treeState: ReturnType<typeof useFileBrowserTree>,
  containerRef: React.RefObject<HTMLDivElement | null>,
  activeFilePath: string | null | undefined,
  onRenameFile?: (oldPath: string, newPath: string) => Promise<boolean>,
) {
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

  useKeyboardShortcuts(containerRef, multiSelect.clearSelection, multiSelect.selectAll);
  useFileTreeReveal({
    activeFilePath,
    sessionId,
    tree: treeState.tree,
    setExpandedPaths: treeState.setExpandedPaths,
    isLoading: treeState.isLoading,
    loadChildren: (node, shouldApply) =>
      loadNodeChildren(node, sessionId, treeState, { force: true, shouldApply }),
  });

  const handleClickOutside = useCallback(
    (e: React.MouseEvent) => {
      if (multiSelect.selectedPaths.size === 0) return;
      const target = e.target as HTMLElement;
      // Don't clear if clicking on a tree node, context menu, or dialog
      if (
        target.closest("[data-testid='file-tree-node']") ||
        target.closest("[role='menu']") ||
        target.closest("[role='alertdialog']") ||
        target.closest("[role='dialog']")
      )
        return;
      multiSelect.clearSelection();
    },
    [multiSelect],
  );

  return { multiSelect, dnd, handleClickOutside };
}

function useKeyboardShortcuts(
  containerRef: React.RefObject<HTMLDivElement | null>,
  clearSelection: () => void,
  selectAll: () => void,
) {
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!container.contains(document.activeElement) && document.activeElement !== container)
        return;
      if (e.key === "Escape") {
        clearSelection();
      } else if ((e.ctrlKey || e.metaKey) && e.key === "a" && !isEditableKeydownTarget(e)) {
        e.preventDefault();
        selectAll();
      }
    };
    container.addEventListener("keydown", handleKeyDown);
    return () => container.removeEventListener("keydown", handleKeyDown);
  }, [containerRef, clearSelection, selectAll]);
}

export function getFileBrowserResetKey({
  sessionId,
  environmentId,
  worktreeCount,
  workspaceFilesRefresh,
}: {
  sessionId: string;
  environmentId?: string | null;
  worktreeCount: number;
  workspaceFilesRefresh: number;
}) {
  return `${environmentId ?? sessionId}:${worktreeCount}:${workspaceFilesRefresh}`;
}

function useFileBrowserResetKey(sessionId: string, environmentId?: string | null) {
  // Worktree count participates in the tree's reset key so an add_branch_to_task
  // call that materializes a sibling worktree forces a fresh tree load.
  const worktreeCount = useAppStore(
    (state) => state.sessionWorktreesBySessionId.itemsBySessionId[sessionId]?.length ?? 0,
  );
  const workspaceFilesRefresh = useAppStore(
    (state) => state.workspaceFilesRefresh.bySessionId[sessionId] ?? 0,
  );
  return getFileBrowserResetKey({
    sessionId,
    environmentId,
    worktreeCount,
    workspaceFilesRefresh,
  });
}

function useFileBrowserData(sessionId: string, environmentId: string | null | undefined) {
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const repository = useRepository(session?.repository_id ?? null);
  const gitStatus = useSessionGitStatus(sessionId);
  const { open: openFolder } = useOpenSessionFolder(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);
  const search = useFileBrowserSearch(sessionId);
  const resetKey = useFileBrowserResetKey(sessionId, environmentId);
  const treeState = useFileBrowserTree(sessionId, resetKey);
  const isTreeLoaded = !treeState.isLoadingTree && treeState.tree !== null;
  const fileStatuses = useMemo(
    () =>
      new Map(Object.entries(gitStatus?.files ?? {}).map(([path, info]) => [path, info.status])),
    [gitStatus?.files],
  );
  const paths = resolveFileBrowserPaths({
    sessionWorktreePath: getFileBrowserSessionWorkspacePath(session),
    repositoryLocalPath: repository?.local_path,
    treePath: treeState.tree?.path,
    treeLoaded: isTreeLoaded,
  });
  return {
    isSessionFailed,
    sessionError,
    openFolder,
    copied,
    copyPath,
    search,
    treeState,
    isTreeLoaded,
    fileStatuses,
    ...paths,
  };
}

function useFileBrowserViewModel({
  sessionId,
  environmentId,
  onOpenFile,
  onCreateFile,
  onRenameFile,
  activeFilePath,
  scrollAreaRef,
  containerRef,
}: Pick<
  FileBrowserProps,
  "sessionId" | "environmentId" | "onOpenFile" | "onCreateFile" | "onRenameFile" | "activeFilePath"
> & {
  scrollAreaRef: React.RefObject<HTMLDivElement | null>;
  containerRef: React.RefObject<HTMLDivElement | null>;
}) {
  const data = useFileBrowserData(sessionId, environmentId);
  useScrollPersistence(sessionId, data.isTreeLoaded, scrollAreaRef, data.treeState.tree);
  const handlers = useFileBrowserHandlers(sessionId, onOpenFile, onCreateFile, data.treeState);
  const selection = useSelectionInteractions(
    sessionId,
    data.treeState,
    containerRef,
    activeFilePath,
    onRenameFile,
  );
  return { data, handlers, ...selection };
}

function FileBrowserTreeContent({
  scrollAreaRef,
  data,
  handlers,
  multiSelect,
  dnd,
  activeFilePath,
  onDeleteFile,
  onRenameFile,
  onDownloadFile,
  showTouchActions,
}: Omit<FileBrowserProps, "sessionId" | "environmentId" | "onOpenFile" | "onCreateFile"> & {
  scrollAreaRef: React.RefObject<HTMLDivElement | null>;
  data: ReturnType<typeof useFileBrowserData>;
  handlers: ReturnType<typeof useFileBrowserHandlers>;
  multiSelect: ReturnType<typeof useSelectionInteractions>["multiSelect"];
  dnd: ReturnType<typeof useSelectionInteractions>["dnd"];
  showTouchActions: boolean;
}) {
  const { search, isSessionFailed, sessionError, treeState, fileStatuses } = data;
  return (
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
        visibleRows={treeState.visibleRows}
        activeFolderPath={handlers.activeFolderPath}
        activeFilePath={activeFilePath}
        visibleLoadingPaths={treeState.visibleLoadingPaths}
        onOpenFile={handlers.openFileByPath}
        onToggleExpand={handlers.toggleExpand}
        onDeleteFile={onDeleteFile}
        onRenameFile={onRenameFile}
        onDownloadFile={onDownloadFile}
        onAddToChatContext={handlers.handleAddToChatContext}
        showTouchActions={showTouchActions}
        onCreateFileSubmit={handlers.handleCreateFileSubmit}
        onCancelCreate={handlers.handleCancelCreate}
        onRetry={() => void treeState.loadTree({ resetRetry: true })}
        setTree={treeState.setTree}
        isSelectedFn={multiSelect.isSelected}
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
  );
}

export function FileBrowser({
  sessionId,
  environmentId,
  onOpenFile,
  onCreateFile,
  onDeleteFile,
  onRenameFile,
  onDownloadFile,
  activeFilePath,
  onAddSources,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: FileBrowserProps) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const showTouchActions = isMobile || !isFinePointer;
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const { data, handlers, multiSelect, dnd, handleClickOutside } = useFileBrowserViewModel({
    sessionId,
    environmentId,
    onOpenFile,
    onCreateFile,
    onRenameFile,
    activeFilePath,
    scrollAreaRef,
    containerRef,
  });
  const { openFolder, copied, copyPath, search, treeState, fullPath, displayPath } = data;
  return (
    <FileTreeEditorProvider sessionId={sessionId} treeRootName={treeState.tree?.name}>
      <div
        className="flex flex-col h-full"
        ref={containerRef}
        tabIndex={-1}
        onMouseDown={handleClickOutside}
      >
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
          onAddSources={onAddSources}
          addSourcesButtonRef={addSourcesButtonRef}
          addSourcesDisabledReason={addSourcesDisabledReason}
        />
        <FileBrowserTreeContent
          scrollAreaRef={scrollAreaRef}
          data={data}
          handlers={handlers}
          multiSelect={multiSelect}
          dnd={dnd}
          activeFilePath={activeFilePath}
          onDeleteFile={onDeleteFile}
          onRenameFile={onRenameFile}
          onDownloadFile={onDownloadFile}
          showTouchActions={showTouchActions}
        />
      </div>
    </FileTreeEditorProvider>
  );
}
