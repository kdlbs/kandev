'use client';

import React, { useEffect, useState, useCallback } from 'react';
import { IconChevronRight, IconChevronDown, IconFile, IconFolder, IconFolderOpen } from '@tabler/icons-react';
import { ScrollArea } from '@kandev/ui/scroll-area';
import { getWebSocketClient } from '@/lib/ws/connection';
import { requestFileTree, requestFileContent } from '@/lib/ws/workspace-files';
import type { FileTreeNode, FileContentResponse, OpenFileTab } from '@/lib/types/backend';

type FileBrowserProps = {
  sessionId: string;
  onOpenFile: (file: OpenFileTab) => void;
};

export function FileBrowser({ sessionId, onOpenFile }: FileBrowserProps) {
  const [tree, setTree] = useState<FileTreeNode | null>(null);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  const [isLoadingTree, setIsLoadingTree] = useState(true);

  // Load initial tree - always try to load, don't wait for agentctl ready status
  useEffect(() => {
    setTree(null);
    setExpandedPaths(new Set());
    setLoadingPaths(new Set());
    setIsLoadingTree(true);

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

      onOpenFile({
        path: node.path,
        name: node.name,
        content: response.content,
      });
    } catch (error) {
      console.error('Failed to load file content:', error);
    }
  }, [sessionId, onOpenFile]);

  const renderTreeNode = (node: FileTreeNode, depth: number = 0): React.ReactNode => {
    const isExpanded = expandedPaths.has(node.path);
    const isLoading = loadingPaths.has(node.path);

    return (
      <div key={node.path}>
        <div
          className="flex items-center gap-1 py-1 px-2 hover:bg-accent cursor-pointer rounded text-sm"
          style={{ paddingLeft: `${depth * 12 + 8}px` }}
          onClick={() => (node.is_dir ? toggleExpand(node) : openFile(node))}
        >
          {node.is_dir && (
            <span className="flex-shrink-0">
              {isLoading ? (
                <span className="animate-spin">⏳</span>
              ) : isExpanded ? (
                <IconChevronDown className="h-4 w-4" />
              ) : (
                <IconChevronRight className="h-4 w-4" />
              )}
            </span>
          )}
          {node.is_dir ? (
            isExpanded ? (
              <IconFolderOpen className="h-3 w-3 flex-shrink-0 text-blue-500" />
            ) : (
              <IconFolder className="h-3 w-3 flex-shrink-0 text-blue-500" />
            )
          ) : (
            <IconFile className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
          )}
          <span className="truncate">{node.name}</span>
          {!node.is_dir && node.size && (
            <span className="text-xs text-muted-foreground ml-auto">
              {(node.size / 1024).toFixed(1)}KB
            </span>
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

  return (
    <div className="flex flex-col h-full">
      <ScrollArea className="flex-1">
        {isLoadingTree ? (
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
