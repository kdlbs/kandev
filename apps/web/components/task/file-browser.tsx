'use client';

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { IconChevronRight, IconChevronDown, IconFolder, IconFolderOpen, IconSearch, IconX, IconLoader2, IconTrash, IconRefresh, IconListTree, IconFolderShare, IconCopy, IconCheck, IconPlus } from '@tabler/icons-react';
import { ScrollArea } from '@kandev/ui/scroll-area';
import { Input } from '@kandev/ui/input';
import { FileIcon } from '@/components/ui/file-icon';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
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
import { InlineFileInput } from './inline-file-input';
import { PanelHeaderBar, PanelHeaderBarSplit } from './panel-primitives';

/**
 * Merge a freshly-fetched tree node into an existing one, preserving
 * already-loaded children so expanded folders don't collapse.
 */
function mergeTreeNodes(existing: FileTreeNode, incoming: FileTreeNode): FileTreeNode {
  // If the incoming node has no children list, keep existing subtree
  if (!incoming.children) return { ...existing, ...incoming, children: existing.children };

  // If existing has no children, just use incoming
  if (!existing.children) return incoming;

  // Build a lookup of existing children by path
  const existingByPath = new Map(existing.children.map((c) => [c.path, c]));

  const mergedChildren = incoming.children.map((inChild) => {
    const exChild = existingByPath.get(inChild.path);
    if (exChild && exChild.is_dir && inChild.is_dir) {
      // Recursively merge directories to keep loaded subtrees
      return mergeTreeNodes(exChild, inChild);
    }
    return inChild;
  });

  return { ...existing, ...incoming, children: mergedChildren };
}

/** Sort comparator: directories first, then alphabetical by name. */
const compareTreeNodes = (a: FileTreeNode, b: FileTreeNode): number => {
  if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
  return a.name.localeCompare(b.name);
};

/** Insert a file node into a parent folder, keeping children sorted (dirs first, then alpha). */
function insertNodeInTree(root: FileTreeNode, parentPath: string, node: FileTreeNode): FileTreeNode {
  if (root.path === parentPath || (parentPath === '' && root.path === '')) {
    const children = [...(root.children ?? []), node].sort(compareTreeNodes);
    return { ...root, children };
  }
  if (!root.children) return root;
  return { ...root, children: root.children.map((c) => insertNodeInTree(c, parentPath, node)) };
}

/** Remove a node by path from the tree. */
function removeNodeFromTree(root: FileTreeNode, targetPath: string): FileTreeNode {
  if (!root.children) return root;
  const filtered = root.children.filter((c) => c.path !== targetPath);
  return { ...root, children: filtered.map((c) => removeNodeFromTree(c, targetPath)) };
}

const MAX_RETRY_ATTEMPTS = 4;
const RETRY_DELAYS_MS = [1000, 2000, 5000, 10000];

type FileBrowserProps = {
  sessionId: string;
  onOpenFile: (file: OpenFileTab) => void;
  onCreateFile?: (path: string) => Promise<boolean>;
  onDeleteFile?: (path: string) => Promise<boolean>;
  activeFilePath?: string | null;
};

export function FileBrowser({ sessionId, onOpenFile, onCreateFile, onDeleteFile, activeFilePath }: FileBrowserProps) {
  const { toast } = useToast();
  const [tree, setTree] = useState<FileTreeNode | null>(null);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [creatingInPath, setCreatingInPath] = useState<string | null>(null);
  const [activeFolderPath, setActiveFolderPath] = useState<string>('');
  const [visibleLoadingPaths, setVisibleLoadingPaths] = useState<Set<string>>(new Set());
  const loadingTimersRef = useRef<Map<string, NodeJS.Timeout>>(new Map());
  const [isLoadingTree, setIsLoadingTree] = useState(true);
  const [loadState, setLoadState] = useState<'loading' | 'waiting' | 'loaded' | 'manual' | 'error'>('loading');
  const [loadError, setLoadError] = useState<string | null>(null);
  const agentctlStatus = useSessionAgentctl(sessionId);
  const { session, isFailed: isSessionFailed, errorMessage: sessionError } = useSession(sessionId);
  const { open: openFolder } = useOpenSessionFolder(sessionId);
  const { copied, copy: copyPath } = useCopyToClipboard(1000);

  // Search state
  const [isSearchActive, setIsSearchActive] = useState(false);
  const [localSearchQuery, setLocalSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<string[] | null>(null);
  const [isSearching, setIsSearching] = useState(false);
  const searchTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Scroll persistence
  const scrollAreaRef = useRef<HTMLDivElement>(null);
  const scrollSaveTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const hasRestoredScrollRef = useRef<string | null>(null);
  const hasInitializedExpandedRef = useRef<string | null>(null);
  const retryAttemptRef = useRef(0);
  const retryTimerRef = useRef<NodeJS.Timeout | null>(null);
  const loadInFlightRef = useRef(false);

  // Workspace path for header
  const fullPath = session?.worktree_path ?? '';
  const displayPath = fullPath.replace(/^\/(?:Users|home)\/[^/]+\//, '~/');

  const collapseAll = useCallback(() => {
    setExpandedPaths(new Set());
  }, []);

  const handleStartCreate = useCallback(() => {
    const folderPath = activeFolderPath;
    // Expand the folder if it's collapsed
    if (folderPath && !expandedPaths.has(folderPath)) {
      setExpandedPaths((prev) => new Set(prev).add(folderPath));
    }
    setCreatingInPath(folderPath);
  }, [activeFolderPath, expandedPaths]);

  const handleCreateFileSubmit = useCallback((parentPath: string, name: string) => {
    setCreatingInPath(null);
    const newPath = parentPath ? `${parentPath}/${name}` : name;

    // Optimistically insert the new file node
    const newNode: FileTreeNode = { name, path: newPath, is_dir: false, size: 0 };
    setTree((prev) => {
      if (!prev) return prev;
      return insertNodeInTree(prev, parentPath, newNode);
    });

    // Fire the actual request; revert on failure
    onCreateFile?.(newPath).then((ok) => {
      if (!ok) {
        setTree((prev) => (prev ? removeNodeFromTree(prev, newPath) : prev));
      }
    }).catch(() => {
      setTree((prev) => (prev ? removeNodeFromTree(prev, newPath) : prev));
    });
  }, [onCreateFile]);

  // Show loading spinner only after 150ms to avoid flash on fast loads
  const showLoading = useCallback((path: string) => {
    const timer = setTimeout(() => {
      setVisibleLoadingPaths((prev) => new Set(prev).add(path));
      loadingTimersRef.current.delete(path);
    }, 150);
    loadingTimersRef.current.set(path, timer);
  }, []);

  const hideLoading = useCallback((path: string) => {
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
      if (searchTimeoutRef.current) {
        clearTimeout(searchTimeoutRef.current);
      }
    }
  }, [isSearchActive]);

  const clearRetryTimer = useCallback(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
  }, []);

  const loadTree = useCallback(async (options?: { resetRetry?: boolean }) => {
    if (loadInFlightRef.current) return;
    loadInFlightRef.current = true;
    setIsLoadingTree(true);
    setLoadState('loading');
    setLoadError(null);

    if (options?.resetRetry) {
      retryAttemptRef.current = 0;
      clearRetryTimer();
    }

    try {
      const client = getWebSocketClient();
      if (!client) {
        throw new Error('WebSocket client not available');
      }

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
        retryTimerRef.current = setTimeout(() => {
          void loadTree();
        }, delay);
      } else {
        setLoadState('manual');
      }
    } finally {
      setIsLoadingTree(false);
      loadInFlightRef.current = false;
    }
  }, [clearRetryTimer, sessionId]);

  // Load initial tree - always try to load, don't wait for agentctl ready status
  useEffect(() => {
    setTree(null);
    setIsLoadingTree(true);
    setLoadState('loading');
    setLoadError(null);
    retryAttemptRef.current = 0;
    clearRetryTimer();
    // Reset the auto-expand ref so it will run for the new session
    hasInitializedExpandedRef.current = null;

    // Restore expanded paths from session storage or reset
    const savedPaths = getFilesPanelExpandedPaths(sessionId);
    if (savedPaths.length > 0) {
      setExpandedPaths(new Set(savedPaths));
      // Mark as initialized only if we have saved paths to restore
      hasInitializedExpandedRef.current = sessionId;
    } else {
      setExpandedPaths(new Set());
      // Leave hasInitializedExpandedRef.current as null so auto-expand will run
    }

    void loadTree({ resetRetry: true });
  }, [clearRetryTimer, loadTree, sessionId]);

  // When agentctl becomes ready, retry loading the tree
  useEffect(() => {
    if (!agentctlStatus.isReady) return;
    if (loadState === 'loaded') return;
    void loadTree({ resetRetry: true });
  }, [agentctlStatus.isReady, loadState, loadTree]);

  // Auto-expand first level folders and load their children
  // Note: We intentionally use tree?.children?.length instead of tree to avoid re-running when tree updates
  useEffect(() => {
    if (!tree || isLoadingTree) {
      return;
    }

    // Only auto-expand if we haven't initialized from saved state
    if (hasInitializedExpandedRef.current === sessionId) {
      return;
    }
    hasInitializedExpandedRef.current = sessionId;

    // Root is no longer a visible node, start with no expanded paths
    setExpandedPaths(new Set());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tree?.children?.length, isLoadingTree, sessionId]);

  // Save expanded paths when they change
  useEffect(() => {
    if (expandedPaths.size > 0 || hasInitializedExpandedRef.current === sessionId) {
      setFilesPanelExpandedPaths(sessionId, Array.from(expandedPaths));
    }
  }, [expandedPaths, sessionId]);

  // Track if tree is loaded (used for scroll restore timing)
  const isTreeLoaded = !isLoadingTree && tree !== null;

  // Restore scroll position after tree loads
  useEffect(() => {
    if (!isTreeLoaded || hasRestoredScrollRef.current === sessionId) return;

    const savedScroll = getFilesPanelScrollPosition(sessionId);
    if (savedScroll > 0 && scrollAreaRef.current) {
      // Find the viewport element within ScrollArea
      const viewport = scrollAreaRef.current.querySelector('[data-radix-scroll-area-viewport]');
      if (viewport) {
        viewport.scrollTop = savedScroll;
        hasRestoredScrollRef.current = sessionId;
      }
    } else {
      hasRestoredScrollRef.current = sessionId;
    }
  }, [isTreeLoaded, sessionId]);

  // Subscribe to file changes and refresh only affected folders
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;

    const unsubscribe = client.on('session.workspace.file.changes', async (msg) => {
      const changes = msg.payload?.changes;
      if (!changes || changes.length === 0) return;

      // Collect candidate folders that may need refreshing:
      // - the parent of each changed path (file created/deleted in that folder)
      // - the path itself (in case backend reports a directory change)
      const candidates = new Set<string>();
      for (const change of changes) {
        const p = change.path;
        // Parent folder
        const lastSlash = p.lastIndexOf('/');
        candidates.add(lastSlash === -1 ? '' : p.substring(0, lastSlash));
        // The path itself (could be a directory)
        candidates.add(p);
      }

      // Only refresh folders we actually have loaded: root ('') or expanded
      const foldersToRefresh = new Set<string>();
      for (const c of candidates) {
        if (c === '' || expandedPaths.has(c)) {
          foldersToRefresh.add(c);
        }
      }

      if (foldersToRefresh.size === 0) return;

      try {
        // Fetch fresh children for each affected folder
        const folderUpdates = new Map<string, FileTreeNode[] | undefined>();
        const fetches = Array.from(foldersToRefresh).map(async (folder) => {
          try {
            const res = await requestFileTree(client, sessionId, folder || '', 1);
            folderUpdates.set(folder, res.root?.children);
          } catch {
            // Folder may have been removed
          }
        });
        await Promise.all(fetches);

        setTree((prev) => {
          if (!prev) return prev;

          // If root ('') was affected, merge root children
          if (folderUpdates.has('')) {
            const freshRootChildren = folderUpdates.get('');
            const existingByPath = new Map(
              (prev.children ?? []).map((c) => [c.path, c])
            );
            const mergedRootChildren = freshRootChildren?.map((incoming) => {
              const existing = existingByPath.get(incoming.path);
              if (existing && existing.is_dir && incoming.is_dir) {
                return mergeTreeNodes(existing, incoming);
              }
              return incoming;
            });
            prev = { ...prev, children: mergedRootChildren };
          }

          // Patch subfolders with fresh children
          const subFolders = Array.from(folderUpdates.keys()).filter((k) => k !== '');
          if (subFolders.length === 0) return prev;

          const patchNode = (node: FileTreeNode): FileTreeNode => {
            if (node.is_dir && folderUpdates.has(node.path)) {
              const freshChildren = folderUpdates.get(node.path);
              return { ...node, children: freshChildren?.map(patchNode) };
            }
            if (node.children) {
              return { ...node, children: node.children.map(patchNode) };
            }
            return node;
          };

          return patchNode(prev);
        });
        setLoadState('loaded');
      } catch (error) {
        console.error('[FileBrowser] Failed to refresh file tree:', error);
      }
    });

    return unsubscribe;
  }, [sessionId, expandedPaths]);

  // Handle scroll events to save position
  const handleScroll = useCallback((event: Event) => {
    const target = event.target as HTMLElement;
    const scrollTop = target.scrollTop;

    // Debounce save
    if (scrollSaveTimeoutRef.current) {
      clearTimeout(scrollSaveTimeoutRef.current);
    }
    scrollSaveTimeoutRef.current = setTimeout(() => {
      setFilesPanelScrollPosition(sessionId, scrollTop);
    }, 150);
  }, [sessionId]);

  // Attach scroll listener to ScrollArea viewport
  useEffect(() => {
    const scrollArea = scrollAreaRef.current;
    if (!scrollArea) return;

    const viewport = scrollArea.querySelector('[data-radix-scroll-area-viewport]');
    if (!viewport) return;

    viewport.addEventListener('scroll', handleScroll);
    return () => {
      viewport.removeEventListener('scroll', handleScroll);
      if (scrollSaveTimeoutRef.current) {
        clearTimeout(scrollSaveTimeoutRef.current);
      }
    };
  }, [handleScroll, tree]);

  // Handle search input change with debounce
  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setLocalSearchQuery(value);

    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }

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

  // Close search and clear state
  const handleCloseSearch = useCallback(() => {
    setIsSearchActive(false);
    setLocalSearchQuery('');
    setSearchResults(null);
    setIsSearching(false);
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }
  }, []);

  // Cleanup search timeout on unmount
  useEffect(() => {
    return () => {
      if (searchTimeoutRef.current) {
        clearTimeout(searchTimeoutRef.current);
      }
    };
  }, []);

  useEffect(() => {
    return () => {
      clearRetryTimer();
    };
  }, [clearRetryTimer]);

  const toggleExpand = useCallback(async (node: FileTreeNode) => {
    if (!node.is_dir) return;

    // Track the last-clicked folder as active
    setActiveFolderPath(node.path);

    const newExpanded = new Set(expandedPaths);

    if (newExpanded.has(node.path)) {
      newExpanded.delete(node.path);
      setExpandedPaths(newExpanded);
    } else {
      // If children not loaded yet, fetch them
      if (!node.children || node.children.length === 0) {
        showLoading(node.path);
        try {
          const client = getWebSocketClient();
          if (!client) return;

          const response = await requestFileTree(client, sessionId, node.path, 1);

          // Update tree with new children
          const updateNode = (n: FileTreeNode): FileTreeNode => {
            if (n.path === node.path) {
              return { ...n, children: response.root.children };
            }
            if (n.children) {
              return { ...n, children: n.children.map(updateNode) };
            }
            return n;
          };

          if (tree) {
            setTree(updateNode(tree));
          }
        } catch (error) {
          console.error('Failed to load children:', error);
        } finally {
          hideLoading(node.path);
        }
      }

      newExpanded.add(node.path);
      setExpandedPaths(newExpanded);
    }
  }, [expandedPaths, sessionId, tree, showLoading, hideLoading]);

  const openFileByPath = useCallback(async (path: string) => {
    try {
      const client = getWebSocketClient();
      if (!client) return;

      const response: FileContentResponse = await requestFileContent(client, sessionId, path);

      const { calculateHash } = await import('@/lib/utils/file-diff');
      const hash = await calculateHash(response.content);

      const name = path.split('/').pop() || path;

      onOpenFile({
        path,
        name,
        content: response.content,
        originalContent: response.content,
        originalHash: hash,
        isDirty: false,
        isBinary: response.is_binary,
      });
    } catch (error) {
      const reason = error instanceof Error ? error.message : 'Unknown error';
      toast({
        title: 'Failed to open file',
        description: reason,
        variant: 'error',
      });
    }
  }, [sessionId, onOpenFile, toast]);

  const renderTreeNode = (node: FileTreeNode, depth: number = 0): React.ReactNode => {
    const isExpanded = expandedPaths.has(node.path);
    const isLoading = visibleLoadingPaths.has(node.path);
    const isActive = !node.is_dir && activeFilePath === node.path;
    const isActiveFolder = node.is_dir && activeFolderPath === node.path;

    return (
      <div key={node.path}>
        <div
          className={cn(
            "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
            "hover:bg-muted",
            isActive && "bg-muted",
            isActiveFolder && "bg-muted/50"
          )}
          style={{ paddingLeft: `${depth * 12 + 8 + (node.is_dir ? 0 : 20)}px` }}
          onClick={() => (node.is_dir ? toggleExpand(node) : openFileByPath(node.path))}
        >
          {node.is_dir && (
            <span className="flex-shrink-0">
              {isLoading ? (
                <IconRefresh className="h-4 w-4 animate-spin text-muted-foreground shrink-0" />
              ) : isExpanded ? (
                <IconChevronDown className="h-3 w-3 text-muted-foreground/60" />
              ) : (
                <IconChevronRight className="h-3 w-3 text-muted-foreground/60" />
              )}
            </span>
          )}
          {node.is_dir ? (
            isExpanded ? (
              <IconFolderOpen className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
            ) : (
              <IconFolder className="h-3.5 w-3.5 flex-shrink-0 text-muted-foreground" />
            )
          ) : (
            <FileIcon
              fileName={node.name}
              filePath={node.path}
              className="flex-shrink-0"
              style={{
                width: '14px',
                height: '14px',
                opacity: isActive ? 1 : 0.7
              }}
            />
          )}
          <span className={`flex-1 truncate ${isActive ? 'text-foreground' : 'text-muted-foreground'} group-hover:text-foreground ${node.is_dir ? 'font-medium' : ''}`}>{node.name}</span>
          {!node.is_dir && onDeleteFile && (
            <div className="flex items-center gap-1 ml-auto opacity-0 group-hover:opacity-100">
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-destructive cursor-pointer"
                    onClick={(e) => {
                      e.stopPropagation();
                      // Optimistically remove, revert on failure
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
          )}
        </div>
        {node.is_dir && isExpanded && (
          <div>
            {creatingInPath === node.path && (
              <InlineFileInput
                depth={depth + 1}
                onSubmit={(name) => handleCreateFileSubmit(node.path, name)}
                onCancel={() => setCreatingInPath(null)}
              />
            )}
            {node.children && [...node.children]
              .sort(compareTreeNodes)
              .map((child) => renderTreeNode(child, depth + 1))}
          </div>
        )}
      </div>
    );
  };

  const renderSearchResults = () => {
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
          return (
            <div
              key={path}
              className={cn(
                "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
                "hover:bg-muted"
              )}
              onClick={() => openFileByPath(path)}
            >
              <FileIcon
                fileName={name}
                filePath={path}
                className="flex-shrink-0"
                style={{ width: '14px', height: '14px' }}
              />
              <span className="truncate text-muted-foreground group-hover:text-foreground">
                {folder && <span>{folder}/</span>}
                <span>{name}</span>
              </span>
            </div>
          );
        })}
      </div>
    );
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header toolbar */}
      {tree && loadState === 'loaded' && (
        isSearchActive ? (
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
              onChange={handleSearchChange}
              onKeyDown={(e) => { if (e.key === 'Escape') handleCloseSearch(); }}
              placeholder="Search files..."
              className="flex-1 min-w-0 h-5 text-xs border-none bg-transparent shadow-none focus-visible:ring-0 px-2"
            />
            <button
              className="text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
              onClick={handleCloseSearch}
            >
              <IconX className="h-3.5 w-3.5" />
            </button>
          </PanelHeaderBar>
        ) : (
          <PanelHeaderBarSplit
            className="group/header"
            left={
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      className="relative shrink-0 cursor-pointer"
                      onClick={() => {
                        if (fullPath) void copyPath(fullPath);
                      }}
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
                {onCreateFile && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                        onClick={handleStartCreate}
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
                      onClick={() => openFolder()}
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
                      onClick={() => setIsSearchActive(true)}
                    >
                      <IconSearch className="h-3.5 w-3.5" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent>Search files</TooltipContent>
                </Tooltip>
                {expandedPaths.size > 0 && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                        onClick={collapseAll}
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
        )
      )}

      <ScrollArea className="flex-1" ref={scrollAreaRef}>
        {isSearchActive && searchResults !== null ? (
          renderSearchResults()
        ) : isSessionFailed ? (
          <div className="p-4 text-sm text-destructive/80 space-y-2">
            <div>Session failed</div>
            {sessionError && (
              <div className="text-xs text-muted-foreground">{sessionError}</div>
            )}
          </div>
        ) : (loadState === 'loading' || isLoadingTree) && !tree ? (
          <div className="p-4 text-sm text-muted-foreground">Loading files...</div>
        ) : loadState === 'waiting' ? (
          <div className="p-4 text-sm text-muted-foreground flex items-center gap-2">
            <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
            Preparing workspace...
          </div>
        ) : loadState === 'manual' ? (
          <div className="p-4 text-sm text-muted-foreground space-y-2">
            <div>{loadError ?? 'Workspace is still starting.'}</div>
            <button
              type="button"
              className="text-xs text-foreground underline cursor-pointer"
              onClick={() => void loadTree({ resetRetry: true })}
            >
              Retry
            </button>
          </div>
        ) : tree ? (
          <div className="pb-2">
            {creatingInPath === '' && (
              <InlineFileInput
                depth={0}
                onSubmit={(name) => handleCreateFileSubmit('', name)}
                onCancel={() => setCreatingInPath(null)}
              />
            )}
            {tree.children && [...tree.children]
              .sort(compareTreeNodes)
              .map((child) => renderTreeNode(child, 0))}
          </div>
        ) : (
          <div className="p-4 text-sm text-muted-foreground">No files found</div>
        )}
      </ScrollArea>
    </div>
  );
}
