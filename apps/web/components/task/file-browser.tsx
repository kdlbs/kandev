'use client';

import React, { useEffect, useState, useCallback, useRef } from 'react';
import { IconChevronRight, IconChevronDown, IconFile, IconFolder, IconFolderOpen, IconSearch, IconX, IconLoader2, IconTrash } from '@tabler/icons-react';
import { ScrollArea } from '@kandev/ui/scroll-area';
import { Input } from '@kandev/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileTree, requestFileContent, searchWorkspaceFiles } from '@/lib/ws/workspace-files';
import type { FileTreeNode, FileContentResponse, OpenFileTab } from '@/lib/types/backend';
import {
  getFilesPanelExpandedPaths,
  setFilesPanelExpandedPaths,
  getFilesPanelScrollPosition,
  setFilesPanelScrollPosition,
} from '@/lib/local-storage';

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
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  const [isLoadingTree, setIsLoadingTree] = useState(true);

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

  // Load initial tree - always try to load, don't wait for agentctl ready status
  useEffect(() => {
    setTree(null);
    setLoadingPaths(new Set());
    setIsLoadingTree(true);
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

    const loadTree = async () => {
      try {
        const client = getWebSocketClient();
        if (!client) return;

        const response = await requestFileTree(client, sessionId, '', 1);
        setTree(response.root);
      } catch (error) {
        console.error('Failed to load file tree:', error);
      } finally {
        setIsLoadingTree(false);
      }
    };

    loadTree();
  }, [sessionId]);

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

    // Only expand the root folder to show its immediate children
    setExpandedPaths(new Set([tree.path]));
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
      // Refresh the root tree when any file changes
      try {
        const response = await requestFileTree(client, sessionId, '', 1);
        setTree(response.root);
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

  const toggleExpand = useCallback(async (node: FileTreeNode) => {
    if (!node.is_dir) return;

    const newExpanded = new Set(expandedPaths);

    if (newExpanded.has(node.path)) {
      newExpanded.delete(node.path);
      setExpandedPaths(newExpanded);
    } else {
      // If children not loaded yet, fetch them
      if (!node.children || node.children.length === 0) {
        setLoadingPaths((prev) => new Set(prev).add(node.path));
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
          setLoadingPaths((prev) => {
            const next = new Set(prev);
            next.delete(node.path);
            return next;
          });
        }
      }

      newExpanded.add(node.path);
      setExpandedPaths(newExpanded);
    }
  }, [expandedPaths, sessionId, tree]);

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
      });
    } catch (error) {
      console.error('Failed to load file content:', error);
    }
  }, [sessionId, onOpenFile]);

  const renderTreeNode = (node: FileTreeNode, depth: number = 0): React.ReactNode => {
    const isExpanded = expandedPaths.has(node.path);
    const isLoading = loadingPaths.has(node.path);
    const isActive = !node.is_dir && activeFilePath === node.path;

    return (
      <div key={node.path}>
        <div
          className={`group flex items-center gap-1 py-1 px-2 hover:bg-muted/50 cursor-pointer rounded text-sm ${isActive ? 'bg-muted/50' : ''}`}
          style={{ paddingLeft: `${depth * 12 + 8 + (node.is_dir ? 0 : 20)}px` }}
          onClick={() => (node.is_dir ? toggleExpand(node) : openFile(node))}
        >
          {node.is_dir && (
            <span className="flex-shrink-0">
              {isLoading ? (
                <IconLoader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              ) : isExpanded ? (
                <IconChevronDown className="h-4 w-4 text-muted-foreground" />
              ) : (
                <IconChevronRight className="h-4 w-4 text-muted-foreground" />
              )}
            </span>
          )}
          {node.is_dir ? (
            isExpanded ? (
              <IconFolderOpen className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
            ) : (
              <IconFolder className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
            )
          ) : (
            <IconFile className={`h-3 w-3 flex-shrink-0 ${isActive ? 'text-foreground' : 'text-muted-foreground'}`} />
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
              className="group flex items-center gap-1 py-1 px-2 hover:bg-muted/50 cursor-pointer rounded text-sm"
              onClick={() => openFileByPath(path)}
            >
              <IconFile className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
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

      <ScrollArea className="flex-1" ref={scrollAreaRef}>
        {isSearchOpen && searchResults !== null ? (
          renderSearchResults()
        ) : isLoadingTree ? (
          <div className="p-4 text-sm text-muted-foreground">Loading files...</div>
        ) : tree ? (
          <div className="pb-2">{renderTreeNode(tree)}</div>
        ) : (
          <div className="p-4 text-sm text-muted-foreground">No files found</div>
        )}
      </ScrollArea>
    </div>
  );
}
