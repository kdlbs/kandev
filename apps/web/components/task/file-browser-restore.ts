import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileTree } from "@/lib/ws/workspace-files";
import type { FileTreeNode } from "@/lib/types/backend";
import { findNodeByPath, mergeTreeNodes } from "./file-tree-utils";
import type { TreeLoadOwner } from "./file-browser-tree-loader";
export type RestoredTree = {
  root: FileTreeNode | null;
  tree: FileTreeNode | null;
  failedPaths: string[];
};

export function restoredExpandedPaths(paths: string[]): string[] {
  const restored = new Set<string>();
  for (const path of paths) {
    if (!path) continue;
    const parts = path.split("/");
    for (let i = 1; i <= parts.length; i++) restored.add(parts.slice(0, i).join("/"));
  }
  return Array.from(restored).sort((a, b) => {
    const depth = a.split("/").length - b.split("/").length;
    return depth || a.localeCompare(b);
  });
}

export function removeFailedExpansions(paths: string[], failedPaths: string[]): Set<string> {
  return new Set(
    paths.filter(
      (expanded) =>
        !failedPaths.some((failed) => expanded === failed || expanded.startsWith(`${failed}/`)),
    ),
  );
}

function mergeLoadedFolder(tree: FileTreeNode, incoming: FileTreeNode): FileTreeNode {
  if (tree.path === incoming.path) return mergeTreeNodes(tree, incoming);
  if (!tree.children) return tree;
  return { ...tree, children: tree.children.map((child) => mergeLoadedFolder(child, incoming)) };
}

export async function fetchRestoredTree({
  client,
  owner,
  paths,
  isCurrentLoad,
  onFolderFailure,
}: {
  client: NonNullable<ReturnType<typeof getWebSocketClient>>;
  owner: TreeLoadOwner;
  paths: string[];
  isCurrentLoad: () => boolean;
  onFolderFailure: (path: string, error: unknown) => void;
}): Promise<RestoredTree | null> {
  const rootResponse = await requestFileTree(client, owner.sessionId, "", 1);
  if (!isCurrentLoad()) return null;
  let tree = rootResponse.root ?? null;
  const failedPaths: string[] = [];
  for (const path of paths) {
    if (failedPaths.some((failed) => path === failed || path.startsWith(`${failed}/`))) continue;
    if (!tree || !findNodeByPath(tree, path)?.is_dir) {
      failedPaths.push(path);
      continue;
    }
    try {
      const response = await requestFileTree(client, owner.sessionId, path, 1);
      if (!isCurrentLoad()) return null;
      if (response.root) tree = mergeLoadedFolder(tree, response.root);
      else failedPaths.push(path);
    } catch (error) {
      failedPaths.push(path);
      onFolderFailure(path, error);
    }
  }
  return { root: rootResponse.root ?? null, tree, failedPaths };
}

export function completeRestoredTree(
  restored: RestoredTree | null,
  isCurrentLoad: () => boolean,
  onFailedPaths: (paths: string[]) => void,
): RestoredTree | null {
  if (!restored || !isCurrentLoad()) return null;
  if (restored.failedPaths.length > 0) onFailedPaths(restored.failedPaths);
  return restored;
}
