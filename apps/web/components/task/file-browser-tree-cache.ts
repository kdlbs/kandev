import type { FileTreeNode } from "@/lib/types/backend";

export class FileBrowserTreeCache {
  private entries = new Map<string, { tree: FileTreeNode; count: number }>();
  private nodes = 0;

  get(key: string): FileTreeNode | null {
    const entry = this.entries.get(key);
    if (!entry) return null;
    this.entries.delete(key);
    this.entries.set(key, entry);
    return entry.tree;
  }

  set(key: string, tree: FileTreeNode | null) {
    this.remove(key);
    if (!tree) return;
    const count = countNodes(tree);
    if (count > 20000) return;
    this.entries.set(key, { tree, count });
    this.nodes += count;
    while (this.entries.size > 4 || this.nodes > 20000) {
      this.remove(this.entries.keys().next().value!);
    }
  }

  private remove(key: string) {
    this.nodes -= this.entries.get(key)?.count ?? 0;
    this.entries.delete(key);
  }
}

function countNodes(root: FileTreeNode) {
  const pending = [root];
  let count = 0;
  while (pending.length && count <= 20000) {
    const node = pending.pop()!;
    count++;
    if (node.children) for (const child of node.children) pending.push(child);
  }
  return count;
}

export type FileTreeCacheBinding = {
  cache: FileBrowserTreeCache;
  key: string;
  isCurrent: () => boolean;
};
