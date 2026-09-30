import type { VisibleRow } from "@/hooks/use-tree";
import type { ChangedFile } from "./changes-panel-helpers";

export type ChangesTreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children?: ChangesTreeNode[];
  file?: ChangedFile;
};

/** Build a sorted directory tree without repeatedly scanning sibling rows. */
export function buildChangesTree(files: ChangedFile[]): ChangesTreeNode[] {
  const root: ChangesTreeNode = { name: "", path: "", isDir: true, children: [] };
  const directoryChildren = new WeakMap<ChangesTreeNode, Map<string, ChangesTreeNode>>();

  for (const file of files) {
    if (!file.path) continue;
    const parts = file.path.split("/");
    let current = root;
    let currentPath = "";

    for (let index = 0; index < parts.length; index++) {
      const part = parts[index];
      const isLast = index === parts.length - 1;
      currentPath = currentPath ? `${currentPath}/${part}` : part;

      if (isLast) {
        current.children!.push({ name: part, path: file.path, isDir: false, file });
        continue;
      }

      let childMap = directoryChildren.get(current);
      if (!childMap) {
        childMap = new Map();
        directoryChildren.set(current, childMap);
      }
      let child = childMap.get(part);
      if (!child) {
        child = { name: part, path: currentPath, isDir: true, children: [] };
        childMap.set(part, child);
        current.children!.push(child);
      }
      current = child;
    }
  }

  return sortChangesTreeNodes(root.children ?? []);
}

function sortChangesTreeNodes(nodes: ChangesTreeNode[]): ChangesTreeNode[] {
  const stack = [nodes];
  while (stack.length > 0) {
    const siblings = stack.pop()!;
    siblings.sort((left, right) => {
      if (left.isDir !== right.isDir) return left.isDir ? -1 : 1;
      return left.name.localeCompare(right.name);
    });
    for (const child of siblings) {
      if (child.isDir && child.children) stack.push(child.children);
    }
  }
  return nodes;
}

/** Flatten only expanded directory children in the order shown by the tree. */
export function flattenChangesTree(
  nodes: ChangesTreeNode[],
  isCollapsed: (path: string) => boolean,
  baseDepth = 0,
): VisibleRow<ChangesTreeNode>[] {
  const rows: VisibleRow<ChangesTreeNode>[] = [];
  const stack: Array<{ node: ChangesTreeNode; depth: number }> = [];
  for (let index = nodes.length - 1; index >= 0; index--) {
    stack.push({ node: nodes[index], depth: baseDepth });
  }

  while (stack.length > 0) {
    const { node: chainRoot, depth } = stack.pop()!;
    let effective = chainRoot;
    let displayName = chainRoot.name;

    while (effective.isDir && effective.children?.length === 1 && effective.children[0].isDir) {
      effective = effective.children[0];
      displayName = `${displayName}/${effective.name}`;
    }

    const isExpanded = effective.isDir && !isCollapsed(effective.path);
    rows.push({
      node: effective,
      chainRoot,
      displayName,
      path: effective.path,
      depth,
      isExpanded,
      isDir: effective.isDir,
    });

    if (!isExpanded || !effective.children) continue;
    for (let index = effective.children.length - 1; index >= 0; index--) {
      stack.push({ node: effective.children[index], depth: depth + 1 });
    }
  }

  return rows;
}
