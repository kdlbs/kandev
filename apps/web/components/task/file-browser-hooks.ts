"use client";

import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileTree, searchWorkspaceFiles } from "@/lib/ws/workspace-files";
import type { FileTreeNode } from "@/lib/types/backend";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { getFilesPanelExpandedPaths, setFilesPanelExpandedPaths } from "@/lib/local-storage";
import { useTree, type VisibleRow } from "@/hooks/use-tree";
import { mergeTreeNodes } from "./file-browser-parts";
import { compareTreeNodes, sortRootChildren } from "./file-tree-utils";
import { restoredExpandedPaths } from "./file-browser-restore";
import { useTreeLoader } from "./file-browser-tree-loader";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const debugLoad = createDebugLogger("file-browser:load");
const debugChanges = createDebugLogger("file-browser:changes");

const FB_GET_PATH = (n: FileTreeNode) => n.path;
const FB_GET_CHILDREN = (n: FileTreeNode) =>
  n.children ? [...n.children].sort(compareTreeNodes) : undefined;
const FB_IS_DIR = (n: FileTreeNode) => n.is_dir;

export type FileBrowserRow = VisibleRow<FileTreeNode>;

export type LoadState = "loading" | "waiting" | "loaded" | "manual" | "error";

/** Hook encapsulating file search state and handlers. */
export function useFileBrowserSearch(sessionId: string) {
  const [isSearchActive, setIsSearchActive] = useState(false);
  const [localSearchQuery, setLocalSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<string[] | null>(null);
  const [isSearching, setIsSearching] = useState(false);
  const searchTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isSearchActive && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [isSearchActive]);

  useEffect(() => {
    if (!isSearchActive) {
      setLocalSearchQuery("");
      setSearchResults(null);
      setIsSearching(false);
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    }
  }, [isSearchActive]);

  useEffect(() => {
    return () => {
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, []);

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      setLocalSearchQuery(value);
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
      if (!value.trim()) {
        setSearchResults(null);
        setIsSearching(false);
        return;
      }
      setIsSearching(true);
      searchTimeoutRef.current = setTimeout(async () => {
        try {
          const client = getWebSocketClient();
          if (!client) return;
          const response = await searchWorkspaceFiles(client, sessionId, value, 50);
          setSearchResults(response.files || []);
        } catch (error) {
          console.error("Failed to search files:", error);
          setSearchResults([]);
        } finally {
          setIsSearching(false);
        }
      }, 300);
    },
    [sessionId],
  );

  const handleCloseSearch = useCallback(() => {
    setIsSearchActive(false);
    setLocalSearchQuery("");
    setSearchResults(null);
    setIsSearching(false);
    if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
  }, []);

  return {
    isSearchActive,
    setIsSearchActive,
    localSearchQuery,
    searchResults,
    isSearching,
    searchInputRef,
    handleSearchChange,
    handleCloseSearch,
  };
}

/** Apply incoming file changes to the tree by refreshing affected folders. */
export function applyFileChanges(ctx: {
  client: ReturnType<typeof getWebSocketClient>;
  sessionId: string;
  expandedPaths: ReadonlySet<string>;
  changes: Array<{ path: string; operation?: string; repository_name?: string }>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
}) {
  const { client, sessionId, expandedPaths, changes, setTree, setLoadState } = ctx;
  const foldersToRefresh = new Set<string>();
  for (const change of changes) {
    if (change.operation === "refresh") {
      foldersToRefresh.add("");
      const repo = change.repository_name;
      for (const exp of expandedPaths) {
        if (!repo || exp === repo || exp.startsWith(repo + "/")) {
          foldersToRefresh.add(exp);
        }
      }
      continue;
    }
    const p = change.path;
    const lastSlash = p.lastIndexOf("/");
    const parent = lastSlash === -1 ? "" : p.substring(0, lastSlash);
    if (parent === "" || expandedPaths.has(parent)) foldersToRefresh.add(parent);
    if (p === "" || expandedPaths.has(p)) foldersToRefresh.add(p);
  }
  if (foldersToRefresh.size === 0) {
    if (isDebug())
      debugChanges("no-folders-to-refresh", {
        sessionId,
        candidates: changes.length,
        expandedPaths: expandedPaths.size,
      });
    return;
  }
  if (isDebug())
    debugChanges("refresh", {
      sessionId,
      folders: Array.from(foldersToRefresh).slice(0, 5),
      total: foldersToRefresh.size,
    });

  void (async () => {
    try {
      const folderUpdates = new Map<string, FileTreeNode[] | undefined>();
      await Promise.all(
        Array.from(foldersToRefresh).map(async (folder) => {
          try {
            const res = await requestFileTree(client!, sessionId, folder || "", 1);
            folderUpdates.set(folder, res.root?.children);
          } catch {
            /* Folder may have been removed */
          }
        }),
      );
      setTree((prev) => {
        if (!prev) return prev;
        let updated = prev;
        if (folderUpdates.has("")) {
          const freshRootChildren = folderUpdates.get("");
          const existingByPath = new Map((updated.children ?? []).map((c) => [c.path, c]));
          const mergedRootChildren = freshRootChildren?.map((incoming) => {
            const existing = existingByPath.get(incoming.path);
            return existing && existing.is_dir && incoming.is_dir
              ? mergeTreeNodes(existing, incoming)
              : incoming;
          });
          updated = { ...updated, children: mergedRootChildren };
        }
        const subFolders = Array.from(folderUpdates.keys()).filter((k) => k !== "");
        if (subFolders.length === 0) return updated;
        const patchNode = (node: FileTreeNode): FileTreeNode => {
          if (node.is_dir && folderUpdates.has(node.path)) {
            return { ...node, children: folderUpdates.get(node.path)?.map(patchNode) };
          }
          return node.children ? { ...node, children: node.children.map(patchNode) } : node;
        };
        return { ...updated, children: updated.children?.map(patchNode) };
      });
      setLoadState("loaded");
    } catch (error) {
      console.error("[FileBrowser] Failed to refresh file tree:", error);
    }
  })();
}

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
    clearRetryTimer,
    loadTree,
    setTree,
    setIsLoadingTree,
    setLoadState,
    setLoadError,
    setExpandedPaths,
    lastResetKeyRef,
  } = ctx;

  useEffect(() => {
    const resetKeyChanged = lastResetKeyRef.current !== effectiveResetKey;
    lastResetKeyRef.current = effectiveResetKey;
    clearRetryTimer();
    retryAttemptRef.current = 0;
    if (resetKeyChanged) {
      setTree(null);
      setIsLoadingTree(true);
      setLoadState(agentctlIsReadyRef.current ? "loading" : "waiting");
      setLoadError(null);
      hasInitializedExpandedRef.current = null;
      const savedPaths = restoredExpandedPaths(getFilesPanelExpandedPaths(effectiveResetKey));
      restoreExpandedPathsRef.current = savedPaths;
      setExpandedPaths(savedPaths.length > 0 ? new Set(savedPaths) : new Set());
    }
    const savedPaths = restoreExpandedPathsRef.current;
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
  }, [clearRetryTimer, loadTree, effectiveResetKey, sessionId, setExpandedPaths]);

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
  sessionIdRef,
  expandedPathsRef,
  setTree,
  setLoadState,
}: {
  sessionIdRef: React.MutableRefObject<string>;
  expandedPathsRef: React.MutableRefObject<ReadonlySet<string>>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
}) {
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;
    return client.on("session.workspace.file.changes", (msg) => {
      const changes = msg.payload?.changes;
      if (!changes || changes.length === 0) {
        if (isDebug()) debugChanges("event-empty", { sessionId: sessionIdRef.current });
        return;
      }
      if (isDebug())
        debugChanges("event", {
          sessionId: sessionIdRef.current,
          count: changes.length,
          expandedPaths: expandedPathsRef.current.size,
          firstPaths: changes.slice(0, 3).map((c: { path: string }) => c.path),
        });
      applyFileChanges({
        client,
        sessionId: sessionIdRef.current,
        expandedPaths: expandedPathsRef.current,
        changes,
        setTree,
        setLoadState,
      });
    });
  }, [sessionIdRef, expandedPathsRef, setTree, setLoadState]);
}

export function useFileBrowserTree(sessionId: string, resetKey?: string) {
  const effectiveResetKey = resetKey ?? sessionId;
  const [tree, setTree] = useState<FileTreeNode | null>(null);
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
  const [isLoadingTree, setIsLoadingTree] = useState(true);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [loadError, setLoadError] = useState<string | null>(null);
  const hasInitializedExpandedRef = useRef<string | null>(null);
  const restoreExpandedPathsRef = useRef<string[]>([]);
  const lastResetKeyRef = useRef<string | null>(null);
  const retryAttemptRef = useRef(0);
  const retryTimerRef = useRef<NodeJS.Timeout | null>(null);
  const sessionIdRef = useRef(sessionId);
  sessionIdRef.current = sessionId;
  const agentctlStatus = useSessionAgentctl(sessionId);
  const { visibleLoadingPaths, showLoading, hideLoading, isLoading } = useLoadingTimers();
  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
  }, []);
  const loadTree = useTreeLoader({
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
    clearRetryTimer,
    loadTree,
    setTree,
    setIsLoadingTree,
    setLoadState,
    setLoadError,
    setExpandedPaths,
    lastResetKeyRef,
  });
  useEffect(() => {
    if (isLoadingTree || hasInitializedExpandedRef.current !== effectiveResetKey) return;
    setFilesPanelExpandedPaths(effectiveResetKey, Array.from(expandedPaths));
  }, [expandedPaths, effectiveResetKey, isLoadingTree]);
  useFileChangeSubscription({ sessionIdRef, expandedPathsRef, setTree, setLoadState });
  return {
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
    showLoading,
    hideLoading,
    isLoading,
    collapseAll: treeApi.collapseAll,
  };
}

export {
  useScrollPersistence,
  loadNodeChildren,
  toggleFolderExpand,
  fetchAndOpenFile,
} from "./file-browser-actions";
export type { ToggleFolderExpandDeps } from "./file-browser-actions";
