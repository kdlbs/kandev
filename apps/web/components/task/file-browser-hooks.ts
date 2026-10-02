"use client";

import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import type React from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { FileTreeNode } from "@/lib/types/backend";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { getFilesPanelExpandedPaths, setFilesPanelExpandedPaths } from "@/lib/local-storage";
import { useTree, type VisibleRow } from "@/hooks/use-tree";
import { compareTreeNodes, sortRootChildren } from "./file-tree-utils";
import { retainExpandedChildren, restoredExpandedPaths } from "./file-browser-restore";
import { useTreeLoader } from "./file-browser-tree-loader";
import { useFileTreeState } from "./file-browser-tree-state";
import type { FileTreeCacheBinding } from "./file-browser-tree-cache";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { applyFileChanges, FolderRefreshes } from "./file-browser-refresh";

const debugLoad = createDebugLogger("file-browser:load");
const debugChanges = createDebugLogger("file-browser:changes");

const FB_GET_PATH = (n: FileTreeNode) => n.path;
const FB_GET_CHILDREN = (n: FileTreeNode) =>
  n.children ? [...n.children].sort(compareTreeNodes) : undefined;
const FB_IS_DIR = (n: FileTreeNode) => n.is_dir;

export type FileBrowserRow = VisibleRow<FileTreeNode>;

export type LoadState = "loading" | "waiting" | "loaded" | "manual" | "error";

type FileBrowserTreeResult = {
  tree: FileTreeNode | null;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  expandedPaths: ReadonlySet<string>;
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>;
  visibleRows: FileBrowserRow[];
  visibleLoadingPaths: Set<string>;
  isLoadingTree: boolean;
  loadState: LoadState;
  loadError: string | null;
  loadTree: ReturnType<typeof useTreeLoader>["loadTree"];
  readTree?: ReturnType<typeof useTreeLoader>["readTree"];
  showLoading: (path: string) => void;
  hideLoading: (path: string) => void;
  isLoading: (path: string) => boolean;
  collapseAll: () => void;
};

function useMemoizedFileBrowserTreeResult(result: FileBrowserTreeResult) {
  return useMemo(
    () => result,
    [
      result.tree,
      result.setTree,
      result.expandedPaths,
      result.setExpandedPaths,
      result.visibleRows,
      result.visibleLoadingPaths,
      result.isLoadingTree,
      result.loadState,
      result.loadError,
      result.loadTree,
      result.readTree,
      result.showLoading,
      result.hideLoading,
      result.isLoading,
      result.collapseAll,
    ],
  );
}

export { useFileBrowserSearch } from "./file-browser-search";

export { applyFileChanges } from "./file-browser-refresh";

function useLoadingTimers() {
  const loadingTimersRef = useRef<Map<string, NodeJS.Timeout>>(new Map());
  const activeLoadsRef = useRef<Set<string>>(new Set());
  const [visibleLoadingPaths, setVisibleLoadingPaths] = useState<Set<string>>(new Set());

  const showLoading = useCallback((path: string) => {
    activeLoadsRef.current.add(path);
    const timer = setTimeout(() => {
      setVisibleLoadingPaths((prev) => new Set(prev).add(path));
      loadingTimersRef.current.delete(path);
    }, 150);
    loadingTimersRef.current.set(path, timer);
  }, []);

  const hideLoading = useCallback((path: string) => {
    activeLoadsRef.current.delete(path);
    const timer = loadingTimersRef.current.get(path);
    if (timer) {
      clearTimeout(timer);
      loadingTimersRef.current.delete(path);
    }
    setVisibleLoadingPaths((prev) => {
      const next = new Set(prev);
      next.delete(path);
      return next;
    });
  }, []);

  const isLoading = useCallback((path: string) => activeLoadsRef.current.has(path), []);

  return { visibleLoadingPaths, showLoading, hideLoading, isLoading };
}

function logLoad(event: string, data: Record<string, unknown>) {
  if (isDebug()) debugLoad(event, data);
}

type TreeLoadEffectsContext = {
  sessionId: string;
  effectiveResetKey: string;
  agentctlIsReady: boolean;
  agentctlIsReadyRef: React.MutableRefObject<boolean>;
  loadStateRef: React.MutableRefObject<LoadState>;
  treeRef: React.MutableRefObject<FileTreeNode | null>;
  retryAttemptRef: React.MutableRefObject<number>;
  hasInitializedExpandedRef: React.MutableRefObject<string | null>;
  restoreExpandedPathsRef: React.MutableRefObject<string[]>;
  expandedPathsRef: React.MutableRefObject<ReadonlySet<string>>;
  clearRetryTimer: () => void;
  loadTree: (options?: {
    resetRetry?: boolean;
    restoreExpandedPaths?: string[];
  }) => Promise<void> | void;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setIsLoadingTree: React.Dispatch<React.SetStateAction<boolean>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
  setLoadError: React.Dispatch<React.SetStateAction<string | null>>;
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>;
  lastResetKeyRef: React.MutableRefObject<string | null>;
  cacheBinding?: FileTreeCacheBinding;
};

function useTreeLoadEffects(ctx: TreeLoadEffectsContext) {
  const {
    sessionId,
    effectiveResetKey,
    agentctlIsReady,
    agentctlIsReadyRef,
    loadStateRef,
    treeRef,
    retryAttemptRef,
    hasInitializedExpandedRef,
    restoreExpandedPathsRef,
    expandedPathsRef,
    clearRetryTimer,
    loadTree,
    setTree,
    setIsLoadingTree,
    setLoadState,
    setLoadError,
    setExpandedPaths,
    lastResetKeyRef,
    cacheBinding,
  } = ctx;
  const lastCacheBindingRef = useRef(cacheBinding);

  useEffect(() => {
    const resetKeyChanged =
      lastResetKeyRef.current !== effectiveResetKey || lastCacheBindingRef.current !== cacheBinding;
    lastCacheBindingRef.current = cacheBinding;
    lastResetKeyRef.current = effectiveResetKey;
    clearRetryTimer();
    retryAttemptRef.current = 0;
    if (resetKeyChanged) {
      const savedPaths = restoredExpandedPaths(getFilesPanelExpandedPaths(effectiveResetKey));
      const retained = cacheBinding?.isCurrent() ? cacheBinding.cache.get(cacheBinding.key) : null;
      setTree(retained);
      setIsLoadingTree(true);
      setLoadState(agentctlIsReadyRef.current ? "loading" : "waiting");
      setLoadError(null);
      hasInitializedExpandedRef.current = null;
      restoreExpandedPathsRef.current = savedPaths;
      setExpandedPaths(savedPaths.length > 0 ? new Set(savedPaths) : new Set());
    }
    const savedPaths = resetKeyChanged
      ? restoreExpandedPathsRef.current
      : Array.from(expandedPathsRef.current);
    const expanded = new Set(savedPaths);
    setTree((current) => (current ? retainExpandedChildren(current, expanded) : current));
    restoreExpandedPathsRef.current = savedPaths;
    logLoad("init-effect", {
      sessionId,
      effectiveResetKey,
      agentctlReady: agentctlIsReadyRef.current,
      savedPaths: savedPaths.length,
      willLoad: agentctlIsReadyRef.current,
    });
    if (agentctlIsReadyRef.current) {
      void loadTree({ resetRetry: true, restoreExpandedPaths: savedPaths });
    } else setIsLoadingTree(false);
    return () => {
      clearRetryTimer();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs intentionally omitted
  }, [clearRetryTimer, loadTree, effectiveResetKey, sessionId, setExpandedPaths, cacheBinding]);

  useEffect(() => {
    let reason: string | null = null;
    if (!agentctlIsReady) reason = "agentctl-not-ready";
    else if (loadStateRef.current === "loading") reason = "already-loading";
    else if (loadStateRef.current === "loaded" && treeRef.current) reason = "already-loaded";
    if (reason) {
      logLoad("ready-effect-skip", { sessionId, reason, loadState: loadStateRef.current });
      return;
    }
    logLoad("ready-flip", { sessionId, loadState: loadStateRef.current });
    void loadTree({ resetRetry: true, restoreExpandedPaths: restoreExpandedPathsRef.current });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refs intentionally omitted
  }, [agentctlIsReady, loadTree, sessionId]);
}

function useFileChangeSubscription({
  sessionId,
  resetKey,
  cacheBinding,
  expandedPathsRef,
  setTree,
  setLoadState,
}: {
  sessionId: string;
  resetKey: string;
  cacheBinding?: FileTreeCacheBinding;
  expandedPathsRef: React.MutableRefObject<ReadonlySet<string>>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
}) {
  const refreshes = useMemo(() => new FolderRefreshes(), [sessionId, resetKey, cacheBinding]);
  const ownerRef = useRef(refreshes);
  ownerRef.current = refreshes;
  useEffect(() => refreshes.committed());
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;
    let current = true;
    const unsubscribe = client.on("session.workspace.file.changes", (msg) => {
      const changes = msg.payload?.changes;
      if (!changes || changes.length === 0) {
        if (isDebug()) debugChanges("event-empty", { sessionId });
        return;
      }
      if (isDebug())
        debugChanges("event", {
          sessionId,
          count: changes.length,
          expandedPaths: expandedPathsRef.current.size,
          firstPaths: changes.slice(0, 3).map((c: { path: string }) => c.path),
        });
      applyFileChanges({
        client,
        sessionId,
        expandedPaths: expandedPathsRef.current,
        changes,
        setTree,
        setLoadState,
        refreshes,
        isCurrent: () =>
          current && ownerRef.current === refreshes && (!cacheBinding || cacheBinding.isCurrent()),
      });
    });
    return () => {
      current = false;
      refreshes.retire();
      unsubscribe();
    };
  }, [sessionId, resetKey, cacheBinding, expandedPathsRef, setTree, setLoadState, refreshes]);
}

function useExpandedFileTree(tree: FileTreeNode | null) {
  const treeApi = useTree<FileTreeNode>({
    nodes: useMemo(() => sortRootChildren(tree), [tree]),
    getPath: FB_GET_PATH,
    getChildren: FB_GET_CHILDREN,
    isDir: FB_IS_DIR,
  });
  const expandedPaths = treeApi.expanded;
  const setExpandedPaths = treeApi.setExpanded;
  const visibleRows = treeApi.visibleRows;
  const expandedPathsRef = useRef<ReadonlySet<string>>(expandedPaths);
  expandedPathsRef.current = expandedPaths;
  return { treeApi, expandedPaths, setExpandedPaths, visibleRows, expandedPathsRef };
}

export function useFileBrowserTree(
  sessionId: string,
  resetKey?: string,
  cacheBinding?: FileTreeCacheBinding,
) {
  const effectiveResetKey = resetKey ?? sessionId;
  const { tree, setTree } = useFileTreeState(effectiveResetKey, cacheBinding);
  const { treeApi, expandedPaths, setExpandedPaths, visibleRows, expandedPathsRef } =
    useExpandedFileTree(tree);
  const [isLoadingTree, setIsLoadingTree] = useState(true);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const hasInitializedExpandedRef = useRef<string | null>(null);
  const restoreExpandedPathsRef = useRef<string[]>([]);
  const lastResetKeyRef = useRef<string | null>(null);
  const retryAttemptRef = useRef(0);
  const retryTimerRef = useRef<NodeJS.Timeout | null>(null);
  const agentctlStatus = useSessionAgentctl(sessionId);
  const { visibleLoadingPaths, showLoading, hideLoading, isLoading } = useLoadingTimers();
  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
  }, []);
  const { loadTree, readTree } = useTreeLoader({
    sessionId,
    effectiveResetKey,
    clearRetryTimer,
    retryAttemptRef,
    retryTimerRef,
    restoreExpandedPathsRef,
    hasInitializedExpandedRef,
    setTree,
    setExpandedPaths,
    setIsLoadingTree,
    setLoadState,
    setLoadError,
    cacheBinding,
  });
  const agentctlIsReadyRef = useRef(agentctlStatus.isReady);
  const loadStateRef = useRef(loadState);
  const treeRef = useRef(tree);
  agentctlIsReadyRef.current = agentctlStatus.isReady;
  loadStateRef.current = loadState;
  treeRef.current = tree;
  useTreeLoadEffects({
    sessionId,
    effectiveResetKey,
    agentctlIsReady: agentctlStatus.isReady,
    agentctlIsReadyRef,
    loadStateRef,
    treeRef,
    retryAttemptRef,
    hasInitializedExpandedRef,
    restoreExpandedPathsRef,
    expandedPathsRef,
    clearRetryTimer,
    loadTree,
    setTree,
    setIsLoadingTree,
    setLoadState,
    setLoadError,
    setExpandedPaths,
    lastResetKeyRef,
    cacheBinding,
  });
  useEffect(() => {
    if (isLoadingTree || hasInitializedExpandedRef.current !== effectiveResetKey) return;
    setFilesPanelExpandedPaths(effectiveResetKey, Array.from(expandedPaths));
  }, [expandedPaths, effectiveResetKey, isLoadingTree]);
  useFileChangeSubscription({
    sessionId,
    resetKey: effectiveResetKey,
    cacheBinding,
    expandedPathsRef,
    setTree,
    setLoadState,
  });
  return useMemoizedFileBrowserTreeResult({
    tree,
    setTree,
    expandedPaths,
    setExpandedPaths,
    visibleRows,
    visibleLoadingPaths,
    isLoadingTree,
    loadState,
    loadError,
    loadTree,
    readTree,
    showLoading,
    hideLoading,
    isLoading,
    collapseAll: treeApi.collapseAll,
  });
}

export {
  useScrollPersistence,
  loadNodeChildren,
  toggleFolderExpand,
  fetchAndOpenFile,
} from "./file-browser-actions";
export type { ToggleFolderExpandDeps } from "./file-browser-actions";
