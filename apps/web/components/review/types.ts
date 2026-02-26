export type ReviewFile = {
  path: string;
  diff: string;
  status: string;
  additions: number;
  deletions: number;
  staged: boolean;
  source: "uncommitted" | "committed" | "pr";
};

export type FileTreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children?: FileTreeNode[];
  file?: ReviewFile;
};

/**
 * Normalize diff content by handling edge cases from the backend.
 * Handles concatenated diffs with "--- Staged changes ---" separator.
 */
export function normalizeDiffContent(diffContent: string): string {
  if (!diffContent || typeof diffContent !== "string") return "";
  let trimmed = diffContent.trim();
  if (!trimmed) return "";

  const stagedSeparator = "--- Staged changes ---";
  if (trimmed.includes(stagedSeparator)) {
    const parts = trimmed.split(stagedSeparator);
    trimmed = (parts[1] || parts[0]).trim();
  }

  return trimmed;
}

/**
 * Simple djb2 hash of a string — fast, non-cryptographic.
 * Used to detect if a diff has changed since the user last reviewed it.
 */
export function hashDiff(content: string): string {
  let hash = 5381;
  for (let i = 0; i < content.length; i++) {
    hash = ((hash << 5) + hash + content.charCodeAt(i)) | 0;
  }
  return hash.toString(36);
}

/**
 * Build a hierarchical file tree from flat file paths.
 * Collapses single-child directories (e.g., `src/components` as one node if `src` has no other children).
 */
export function buildFileTree(files: ReviewFile[]): FileTreeNode[] {
  const root: FileTreeNode = { name: "", path: "", isDir: true, children: [] };

  for (const file of files) {
    const parts = file.path.split("/");
    let current = root;

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const isLast = i === parts.length - 1;
      const partPath = parts.slice(0, i + 1).join("/");

      if (isLast) {
        current.children!.push({
          name: part,
          path: file.path,
          isDir: false,
          file,
        });
      } else {
        let child = current.children!.find((c) => c.isDir && c.name === part);
        if (!child) {
          child = { name: part, path: partPath, isDir: true, children: [] };
          current.children!.push(child);
        }
        current = child;
      }
    }
  }

  // Collapse single-child directories
  function collapse(node: FileTreeNode): FileTreeNode {
    if (!node.isDir || !node.children) return node;

    node.children = node.children.map(collapse);

    if (node.children.length === 1 && node.children[0].isDir && node.name !== "") {
      const child = node.children[0];
      return {
        ...child,
        name: `${node.name}/${child.name}`,
      };
    }

    return node;
  }

  const collapsed = collapse(root);

  // Sort: directories first, then files, alphabetically
  function sortTree(nodes: FileTreeNode[]): FileTreeNode[] {
    return nodes
      .sort((a, b) => {
        if (a.isDir && !b.isDir) return -1;
        if (!a.isDir && b.isDir) return 1;
        return a.name.localeCompare(b.name);
      })
      .map((node) => {
        if (node.isDir && node.children) {
          return { ...node, children: sortTree(node.children) };
        }
        return node;
      });
  }

  return sortTree(collapsed.children ?? []);
}
