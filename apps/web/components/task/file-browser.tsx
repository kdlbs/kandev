'use client';

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { IconChevronRight, IconChevronDown, IconFolder, IconFolderOpen, IconSearch, IconX, IconLoader2, IconTrash, IconRefresh, IconListTree, IconFolderShare, IconCopy, IconCheck } from '@tabler/icons-react';
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

type FileBrowserProps = {
  sessionId: string;
  onOpenFile: (file: OpenFileTab) => void;
  onDeleteFile?: (path: string) => void;
  isSearchOpen?: boolean;
  onCloseSearch?: () => void;
  activeFilePath?: string | null;
};

export function FileBrowser({ sessionId, onOpenFile, onDeleteFile, isSearchOpen = false, onCloseSearch, activeFilePath }: FileBrowserProps) {
  const [tree, setTree] = useState<FileTreeNode | null>(null);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
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

  const MAX_RETRY_ATTEMPTS = 4;
  const RETRY_DELAYS_MS = [1000, 2000, 5000, 10000];

  // Workspace path for header
  const fullPath = session?.worktree_path ?? '';
  const displayPath = fullPath.replace(/^\/(?:Users|home)\/[^/]+\//, '~/');

  const collapseAll = useCallback(() => {
    setExpandedPaths(new Set());
  }, []);

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
    if (isSearchOpen && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [isSearchOpen]);

  // Clear search when closing
  useEffect(() => {
    if (!isSearchOpen) {
      setLocalSearchQuery('');
      setSearchResults(null);
      setIsSearching(false);
      if (searchTimeoutRef.current) {
        clearTimeout(searchTimeoutRef.current);
      }
    }
  }, [isSearchOpen]);

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

  // Subscribe to file changes and refresh tree
  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;

    const unsubscribe = client.on('session.workspace.file.changes', async () => {
      // Refresh the root tree when any file changes.
      // Merge new top-level children into the existing tree so that
      // already-loaded subtrees (expanded folders) are preserved.
      try {
        const response = await requestFileTree(client, sessionId, '', 1);
        const newRoot = response.root ?? null;
        setTree((prev) => {
          if (!prev || !newRoot) return newRoot;
          return mergeTreeNodes(prev, newRoot);
        });
        setLoadState('loaded');
      } catch (error) {
        console.error('Failed to refresh file tree:', error);
      }
    });

    return unsubscribe;
  }, [sessionId]);

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

  // Close search (clears and notifies parent)
  const handleCloseSearch = useCallback(() => {
    setLocalSearchQuery('');
    setSearchResults(null);
    setIsSearching(false);
    if (searchTimeoutRef.current) {
      clearTimeout(searchTimeoutRef.current);
    }
    onCloseSearch?.();
  }, [onCloseSearch]);

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

  const openFile = useCallback(async (node: FileTreeNode) => {
    if (node.is_dir) return;

    try {
      const client = getWebSocketClient();
      if (!client) return;

      const response: FileContentResponse = await requestFileContent(client, sessionId, node.path);

      // Calculate hash for the file content
      const { calculateHash } = await import('@/lib/utils/file-diff');
      const hash = await calculateHash(response.content);

      onOpenFile({
        path: node.path,
        name: node.name,
        content: response.content,
        originalContent: response.content,
        originalHash: hash,
        isDirty: false,
        isBinary: response.is_binary,
      });
    } catch (error) {
      console.error('Failed to load file content:', error);
    }
  }, [sessionId, onOpenFile]);

  // Open file from search result (path only)
  const openFileByPath = useCallback(async (path: string) => {
    try {
      const client = getWebSocketClient();
      if (!client) return;

      const response: FileContentResponse = await requestFileContent(client, sessionId, path);

      // Calculate hash for the file content
      const { calculateHash } = await import('@/lib/utils/file-diff');
      const hash = await calculateHash(response.content);

      // Extract filename from path
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
      console.error('Failed to load file content:', error);
    }
  }, [sessionId, onOpenFile]);

  const renderTreeNode = (node: FileTreeNode, depth: number = 0): React.ReactNode => {
    const isExpanded = expandedPaths.has(node.path);
    const isLoading = visibleLoadingPaths.has(node.path);
    const isActive = !node.is_dir && activeFilePath === node.path;

    return (
      <div key={node.path}>
        <div
          className={cn(
            "group flex w-full items-center gap-1 px-2 py-0.5 text-left text-sm cursor-pointer",
            "hover:bg-muted",
            isActive && "bg-muted"
          )}
          style={{ paddingLeft: `${depth * 12 + 8 + (node.is_dir ? 0 : 20)}px` }}
          onClick={() => (node.is_dir ? toggleExpand(node) : openFile(node))}
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
                      onDeleteFile(node.path);
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
        {node.is_dir && isExpanded && node.children && (
          <div>
            {[...node.children]
              .sort((a, b) => {
                // Folders first, then alphabetically
                if (a.is_dir && !b.is_dir) return -1;
                if (!a.is_dir && b.is_dir) return 1;
                return a.name.localeCompare(b.name);
              })
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
      {/* Search input - only shown when search is open */}
      {isSearchOpen && (
        <div className="px-2 pb-2">
          <div className="relative">
            {isSearching ? (
              <IconLoader2 className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground pointer-events-none animate-spin" />
            ) : (
              <IconSearch className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground pointer-events-none" />
            )}
            <Input
              ref={searchInputRef}
              type="text"
              value={localSearchQuery}
              onChange={handleSearchChange}
              placeholder="Search files..."
              className="pl-7 pr-7 h-7 text-xs"
            />
            <button
              type="button"
              onClick={handleCloseSearch}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <IconX className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      )}

      {/* Header toolbar */}
      {tree && loadState === 'loaded' && (
        <div className="group/header flex items-center gap-1 border-b border-border/50 px-2 py-1">
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
          <span className="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">
            {displayPath}
          </span>
          <div className="flex shrink-0 items-center gap-0.5">
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
          </div>
        </div>
      )}

      <ScrollArea className="flex-1" ref={scrollAreaRef}>
        {isSearchOpen && searchResults !== null ? (
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
            {tree.children && [...tree.children]
              .sort((a, b) => {
                if (a.is_dir && !b.is_dir) return -1;
                if (!a.is_dir && b.is_dir) return 1;
                return a.name.localeCompare(b.name);
              })
              .map((child) => renderTreeNode(child, 0))}
          </div>
        ) : (
          <div className="p-4 text-sm text-muted-foreground">No files found</div>
        )}
      </ScrollArea>
    </div>
  );
}
