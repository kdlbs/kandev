import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileTree } from "@/lib/ws/workspace-files";
import type { FileTreeNode, FileTreeResponse } from "@/lib/types/backend";
import { isWorkspaceTreePath } from "@/lib/workspace-file-path";
import { findNodeByPath, mergeTreeNodes } from "./file-tree-utils";
import type { TreeLoadOwner } from "./file-browser-tree-loader";
export type RestoredTree = {
  root: FileTreeNode | null;
  tree: FileTreeNode | null;
  failedPaths: string[];
  errors?: unknown[];
};

export function restoredExpandedPaths(paths: string[]): string[] {
  const restored = new Set<string>();
  for (const path of paths) {
    if (!path || !isWorkspaceTreePath(path)) continue;
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

/** Keep visible retained branches; collapsed descendants must load again when opened. */
export function retainExpandedChildren(
  tree: FileTreeNode,
  expanded: ReadonlySet<string>,
): FileTreeNode {
  if (!tree.children) return tree;
  return {
    ...tree,
    children: tree.children.map((child) => {
      if (!child.is_dir || !child.children) return child;
      return expanded.has(child.path)
        ? retainExpandedChildren(child, expanded)
        : { ...child, children: undefined };
    }),
  };
}

export function mergeLoadedFolder(tree: FileTreeNode, incoming: FileTreeNode): FileTreeNode {
  if (tree.path === incoming.path) return mergeTreeNodes(tree, incoming);
  if (!tree.children) return tree;
  const children = tree.children.map((child) => mergeLoadedFolder(child, incoming));
  return children.every((child, index) => child === tree.children![index])
    ? tree
    : { ...tree, children };
}

type FolderResult = { path: string } & (
  | { ok: true; response: FileTreeResponse }
  | { ok: false; error: unknown }
);

function startReadyFolders({
  pending,
  running,
  blocked,
  tree,
  readPath,
  failedPaths,
}: {
  pending: Set<string>;
  running: Map<string, Promise<FolderResult>>;
  blocked: Set<string>;
  tree: FileTreeNode | null;
  readPath: (path: string) => Promise<FileTreeResponse>;
  failedPaths: string[];
}) {
  for (const path of pending) {
    if (running.size >= 4) break;
    const parent = path.slice(0, Math.max(0, path.lastIndexOf("/")));
    if (pending.has(parent) || running.has(parent)) continue;
    pending.delete(path);
    if ([...blocked].some((ancestor) => path.startsWith(`${ancestor}/`))) continue;
    if (!tree || !findNodeByPath(tree, path)?.is_dir) {
      failedPaths.push(path);
      blocked.add(path);
      continue;
    }
    running.set(
      path,
      readPath(path).then(
        (response): FolderResult => ({ ok: true, path, response }),
        (error: unknown): FolderResult => ({ ok: false, path, error }),
      ),
    );
  }
}

function applyFolderResult(
  result: FolderResult,
  tree: FileTreeNode | null,
  state: {
    errors: unknown[];
    blocked: Set<string>;
    failedPaths: string[];
    onTree?: (tree: FileTreeNode | null, root: boolean) => void;
  },
) {
  if (!result.ok) {
    state.errors.push(result.error);
    state.blocked.add(result.path);
  } else if (result.response.root && tree) {
    tree = mergeLoadedFolder(tree, result.response.root);
    state.onTree?.(result.response.root, false);
  } else {
    state.failedPaths.push(result.path);
    state.blocked.add(result.path);
  }
  return tree;
}

export async function fetchRestoredTree({
  client,
  owner,
  paths,
  isCurrentLoad,
  onTree,
  readPath = (path) => requestFileTree(client, owner.sessionId, path, 1),
}: {
  client: NonNullable<ReturnType<typeof getWebSocketClient>>;
  owner: TreeLoadOwner;
  paths: string[];
  isCurrentLoad: () => boolean;
  onTree?: (tree: FileTreeNode | null, root: boolean) => void;
  readPath?: (path: string) => Promise<FileTreeResponse>;
}): Promise<RestoredTree | null> {
  const rootResponse = await readPath("");
  if (!isCurrentLoad()) return null;
  let tree: FileTreeNode | null = rootResponse.root ?? null;
  const failedPaths: string[] = [];
  const errors: unknown[] = [];
  const blocked = new Set<string>();
  const pending = new Set(restoredExpandedPaths(paths));
  const running = new Map<string, Promise<FolderResult>>();
  // Empty materializing roots retain the loader's bounded readiness retries.
  if (!tree || tree.children?.length) onTree?.(tree, true);
  while (pending.size > 0 || running.size > 0) {
    if (!isCurrentLoad()) return null;
    startReadyFolders({ pending, running, blocked, tree, readPath, failedPaths });
    if (running.size === 0) break;
    const result = await Promise.race(running.values());
    running.delete(result.path);
    if (!isCurrentLoad()) return null;
    tree = applyFolderResult(result, tree, { errors, blocked, failedPaths, onTree });
  }
  return { root: rootResponse.root ?? null, tree, failedPaths, errors };
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
