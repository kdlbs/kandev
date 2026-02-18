'use client';

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { ScrollArea } from '@kandev/ui/scroll-area';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileTree, requestFileContent, searchWorkspaceFiles } from '@/lib/ws/workspace-files';
import type { FileTreeNode, FileContentResponse, OpenFileTab } from '@/lib/types/backend';
import { useSessionAgentctl } from '@/hooks/domains/session/use-session-agentctl';
import { useSession } from '@/hooks/domains/session/use-session';
import { useOpenSessionFolder } from '@/hooks/use-open-session-folder';
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard';
import {
  getFilesPanelExpandedPaths,
  setFilesPanelExpandedPaths,
  getFilesPanelScrollPosition,
  setFilesPanelScrollPosition,
} from '@/lib/local-storage';
import { useToast } from '@/components/toast-provider';
import {
  mergeTreeNodes,
  insertNodeInTree,
  removeNodeFromTree,
  FileBrowserSearchHeader,
  FileBrowserToolbar,
  FileBrowserContentArea,
} from './file-browser-parts';

const MAX_RETRY_ATTEMPTS = 4;
const RETRY_DELAYS_MS = [1000, 2000, 5000, 10000];

type LoadState = 'loading' | 'waiting' | 'loaded' | 'manual' | 'error';

/** Hook encapsulating file search state and handlers. */
function useFileBrowserSearch(sessionId: string) {
  const [isSearchActive, setIsSearchActive] = useState(false);
  const [localSearchQuery, setLocalSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<string[] | null>(null);
  const [isSearching, setIsSearching] = useState(false);
  const searchTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Focus search input when search opens
  useEffect(() => {
    if (isSearchActive && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [isSearchActive]);

  // Clear search when closing
  useEffect(() => {
    if (!isSearchActive) {
      setLocalSearchQuery('');
      setSearchResults(null);
      setIsSearching(false);
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    }
  }, [isSearchActive]);

  // Cleanup on unmount
  useEffect(() => {
    return () => { if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current); };
  }, []);

  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
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
        console.error('Failed to search files:', error);
        setSearchResults([]);
      } finally {
        setIsSearching(false);
      }
    }, 300);
  }, [sessionId]);

  const handleCloseSearch = useCallback(() => {
    setIsSearchActive(false);
    setLocalSearchQuery('');
    setSearchResults(null);
    setIsSearching(false);
    if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
  }, []);

  return {
    isSearchActive, setIsSearchActive, localSearchQuery, searchResults,
    isSearching, searchInputRef, handleSearchChange, handleCloseSearch,
  };
}

/** Apply incoming file changes to the tree by refreshing affected folders. */
function applyFileChanges(ctx: {
  client: ReturnType<typeof getWebSocketClient>;
  sessionId: string;
  expandedPaths: Set<string>;
  changes: Array<{ path: string }>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
}) {
  const { client, sessionId, expandedPaths, changes, setTree, setLoadState } = ctx;
  const candidates = new Set<string>();
  for (const change of changes) {
    const p = change.path;
    const lastSlash = p.lastIndexOf('/');
    candidates.add(lastSlash === -1 ? '' : p.substring(0, lastSlash));
    candidates.add(p);
  }
  const foldersToRefresh = new Set<string>();
  for (const c of candidates) {
    if (c === '' || expandedPaths.has(c)) foldersToRefresh.add(c);
  }
  if (foldersToRefresh.size === 0) return;

  void (async () => {
    try {
      const folderUpdates = new Map<string, FileTreeNode[] | undefined>();
      await Promise.all(Array.from(foldersToRefresh).map(async (folder) => {
        try {
          const res = await requestFileTree(client!, sessionId, folder || '', 1);
          folderUpdates.set(folder, res.root?.children);
        } catch { /* Folder may have been removed */ }
      }));
      setTree((prev) => {
        if (!prev) return prev;
        let updated = prev;
        if (folderUpdates.has('')) {
          const freshRootChildren = folderUpdates.get('');
          const existingByPath = new Map((updated.children ?? []).map((c) => [c.path, c]));
          const mergedRootChildren = freshRootChildren?.map((incoming) => {
            const existing = existingByPath.get(incoming.path);
            return (existing && existing.is_dir && incoming.is_dir) ? mergeTreeNodes(existing, incoming) : incoming;
          });
          updated = { ...updated, children: mergedRootChildren };
        }
        const subFolders = Array.from(folderUpdates.keys()).filter((k) => k !== '');
        if (subFolders.length === 0) return updated;
        const patchNode = (node: FileTreeNode): FileTreeNode => {
          if (node.is_dir && folderUpdates.has(node.path)) {
            return { ...node, children: folderUpdates.get(node.path)?.map(patchNode) };
          }
          return node.children ? { ...node, children: node.children.map(patchNode) } : node;
        };
        return patchNode(updated);
      });
      setLoadState('loaded');
    } catch (error) {
      console.error('[FileBrowser] Failed to refresh file tree:', error);
    }
  })();
}

/** Hook for tree loading with retry logic, file-change subscription, and expanded state. */
function useFileBrowserTree(sessionId: string) {
  const [tree, setTree] = useState<FileTreeNode | null>(null);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [visibleLoadingPaths, setVisibleLoadingPaths] = useState<Set<string>>(new Set());
  const [isLoadingTree, setIsLoadingTree] = useState(true);
  const [loadState, setLoadState] = useState<LoadState>('loading');
  const [loadError, setLoadError] = useState<string | null>(null);
  const loadingTimersRef = useRef<Map<string, NodeJS.Timeout>>(new Map());
  const hasInitializedExpandedRef = useRef<string | null>(null);
  const retryAttemptRef = useRef(0);
  const retryTimerRef = useRef<NodeJS.Timeout | null>(null);
  const loadInFlightRef = useRef(false);
  const agentctlStatus = useSessionAgentctl(sessionId);

  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current) { clearTimeout(retryTimerRef.current); retryTimerRef.current = null; }
  }, []);

  const showLoading = useCallback((path: string) => {
    const timer = setTimeout(() => {
      setVisibleLoadingPaths((prev) => new Set(prev).add(path));
      loadingTimersRef.current.delete(path);
    }, 150);
    loadingTimersRef.current.set(path, timer);
  }, []);

  const hideLoading = useCallback((path: string) => {
    const timer = loadingTimersRef.current.get(path);
    if (timer) { clearTimeout(timer); loadingTimersRef.current.delete(path); }
    setVisibleLoadingPaths((prev) => { const next = new Set(prev); next.delete(path); return next; });
  }, []);

  const loadTree = useCallback(async (options?: { resetRetry?: boolean }) => {
    if (loadInFlightRef.current) return;
    loadInFlightRef.current = true;
    setIsLoadingTree(true);
    setLoadState('loading');
    setLoadError(null);
    if (options?.resetRetry) { retryAttemptRef.current = 0; clearRetryTimer(); }
    try {
      const client = getWebSocketClient();
      if (!client) throw new Error('WebSocket client not available');
      const response = await requestFileTree(client, sessionId, '', 1);
      setTree(response.root ?? null);
      setLoadState('loaded');
      retryAttemptRef.current = 0;
      clearRetryTimer();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to load file tree';
      setLoadError(message);
      if (retryAttemptRef.current < MAX_RETRY_ATTEMPTS) {
        const delay = RETRY_DELAYS_MS[Math.min(retryAttemptRef.current, RETRY_DELAYS_MS.length - 1)];
        retryAttemptRef.current += 1;
        setLoadState('waiting');
        clearRetryTimer();
        retryTimerRef.current = setTimeout(() => { void loadTree(); }, delay);
      } else {
        setLoadState('manual');
      }
    } finally {
      setIsLoadingTree(false);
      loadInFlightRef.current = false;
    }
  }, [clearRetryTimer, sessionId]);

  // Load initial tree and cleanup
  useEffect(() => {
    setTree(null); setIsLoadingTree(true); setLoadState('loading'); setLoadError(null);
    retryAttemptRef.current = 0; clearRetryTimer(); hasInitializedExpandedRef.current = null;
    const savedPaths = getFilesPanelExpandedPaths(sessionId);
    setExpandedPaths(savedPaths.length > 0 ? new Set(savedPaths) : new Set());
    if (savedPaths.length > 0) hasInitializedExpandedRef.current = sessionId;
    void loadTree({ resetRetry: true });
    return () => { clearRetryTimer(); };
  }, [clearRetryTimer, loadTree, sessionId]);

  // When agentctl becomes ready, retry loading
  useEffect(() => {
    if (!agentctlStatus.isReady || loadState === 'loaded') return;
    void loadTree({ resetRetry: true });
  }, [agentctlStatus.isReady, loadState, loadTree]);

  // Auto-expand first level folders
  useEffect(() => {
    if (!tree || isLoadingTree || hasInitializedExpandedRef.current === sessionId) return;
    hasInitializedExpandedRef.current = sessionId;
    setExpandedPaths(new Set());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tree?.children?.length, isLoadingTree, sessionId]);

  // Save expanded paths when they change
  useEffect(() => {
    if (expandedPaths.size > 0 || hasInitializedExpandedRef.current === sessionId) {
      setFilesPanelExpandedPaths(sessionId, Array.from(expandedPaths));
    }
  }, [expandedPaths, sessionId]);

  // Subscribe to file changes and refresh only affected folders
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;
    return client.on('session.workspace.file.changes', (msg) => {
      const changes = msg.payload?.changes;
      if (!changes || changes.length === 0) return;
      applyFileChanges({ client, sessionId, expandedPaths, changes, setTree, setLoadState });
    });
  }, [sessionId, expandedPaths]);

  const collapseAll = useCallback(() => { setExpandedPaths(new Set()); }, []);

  return {
    tree, setTree, expandedPaths, setExpandedPaths,
    visibleLoadingPaths, isLoadingTree, loadState, loadError, loadTree,
    showLoading, hideLoading, collapseAll,
  };
}

/** Hook for scroll position persistence in the file browser.
 * Restores scroll on tree load and saves position on scroll.
 * Must be given the scrollAreaRef owned by the calling component. */
function useScrollPersistence(
  sessionId: string,
  isTreeLoaded: boolean,
  scrollAreaRef: React.RefObject<HTMLDivElement | null>,
  tree: FileTreeNode | null,
) {
  const scrollSaveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const hasRestoredScrollRef = useRef<string | null>(null);

  // Restore scroll position after tree loads
  useEffect(() => {
    if (!isTreeLoaded || hasRestoredScrollRef.current === sessionId) return;
    const savedScroll = getFilesPanelScrollPosition(sessionId);
    if (savedScroll > 0 && scrollAreaRef.current) {
      const viewport = scrollAreaRef.current.querySelector('[data-radix-scroll-area-viewport]');
      if (viewport) { viewport.scrollTop = savedScroll; hasRestoredScrollRef.current = sessionId; }
    } else {
      hasRestoredScrollRef.current = sessionId;
    }
  }, [isTreeLoaded, sessionId, scrollAreaRef]);

  // Attach scroll listener to ScrollArea viewport
  useEffect(() => {
    const el = scrollAreaRef.current;
    if (!el) return;
    const viewport = el.querySelector('[data-radix-scroll-area-viewport]');
    if (!viewport) return;
    const onScroll = (event: Event) => {
      const target = event.target as HTMLElement;
      if (scrollSaveTimeoutRef.current) clearTimeout(scrollSaveTimeoutRef.current);
      scrollSaveTimeoutRef.current = setTimeout(() => {
        setFilesPanelScrollPosition(sessionId, target.scrollTop);
      }, 150);
    };
    viewport.addEventListener('scroll', onScroll);
    return () => {
      viewport.removeEventListener('scroll', onScroll);
      if (scrollSaveTimeoutRef.current) clearTimeout(scrollSaveTimeoutRef.current);
    };
  }, [sessionId, tree, scrollAreaRef]);
}

/** Fetch children for a folder node if not already loaded. */
async function loadNodeChildren(
  node: FileTreeNode,
  sessionId: string,
  treeState: ReturnType<typeof useFileBrowserTree>,
) {
  if (node.children && node.children.length > 0) return;
  treeState.showLoading(node.path);
  try {
    const client = getWebSocketClient();
    if (!client) return;
    const response = await requestFileTree(client, sessionId, node.path, 1);
    const updateNode = (n: FileTreeNode): FileTreeNode => {
      if (n.path === node.path) return { ...n, children: response.root.children };
      return n.children ? { ...n, children: n.children.map(updateNode) } : n;
    };
    if (treeState.tree) treeState.setTree(updateNode(treeState.tree));
  } catch (error) {
    console.error('Failed to load children:', error);
  } finally {
    treeState.hideLoading(node.path);
  }
}

/** Fetch and open a file by path. */
async function fetchAndOpenFile(
  sessionId: string,
  path: string,
  onOpenFile: (file: OpenFileTab) => void,
  toast: ReturnType<typeof useToast>['toast'],
) {
  try {
    const client = getWebSocketClient();
    if (!client) return;
    const response: FileContentResponse = await requestFileContent(client, sessionId, path);
    const { calculateHash } = await import('@/lib/utils/file-diff');
    const hash = await calculateHash(response.content);
    const name = path.split('/').pop() || path;
    onOpenFile({ path, name, content: response.content, originalContent: response.content, originalHash: hash, isDirty: false, isBinary: response.is_binary });
  } catch (error) {
    const reason = error instanceof Error ? error.message : 'Unknown error';
    toast({ title: 'Failed to open file', description: reason, variant: 'error' });
  }
}

type FileBrowserProps = {
  sessionId: string;
  onOpenFile: (file: OpenFileTab) => void;
  onCreateFile?: (path: string) => Promise<boolean>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  activeFilePath?: string | null;
};

export function FileBrowser({ sessionId, onOpenFile, onCreateFile, onDeleteFile, activeFilePath }: FileBrowserProps) {
  const { toast } = useToast();
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const { open: openFolder } = useOpenSessionFolder(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);
  const [creatingInPath, setCreatingInPath] = useState<string | null>(null);
  const [activeFolderPath, setActiveFolderPath] = useState<string>('');
  const scrollAreaRef = useRef<HTMLDivElement>(null);

  const search = useFileBrowserSearch(sessionId);
  const treeState = useFileBrowserTree(sessionId);
  const isTreeLoaded = !treeState.isLoadingTree && treeState.tree !== null;
  useScrollPersistence(sessionId, isTreeLoaded, scrollAreaRef, treeState.tree);
  // Workspace path for header
  const fullPath = session?.worktree_path ?? '';
  const displayPath = fullPath.replace(/^\/(?:Users|home)\/[^/]+\//, '~/');

  const handleStartCreate = useCallback(() => {
    if (activeFolderPath && !treeState.expandedPaths.has(activeFolderPath)) {
      treeState.setExpandedPaths((prev) => new Set(prev).add(activeFolderPath));
    }
    setCreatingInPath(activeFolderPath);
  }, [activeFolderPath, treeState]);

  const handleCreateFileSubmit = useCallback((parentPath: string, name: string) => {
    setCreatingInPath(null);
    const newPath = parentPath ? `${parentPath}/${name}` : name;
    const newNode: FileTreeNode = { name, path: newPath, is_dir: false, size: 0 };
    treeState.setTree((prev) => (prev ? insertNodeInTree(prev, parentPath, newNode) : prev));
    onCreateFile?.(newPath).then((ok) => {
      if (!ok) treeState.setTree((prev) => (prev ? removeNodeFromTree(prev, newPath) : prev));
    }).catch(() => {
      treeState.setTree((prev) => (prev ? removeNodeFromTree(prev, newPath) : prev));
    });
  }, [onCreateFile, treeState]);

  const toggleExpand = useCallback(async (node: FileTreeNode) => {
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
  }, [treeState, sessionId]);

  const openFileByPath = useCallback((path: string) => fetchAndOpenFile(sessionId, path, onOpenFile, toast), [sessionId, onOpenFile, toast]);
  const handleCancelCreate = useCallback(() => { setCreatingInPath(null); }, []);

  return (
    <div className="flex flex-col h-full">
      {treeState.tree && treeState.loadState === 'loaded' && (
        search.isSearchActive ? (
          <FileBrowserSearchHeader
            isSearching={search.isSearching}
            localSearchQuery={search.localSearchQuery}
            searchInputRef={search.searchInputRef}
            onSearchChange={search.handleSearchChange}
            onCloseSearch={search.handleCloseSearch}
          />
        ) : (
          <FileBrowserToolbar
            displayPath={displayPath}
            fullPath={fullPath}
            copied={copied}
            expandedPathsSize={treeState.expandedPaths.size}
            onCopyPath={copyPath}
            onStartCreate={onCreateFile ? handleStartCreate : undefined}
            onOpenFolder={() => openFolder()}
            onStartSearch={() => search.setIsSearchActive(true)}
            onCollapseAll={treeState.collapseAll}
            showCreateButton={Boolean(onCreateFile)}
          />
        )
      )}

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
