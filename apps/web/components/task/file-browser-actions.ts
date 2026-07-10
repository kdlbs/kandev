"use client";

import { useEffect, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileTree, requestFileContent } from "@/lib/ws/workspace-files";
import type { FileTreeNode, FileContentResponse, OpenFileTab } from "@/lib/types/backend";
import { getFilesPanelScrollPosition, setFilesPanelScrollPosition } from "@/lib/local-storage";
import type { useToast } from "@/components/toast-provider";
import type { useFileBrowserTree } from "./file-browser-hooks";

export type FetchAndOpenFileOptions = {
  repo?: string;
  signal?: AbortSignal;
};

/** Hook for scroll position persistence in the file browser. */
export function useScrollPersistence(
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
      const viewport = scrollAreaRef.current.querySelector("[data-radix-scroll-area-viewport]");
      if (viewport) {
        viewport.scrollTop = savedScroll;
        hasRestoredScrollRef.current = sessionId;
      }
    } else {
      hasRestoredScrollRef.current = sessionId;
    }
  }, [isTreeLoaded, sessionId, scrollAreaRef]);

  // Attach scroll listener to ScrollArea viewport
  useEffect(() => {
    const el = scrollAreaRef.current;
    if (!el) return;
    const viewport = el.querySelector("[data-radix-scroll-area-viewport]");
    if (!viewport) return;
    const onScroll = (event: Event) => {
      const target = event.target as HTMLElement;
      if (scrollSaveTimeoutRef.current) clearTimeout(scrollSaveTimeoutRef.current);
      scrollSaveTimeoutRef.current = setTimeout(() => {
        setFilesPanelScrollPosition(sessionId, target.scrollTop);
      }, 150);
    };
    viewport.addEventListener("scroll", onScroll);
    return () => {
      viewport.removeEventListener("scroll", onScroll);
      if (scrollSaveTimeoutRef.current) clearTimeout(scrollSaveTimeoutRef.current);
    };
  }, [sessionId, tree, scrollAreaRef]);
}

/** Fetch children for a folder node if not already loaded. */
export async function loadNodeChildren(
  node: FileTreeNode,
  sessionId: string,
  treeState: ReturnType<typeof useFileBrowserTree>,
) {
  if (node.children && node.children.length > 0) return;
  // Dedupe in-flight fetches so rapid double-clicks don't issue two WS round-trips.
  if (treeState.isLoading(node.path)) return;
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
    console.error("Failed to load children:", error);
  } finally {
    treeState.hideLoading(node.path);
  }
}

export type ToggleFolderExpandDeps = {
  node: FileTreeNode;
  sessionId: string;
  treeState: ReturnType<typeof useFileBrowserTree>;
  setActiveFolderPath: (path: string) => void;
  loadChildren?: typeof loadNodeChildren;
};

// Flip expanded synchronously *then* fetch children so the chevron rotates on the first click.
export async function toggleFolderExpand({
  node,
  sessionId,
  treeState,
  setActiveFolderPath,
  loadChildren = loadNodeChildren,
}: ToggleFolderExpandDeps): Promise<void> {
  if (!node.is_dir) return;
  setActiveFolderPath(node.path);
  const wasExpanded = treeState.expandedPaths.has(node.path);
  treeState.setExpandedPaths((prev) => {
    const next = new Set(prev);
    if (next.has(node.path)) next.delete(node.path);
    else next.add(node.path);
    return next;
  });
  if (!wasExpanded) {
    await loadChildren(node, sessionId, treeState);
  }
}

/** Fetch and open a file by path. */
export async function fetchAndOpenFile(
  sessionId: string,
  path: string,
  onOpenFile: (file: OpenFileTab) => void,
  toast: ReturnType<typeof useToast>["toast"],
  options: FetchAndOpenFileOptions = {},
) {
  const { repo, signal } = options;
  try {
    if (signal?.aborted) return;
    const client = getWebSocketClient();
    if (!client) return;
    const response: FileContentResponse = await requestFileContent(client, sessionId, path, repo);
    if (signal?.aborted) return;
    const { calculateHash } = await import("@/lib/utils/file-diff");
    const hash = await calculateHash(response.content);
    if (signal?.aborted) return;
    const name = path.split("/").pop() || path;
    onOpenFile({
      path,
      name,
      repo,
      content: response.content,
      originalContent: response.content,
      originalHash: hash,
      isDirty: false,
      isBinary: response.is_binary,
    });
  } catch (error) {
    if (signal?.aborted) return;
    const reason = error instanceof Error ? error.message : "Unknown error";
    toast({ title: "Failed to open file", description: reason, variant: "error" });
  }
}
