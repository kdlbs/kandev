import { djb2Hash } from "@/lib/utils/hash";

export type ReviewFile = {
  path: string;
  diff: string;
  status: string;
  additions: number;
  deletions: number;
  staged: boolean;
  source: "uncommitted" | "committed" | "pr";
  diff_skip_reason?: "too_large" | "binary" | "truncated" | "budget_exceeded";
  /**
   * Repository this file belongs to. Set on multi-repo task changes so the
   * file tree groups files under per-repo top-level nodes. Optional for
   * single-repo tasks; the tree falls back to flat path-only grouping.
   */
  repository_id?: string;
  /** Human-readable repo name for tree node labels (e.g. "frontend"). */
  repository_name?: string;
};

/**
 * Composite per-file key used by the review dialog's in-memory state
 * (reviewed set, stale set, file refs, selected file, comment counts) and
 * by the persisted `useSessionFileReviews` rows. The NUL separator is
 * impossible to embed in a real path or repository name, so the key is
 * always uniquely splittable. Single-repo files (no `repository_name`)
 * keep the legacy bare-path key for backwards compatibility with existing
 * `session_file_reviews` rows.
 */
const FILE_KEY_SEP = "\u0000";

export function reviewFileKey(file: { path: string; repository_name?: string }): string {
  return file.repository_name ? `${file.repository_name}${FILE_KEY_SEP}${file.path}` : file.path;
}

export function splitReviewFileKey(key: string): { repositoryName: string; path: string } {
  const sep = key.indexOf(FILE_KEY_SEP);
  if (sep < 0) return { repositoryName: "", path: key };
  return { repositoryName: key.slice(0, sep), path: key.slice(sep + 1) };
}

export function diffSkipReasonLabel(reason?: string): string {
  switch (reason) {
    case "too_large":
      return "File too large to diff (>10 MB)";
    case "binary":
      return "Binary file — not diffable";
    case "truncated":
      return "Diff truncated (>256 KB)";
    case "budget_exceeded":
      return "Diff skipped — too many changed files";
    default:
      return "Loading diff...";
  }
}

export type FileTreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children?: FileTreeNode[];
  file?: ReviewFile;
  /**
   * True when this node represents a repository root in a multi-repo file
   * tree. Repo roots are pinned (never collapsed into their first child) so
   * the user always sees the repo grouping at the top level.
   */
  isRepoRoot?: boolean;
  /** Repository id this node represents (only set when isRepoRoot is true). */
  repositoryId?: string;
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
 * Hash diff content for change detection.
 * Used to detect if a diff has changed since the user last reviewed it.
 */
export const hashDiff = djb2Hash;

/**
 * Build a hierarchical file tree from flat file paths.
 * Collapses single-child directories (e.g., `src/components` as one node if `src` has no other children).
 *
 * Multi-repo: when files carry repository_name and 2+ distinct repos are
 * present, the tree gets a top-level repo node per repository so the user
 * sees per-repo grouping. Repo roots are pinned (never collapsed) and the
 * file paths inside them stay relative to the repo root.
 */
export function buildFileTree(files: ReviewFile[]): FileTreeNode[] {
  const repoNames = new Set<string>();
  for (const f of files) {
    if (f.repository_name) repoNames.add(f.repository_name);
  }
  const isMultiRepo = repoNames.size > 1;

  if (isMultiRepo) return buildMultiRepoTree(files);
  return buildFlatTree(files);
}

function buildMultiRepoTree(files: ReviewFile[]): FileTreeNode[] {
  // Group by repo first, then build a sub-tree for each.
  const byRepo = new Map<string, { name: string; id: string; files: ReviewFile[] }>();
  for (const f of files) {
    const name = f.repository_name ?? "(unspecified)";
    const id = f.repository_id ?? name;
    const key = id;
    let entry = byRepo.get(key);
    if (!entry) {
      entry = { name, id, files: [] };
      byRepo.set(key, entry);
    }
    entry.files.push(f);
  }
  const repoRoots: FileTreeNode[] = [];
  for (const [, entry] of byRepo) {
    const subtree = buildFlatTree(entry.files);
    repoRoots.push({
      name: entry.name,
      path: `__repo__:${entry.id}`,
      isDir: true,
      isRepoRoot: true,
      repositoryId: entry.id,
      children: subtree,
    });
  }
  // Stable alphabetical order by repo name.
  return repoRoots.sort((a, b) => a.name.localeCompare(b.name));
}

function buildFlatTree(files: ReviewFile[]): FileTreeNode[] {
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

  // Collapse single-child directories. Repo roots are never collapsed —
  // they encode user-meaningful grouping that survives even when the repo
  // has only one changed file.
  function collapse(node: FileTreeNode): FileTreeNode {
    if (!node.isDir || !node.children) return node;

    node.children = node.children.map(collapse);

    if (
      node.children.length === 1 &&
      node.children[0].isDir &&
      node.name !== "" &&
      !node.isRepoRoot
    ) {
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
