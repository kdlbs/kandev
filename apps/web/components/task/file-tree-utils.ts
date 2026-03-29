import type { FileTreeNode } from "@/lib/types/backend";

/** Sort comparator: directories first, then alphabetical by name. */
export const compareTreeNodes = (a: FileTreeNode, b: FileTreeNode): number => {
  if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
  return a.name.localeCompare(b.name);
};

/**
 * Merge a freshly-fetched tree node into an existing one, preserving
 * already-loaded children so expanded folders don't collapse.
 */
export function mergeTreeNodes(existing: FileTreeNode, incoming: FileTreeNode): FileTreeNode {
  if (!incoming.children) return { ...existing, ...incoming, children: existing.children };
  if (!existing.children) return incoming;
  const existingByPath = new Map(existing.children.map((c) => [c.path, c]));
  const mergedChildren = incoming.children.map((inChild) => {
    const exChild = existingByPath.get(inChild.path);
    if (exChild && exChild.is_dir && inChild.is_dir) {
      return mergeTreeNodes(exChild, inChild);
    }
    return inChild;
  });
  return { ...existing, ...incoming, children: mergedChildren };
}

/** Insert a file node into a parent folder, keeping children sorted (dirs first, then alpha). */
export function insertNodeInTree(
  root: FileTreeNode,
  parentPath: string,
  node: FileTreeNode,
): FileTreeNode {
  if (root.path === parentPath || (parentPath === "" && root.path === "")) {
    const children = [...(root.children ?? []), node].sort(compareTreeNodes);
    return { ...root, children };
  }
  if (!root.children) return root;
  return { ...root, children: root.children.map((c) => insertNodeInTree(c, parentPath, node)) };
}

export function removeNodeFromTree(root: FileTreeNode, targetPath: string): FileTreeNode {
  if (!root.children) return root;
  const filtered = root.children.filter((c) => c.path !== targetPath);
  return { ...root, children: filtered.map((c) => removeNodeFromTree(c, targetPath)) };
}

/** Rename a node in the tree, updating its name and path. */
function replacePathPrefix(path: string, oldPrefix: string, newPrefix: string): string {
  if (path === oldPrefix) return newPrefix;
  if (path.startsWith(`${oldPrefix}/`)) return `${newPrefix}${path.slice(oldPrefix.length)}`;
  return path;
}

function renameSubtree(node: FileTreeNode, oldPath: string, newPath: string): FileTreeNode {
  const nextPath = replacePathPrefix(node.path, oldPath, newPath);
  const nextName = nextPath.split("/").pop() || nextPath;
  const nextChildren = node.children?.map((child) => renameSubtree(child, oldPath, newPath));
  return {
    ...node,
    name: nextName,
    path: nextPath,
    children: nextChildren,
  };
}

export function treeContainsPath(root: FileTreeNode, targetPath: string): boolean {
  if (root.path === targetPath) return true;
  if (!root.children) return false;
  return root.children.some((child) => treeContainsPath(child, targetPath));
}

export function countFilesInTree(node: FileTreeNode): number {
  if (!node.children || node.children.length === 0) return node.is_dir ? 0 : 1;
  const base = node.is_dir ? 0 : 1;
  return node.children.reduce((sum, child) => sum + countFilesInTree(child), base);
}

export function renameNodeInTree(
  root: FileTreeNode,
  oldPath: string,
  newPath: string,
): FileTreeNode {
  if (root.path === oldPath) {
    return renameSubtree(root, oldPath, newPath);
  }
  if (!root.children) return root;
  const nextChildren = root.children.map((c) => renameNodeInTree(c, oldPath, newPath));
  return { ...root, children: nextChildren.sort(compareTreeNodes) };
}

/** Collect visible (expanded) node paths in DFS order for multi-select range computation. */
export function getVisiblePaths(tree: FileTreeNode, expandedPaths: Set<string>): string[] {
  const result: string[] = [];
  function walk(node: FileTreeNode) {
    // Skip the root node itself (it represents the workspace root)
    if (node !== tree) result.push(node.path);
    if (node.is_dir && (node === tree || expandedPaths.has(node.path)) && node.children) {
      const sorted = [...node.children].sort(compareTreeNodes);
      for (const child of sorted) walk(child);
    }
  }
  walk(tree);
  return result;
}

/** Find a node in the tree by path. */
export function findNodeByPath(root: FileTreeNode, targetPath: string): FileTreeNode | null {
  if (root.path === targetPath) return root;
  if (!root.children) return null;
  for (const child of root.children) {
    const found = findNodeByPath(child, targetPath);
    if (found) return found;
  }
  return null;
}

/** Move nodes from their current locations into a target directory. Returns the updated tree. */
export function moveNodesInTree(
  root: FileTreeNode,
  sourcePaths: string[],
  targetDirPath: string,
): FileTreeNode {
  // Collect the nodes to move before removing them
  const nodesToMove: FileTreeNode[] = [];
  for (const path of sourcePaths) {
    const node = findNodeByPath(root, path);
    if (node) {
      const name = node.name;
      const newPath = targetDirPath ? `${targetDirPath}/${name}` : name;
      nodesToMove.push(renameSubtree(node, path, newPath));
    }
  }

  // Remove source nodes from tree
  let updated = root;
  for (const path of sourcePaths) {
    updated = removeNodeFromTree(updated, path);
  }

  // Insert into target directory
  for (const node of nodesToMove) {
    updated = insertNodeInTree(updated, targetDirPath, node);
  }

  return updated;
}
