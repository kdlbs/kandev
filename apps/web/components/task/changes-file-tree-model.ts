import type { ChangedFile } from "./changes-panel-helpers";

export type ChangesTreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children?: ChangesTreeNode[];
  file?: ChangedFile;
};

/** Build a sorted directory tree from the flat Changes file collection. */
export function buildChangesTree(files: ChangedFile[]): ChangesTreeNode[] {
  const root: ChangesTreeNode = { name: "", path: "", isDir: true, children: [] };
  for (const file of files) {
    if (!file.path) continue;
    const parts = file.path.split("/");
    let current = root;
    for (let index = 0; index < parts.length; index++) {
      const part = parts[index];
      const isLast = index === parts.length - 1;
      const partPath = parts.slice(0, index + 1).join("/");
      if (isLast) {
        current.children!.push({ name: part, path: file.path, isDir: false, file });
      } else {
        let child = current.children!.find((node) => node.isDir && node.name === part);
        if (!child) {
          child = { name: part, path: partPath, isDir: true, children: [] };
          current.children!.push(child);
        }
        current = child;
      }
    }
  }
  return sortChangesTreeNodes(root.children ?? []);
}

function sortChangesTreeNodes(nodes: ChangesTreeNode[]): ChangesTreeNode[] {
  return nodes
    .sort((left, right) => {
      if (left.isDir !== right.isDir) return left.isDir ? -1 : 1;
      return left.name.localeCompare(right.name);
    })
    .map((node) =>
      node.isDir && node.children
        ? { ...node, children: sortChangesTreeNodes(node.children) }
        : node,
    );
}
